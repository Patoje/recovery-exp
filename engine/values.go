package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Values resolves references used in the contract:
//
//	${env:NAME}             variable of the environment file (or of the process)
//	${run:FILE#a.b}         field of a JSON artifact of the run (FILE relative to the run dir)
//	${objective:NAME}       variable of the objectives file
//
// Templates use {field} for fields of the current item (a witness, a dependency).
type Values struct {
	RunDir     string
	Env        map[string]string
	Objectives map[string]string
	cache      map[string]any
}

func NewValues(runDir, envFile, objectivesFile string) (*Values, error) {
	v := &Values{RunDir: runDir, Env: map[string]string{}, Objectives: map[string]string{}, cache: map[string]any{}}
	if envFile != "" {
		m, err := readEnvFile(envFile)
		if err != nil {
			return nil, err
		}
		v.Env = m
	}
	if objectivesFile != "" {
		m, err := readEnvFile(objectivesFile)
		if err != nil {
			return nil, err
		}
		v.Objectives = m
	}
	return v, nil
}

func readEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "=") {
			continue
		}
		k, val, _ := strings.Cut(line, "=")
		out[strings.TrimSpace(k)] = strings.TrimSpace(val)
	}
	return out, s.Err()
}

var refRx = regexp.MustCompile(`\$\{(env|run|objective):([^}]*)\}`)

// IsRef reports whether s contains at least one reference.
func IsRef(s string) bool { return refRx.MatchString(s) }

// Resolve replaces every reference in s with its value (as text).
func (v *Values) Resolve(s string) (string, error) {
	var firstErr error
	out := refRx.ReplaceAllStringFunc(s, func(m string) string {
		parts := refRx.FindStringSubmatch(m)
		val, err := v.lookup(parts[1], parts[2])
		if err != nil && firstErr == nil {
			firstErr = err
		}
		switch x := val.(type) {
		case string:
			return x
		case nil:
			return ""
		default:
			b, _ := json.Marshal(x)
			return string(b)
		}
	})
	return out, firstErr
}

// ResolveAny returns the raw value of a string that is exactly one reference
// (maps and lists keep their structure); otherwise it resolves it as text.
func (v *Values) ResolveAny(s string) (any, error) {
	if m := refRx.FindStringSubmatch(s); m != nil && m[0] == s {
		return v.lookup(m[1], m[2])
	}
	return v.Resolve(s)
}

func (v *Values) lookup(kind, path string) (any, error) {
	switch kind {
	case "env":
		if x, ok := v.Env[path]; ok {
			return x, nil
		}
		if x, ok := os.LookupEnv(path); ok {
			return x, nil
		}
		return nil, fmt.Errorf("variable %s no definida", path)
	case "objective":
		if x, ok := v.Objectives[path]; ok {
			return x, nil
		}
		return nil, fmt.Errorf("objetivo %s no definido", path)
	case "run":
		file, field, _ := strings.Cut(path, "#")
		doc, err := v.artifact(file)
		if err != nil {
			return nil, err
		}
		if field == "" {
			return doc, nil
		}
		return jsonPath(doc, field)
	}
	return nil, fmt.Errorf("referencia desconocida %s", kind)
}

func (v *Values) artifact(file string) (any, error) {
	if strings.Contains(file, "..") || filepath.IsAbs(file) {
		return nil, fmt.Errorf("artefacto fuera de la corrida: %s", file)
	}
	if d, ok := v.cache[file]; ok {
		return d, nil
	}
	b, err := os.ReadFile(filepath.Join(v.RunDir, file))
	if err != nil {
		return nil, fmt.Errorf("artefacto %s: %w", file, err)
	}
	var d any
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("artefacto %s: %w", file, err)
	}
	v.cache[file] = d
	return d, nil
}

var selRx = regexp.MustCompile(`^([^\[]+)\[([^=\]]+)=([^\]]*)\]$`)

// jsonPath follows a.b.c; a segment name[k=v] takes the first element of the list
// "name" whose field k equals v.
func jsonPath(doc any, path string) (any, error) {
	cur := doc
	for _, seg := range strings.Split(path, ".") {
		k, selK, selV := seg, "", ""
		if m := selRx.FindStringSubmatch(seg); m != nil {
			k, selK, selV = m[1], m[2], m[3]
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("campo %s: no es un objeto", path)
		}
		cur, ok = m[k]
		if !ok {
			return nil, fmt.Errorf("campo %s no existe", path)
		}
		if selK != "" {
			l, _ := cur.([]any)
			var found any
			for _, it := range l {
				if o, ok := it.(map[string]any); ok && fmt.Sprint(o[selK]) == selV {
					found = it
					break
				}
			}
			if found == nil {
				return nil, fmt.Errorf("campo %s: ningún elemento con %s=%s", path, selK, selV)
			}
			cur = found
		}
	}
	return cur, nil
}

var fieldRx = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Fill replaces {field} with values of item. Missing fields are an error.
func Fill(tmpl string, item map[string]any) (string, error) {
	var missing []string
	out := fieldRx.ReplaceAllStringFunc(tmpl, func(m string) string {
		k := m[1 : len(m)-1]
		x, ok := item[k]
		if !ok {
			missing = append(missing, k)
			return ""
		}
		switch t := x.(type) {
		case string:
			return t
		case float64:
			if t == float64(int64(t)) {
				return fmt.Sprintf("%d", int64(t))
			}
			return fmt.Sprintf("%v", t)
		default:
			return fmt.Sprintf("%v", t)
		}
	})
	if missing != nil {
		return "", fmt.Errorf("faltan campos %v", missing)
	}
	return out, nil
}

// isSecretRef: values from the environment are never written to the report.
func isSecretRef(s string) bool {
	return strings.Contains(s, "${env:") || strings.Contains(s, "secrets/")
}
