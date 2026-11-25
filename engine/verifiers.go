package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	Pass = "pass"
	Fail = "fail"
	Err  = "error" // the evidence could not be obtained: never read as "empty"
)

// Result of one check. Expected and Observed never contain secret values.
type Result struct {
	ID         string    `json:"id"`
	Verifier   string    `json:"verifier"`
	Phase      string    `json:"phase"`
	Status     string    `json:"status"`
	Expected   string    `json:"expected,omitempty"`
	Observed   string    `json:"observed,omitempty"`
	Message    string    `json:"message"`
	Evidence   []string  `json:"evidence,omitempty"`
	Custom     bool      `json:"custom_check,omitempty"`
	Reason     string    `json:"custom_reason,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`
}

// Engine holds what checks may use: the contract, run artifacts, docker and the probe.
type Engine struct {
	C      *Contract
	V      *Values
	D      Docker
	P      ProbeRunner
	RunDir string
	Now    func() time.Time
}

type verifier struct {
	newParams func() any
	validate  func(c *Contract, p any, comps map[string]bool) error
	phase     func(p any) string
	host      func(e *Engine, id string, p any) Result                    // runs where the engine runs
	probe     func(e *Engine, id string, p any) ([]ProbeOp, error)        // network ops for the probe
	evaluate  func(e *Engine, id string, p any, res []ProbeResult) Result // after the probe ran
	final     bool                                                        // evaluated after every other check
}

func post(any) string { return "post" }
func pre(any) string  { return "pre" }

func needComponent(name string, comps map[string]bool) error {
	if name == "" || !comps[name] {
		return fmt.Errorf("componente %q no declarado", name)
	}
	return nil
}

func noValidate(*Contract, any, map[string]bool) error { return nil }

// ---------- parameters ----------

type ComponentImageParams struct {
	Component string `yaml:"component"`
	Source    string `yaml:"source"` // configured | running
	Expected  string `yaml:"expected"`
}

type ComponentVersionParams struct {
	Component string   `yaml:"component"`
	Command   []string `yaml:"command"`
	Regex     string   `yaml:"regex"`
	Expected  string   `yaml:"expected"`
}

type ComponentsRunningParams struct {
	Components     []string `yaml:"components"`
	RequireHealthy bool     `yaml:"require_healthy"`
}

type StartOrderParams struct{}

type SecretFingerprintParams struct {
	Component string `yaml:"component"`
	File      string `yaml:"file"`
	Format    string `yaml:"format"` // ini | env | raw
	Section   string `yaml:"section"`
	Key       string `yaml:"key"`
	Expected  string `yaml:"expected"` // sha256 hex
}

type DBRef struct {
	Component string `yaml:"component"`
	Client    string `yaml:"client"` // psql
	User      string `yaml:"user"`
	Name      string `yaml:"name"`
}

type SQLScalarParams struct {
	Database DBRef  `yaml:"database"`
	Query    string `yaml:"query"`
	Expected string `yaml:"expected"`
}

type TableCountsParams struct {
	Database  DBRef    `yaml:"database"`
	Schema    string   `yaml:"schema"`
	Inventory string   `yaml:"inventory"`
	Ignore    []string `yaml:"ignore"`
}

type DiskRef struct {
	Component   string `yaml:"component"`
	Root        string `yaml:"root"`
	Glob        string `yaml:"glob"`
	StripSuffix string `yaml:"strip_suffix"`
}

type EntitiesParams struct {
	Database DBRef   `yaml:"database"`
	Query    string  `yaml:"query"`
	Disk     DiskRef `yaml:"disk"`
}

type GitRefsParams struct {
	Disk      DiskRef `yaml:"disk"`
	Inventory string  `yaml:"inventory"`
}

type LogPatternsParams struct {
	Files        []string `yaml:"files"`
	Patterns     []string `yaml:"patterns"`
	MustMatch    []string `yaml:"must_match"`
	MustNotMatch []string `yaml:"must_not_match"`
	rx           []*regexp.Regexp
}

type ResticCheckParams struct {
	Service    string   `yaml:"service"`
	Profiles   []string `yaml:"profiles"`
	GlobalArgs []string `yaml:"global_args"`
	CheckArgs  []string `yaml:"check_args"`
}

type CustomCheckParams struct {
	Component string   `yaml:"component"`
	Command   []string `yaml:"command"`
	Reason    string   `yaml:"reason"`
}

type HTTPRequestParams struct {
	Request HTTPSpec   `yaml:"request"`
	Expect  ExpectSpec `yaml:"expect"`
}

type TOTPSpec struct {
	Secret string `yaml:"secret"`
	Header string `yaml:"header"`
	Digits int    `yaml:"digits"`
	Period int    `yaml:"period"`
}

type LoginTOTPParams struct {
	Request HTTPSpec   `yaml:"request"`
	TOTP    TOTPSpec   `yaml:"totp"`
	Expect  ExpectSpec `yaml:"expect"`
}

type GitPushParams struct {
	URL        string            `yaml:"url"`
	Auth       *Auth             `yaml:"auth"`
	FilePrefix string            `yaml:"file_prefix"`
	Author     string            `yaml:"author"`
	Env        map[string]string `yaml:"env"` // extra environment for git
}

type DependenciesParams struct {
	From    string     `yaml:"from"` // reference to a list of objects
	Request HTTPSpec   `yaml:"request"`
	Expect  ExpectSpec `yaml:"expect"`
}

type WitnessFetchParams struct {
	Type     string   `yaml:"type"`
	Sources  []string `yaml:"sources"`
	MinCount int      `yaml:"min_count"`
}

type RTOParams struct {
	Objective       string   `yaml:"objective"`
	RestoreStarted  string   `yaml:"restore_started"`
	RestoreStatus   string   `yaml:"restore_status"`
	RestoreStatusOK string   `yaml:"restore_status_ok"`
	Requires        []string `yaml:"requires"`
}

type RPOParams struct {
	Objective   string   `yaml:"objective"`
	Incident    string   `yaml:"incident"`
	Sources     []string `yaml:"sources"`
	CreatedAt   string   `yaml:"created_at_field"`
	WitnessType string   `yaml:"type_field"`
}

// ---------- registry: the closed vocabulary ----------

var verifiers map[string]verifier

func init() {
	verifiers = map[string]verifier{
		"component_image": {
			newParams: func() any { return &ComponentImageParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*ComponentImageParams)
				if x.Source != "configured" && x.Source != "running" {
					return fmt.Errorf("source debe ser configured o running")
				}
				return needComponent(x.Component, comps)
			},
			phase: func(p any) string {
				if p.(*ComponentImageParams).Source == "configured" {
					return "pre"
				}
				return "post"
			},
			host: checkComponentImage,
		},
		"component_version": {
			newParams: func() any { return &ComponentVersionParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*ComponentVersionParams)
				if len(x.Command) == 0 {
					return fmt.Errorf("command vacío")
				}
				if _, err := regexp.Compile(x.Regex); err != nil || !strings.Contains(x.Regex, "(") {
					return fmt.Errorf("regex inválida o sin grupo")
				}
				return needComponent(x.Component, comps)
			},
			phase: pre, host: checkComponentVersion,
		},
		"components_running": {
			newParams: func() any { return &ComponentsRunningParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				for _, n := range p.(*ComponentsRunningParams).Components {
					if err := needComponent(n, comps); err != nil {
						return err
					}
				}
				return nil
			},
			phase: post, host: checkComponentsRunning,
		},
		"start_order": {
			newParams: func() any { return &StartOrderParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				if len(c.Components) < 2 {
					return fmt.Errorf("hacen falta al menos dos componentes")
				}
				return nil
			},
			phase: post, host: checkStartOrder,
		},
		"secret_fingerprint": {
			newParams: func() any { return &SecretFingerprintParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*SecretFingerprintParams)
				if x.Format != "ini" && x.Format != "env" && x.Format != "raw" {
					return fmt.Errorf("format debe ser ini, env o raw")
				}
				if x.Format != "raw" && x.Key == "" {
					return fmt.Errorf("key obligatoria")
				}
				return needComponent(x.Component, comps)
			},
			phase: post, host: checkSecretFingerprint,
		},
		"sql_scalar": {
			newParams: func() any { return &SQLScalarParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*SQLScalarParams)
				return validateDB(x.Database, x.Query != "", comps)
			},
			phase: post, host: checkSQLScalar,
		},
		"table_counts_match_inventory": {
			newParams: func() any { return &TableCountsParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*TableCountsParams)
				return validateDB(x.Database, x.Schema != "" && IsRef(x.Inventory), comps)
			},
			phase: post, host: checkTableCounts,
		},
		"entities_match_disk": {
			newParams: func() any { return &EntitiesParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*EntitiesParams)
				if err := validateDisk(x.Disk, comps); err != nil {
					return err
				}
				return validateDB(x.Database, x.Query != "", comps)
			},
			phase: post, host: checkEntities,
		},
		"git_refs_match_inventory": {
			newParams: func() any { return &GitRefsParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*GitRefsParams)
				if !IsRef(x.Inventory) {
					return fmt.Errorf("inventory debe ser una referencia")
				}
				return validateDisk(x.Disk, comps)
			},
			phase: post, host: checkGitRefs,
		},
		"log_patterns": {
			newParams: func() any { return &LogPatternsParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*LogPatternsParams)
				if len(x.Files) == 0 || len(x.Patterns) == 0 {
					return fmt.Errorf("files y patterns obligatorios")
				}
				for _, s := range x.Patterns {
					rx, err := regexp.Compile(s)
					if err != nil {
						return fmt.Errorf("patrón %q: %v", s, err)
					}
					x.rx = append(x.rx, rx)
				}
				return nil
			},
			phase: post, host: checkLogPatterns,
		},
		"restic_check": {
			newParams: func() any { return &ResticCheckParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				if p.(*ResticCheckParams).Service == "" {
					return fmt.Errorf("service obligatorio")
				}
				return nil
			},
			phase: pre, host: checkRestic,
		},
		"custom_check": {
			newParams: func() any { return &CustomCheckParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*CustomCheckParams)
				if len(x.Command) == 0 || strings.TrimSpace(x.Reason) == "" {
					return fmt.Errorf("command y reason obligatorios")
				}
				return needComponent(x.Component, comps)
			},
			phase: post, host: checkCustom,
		},
		"http_request": {
			newParams: func() any { return &HTTPRequestParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				return validateHTTP(p.(*HTTPRequestParams).Request)
			},
			phase: post,
			probe: func(e *Engine, id string, p any) ([]ProbeOp, error) {
				x := p.(*HTTPRequestParams)
				op, err := e.httpOp(id, x.Request, nil)
				return []ProbeOp{op}, err
			},
			evaluate: func(e *Engine, id string, p any, res []ProbeResult) Result {
				return e.evalSingle(id, p.(*HTTPRequestParams).Expect, res)
			},
		},
		"login_totp": {
			newParams: func() any { return &LoginTOTPParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*LoginTOTPParams)
				if x.TOTP.Secret == "" || x.TOTP.Header == "" || x.Request.Auth == nil {
					return fmt.Errorf("totp.secret, totp.header y request.auth obligatorios")
				}
				return validateHTTP(x.Request)
			},
			phase: post,
			probe: func(e *Engine, id string, p any) ([]ProbeOp, error) {
				x := p.(*LoginTOTPParams)
				op, err := e.httpOp(id, x.Request, nil)
				if err != nil {
					return nil, err
				}
				sec, err := e.V.Resolve(x.TOTP.Secret)
				if err != nil {
					return nil, err
				}
				op.TOTP = &TOTP{Secret: sec, Header: x.TOTP.Header, Digits: x.TOTP.Digits, Period: x.TOTP.Period}
				return []ProbeOp{op}, nil
			},
			evaluate: func(e *Engine, id string, p any, res []ProbeResult) Result {
				return e.evalSingle(id, p.(*LoginTOTPParams).Expect, res)
			},
		},
		"git_push": {
			newParams: func() any { return &GitPushParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				if p.(*GitPushParams).URL == "" {
					return fmt.Errorf("url obligatoria")
				}
				return nil
			},
			phase: post, probe: gitPushOps, evaluate: evalGitPush,
		},
		"dependencies_reachable": {
			newParams: func() any { return &DependenciesParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*DependenciesParams)
				if !IsRef(x.From) {
					return fmt.Errorf("from debe ser una referencia a una lista")
				}
				return validateHTTP(x.Request)
			},
			phase: post, probe: dependencyOps, evaluate: evalDependencies,
		},
		"witness_fetch": {
			newParams: func() any { return &WitnessFetchParams{MinCount: 1} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*WitnessFetchParams)
				if _, ok := c.WitnessTypes[x.Type]; !ok {
					return fmt.Errorf("tipo de testigo %q no declarado en witness_types", x.Type)
				}
				if len(x.Sources) == 0 {
					return fmt.Errorf("sources obligatorio")
				}
				return nil
			},
			phase: post, probe: witnessFetchOps, evaluate: evalWitnessFetch,
		},
		"rpo": {
			newParams: func() any { return &RPOParams{CreatedAt: "created_at", WitnessType: "type"} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*RPOParams)
				if !IsRef(x.Objective) || !IsRef(x.Incident) || len(x.Sources) == 0 {
					return fmt.Errorf("objective, incident y sources obligatorios")
				}
				return nil
			},
			phase: post, probe: rpoOps, evaluate: evalRPO,
		},
		"rto": {
			newParams: func() any { return &RTOParams{} },
			validate: func(c *Contract, p any, comps map[string]bool) error {
				x := p.(*RTOParams)
				if !IsRef(x.Objective) || !IsRef(x.RestoreStarted) || len(x.Requires) == 0 {
					return fmt.Errorf("objective, restore_started y requires obligatorios")
				}
				return nil
			},
			phase: post, final: true,
		},
	}
}

func validateDB(db DBRef, extra bool, comps map[string]bool) error {
	if db.Client != "psql" {
		return fmt.Errorf("database.client: solo psql")
	}
	if db.User == "" || db.Name == "" || !extra {
		return fmt.Errorf("faltan parámetros de la base o de la consulta")
	}
	return needComponent(db.Component, comps)
}

func validateDisk(d DiskRef, comps map[string]bool) error {
	if !strings.HasPrefix(d.Root, "/") || d.Glob == "" {
		return fmt.Errorf("disk.root absoluto y disk.glob obligatorios")
	}
	return needComponent(d.Component, comps)
}

func validateHTTP(h HTTPSpec) error {
	if h.URL == "" {
		return fmt.Errorf("request.url obligatoria")
	}
	return nil
}

func phaseOf(ch Check) string { return verifiers[ch.Verifier].phase(ch.params) }

// ---------- helpers ----------

func (e *Engine) service(component string) string {
	c, _ := e.C.component(component)
	return c.Service
}

func (e *Engine) start(id, verifierName, phase string) Result {
	return Result{ID: id, Verifier: verifierName, Phase: phase, StartedAt: e.Now()}
}

func (e *Engine) done(r Result, status, msg string) Result {
	r.Status, r.Message = status, msg
	r.FinishedAt = e.Now()
	r.DurationMS = r.FinishedAt.Sub(r.StartedAt).Milliseconds()
	return r
}

// shown returns the text of an expected value for the report (never a secret).
func (e *Engine) shown(expr, resolved string) string {
	if isSecretRef(expr) {
		return expr
	}
	return resolved
}

func (e *Engine) evidence(id, suffix string, data []byte) string {
	rel := filepath.Join("contract", "evidence", id+suffix)
	p := filepath.Join(e.RunDir, rel)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return ""
	}
	return rel
}

func (e *Engine) psql(db DBRef, sql string, vars map[string]string) (string, error) {
	user, err := e.V.Resolve(db.User)
	if err != nil {
		return "", err
	}
	name, err := e.V.Resolve(db.Name)
	if err != nil {
		return "", err
	}
	argv := []string{"psql", "-U", user, "-d", name, "-At", "-X", "-v", "ON_ERROR_STOP=1"}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		argv = append(argv, "-v", k+"="+vars[k])
	}
	argv = append(argv, "-f", "-")
	out, err := Exec(e.D, e.service(db.Component), map[string]string{"PGOPTIONS": "-c default_transaction_read_only=on"},
		[]byte(sql), argv...)
	return strings.TrimSpace(string(out)), err
}

// listDisk returns the entries matching root/glob in a service. A failure to read is an error.
func (e *Engine) listDisk(d DiskRef, withRefs bool) (string, error) {
	script := `cd "$1" || exit 3
for d in $2; do [ -e "$d" ] || continue; echo "## $d"; done`
	if withRefs {
		script = `cd "$1" || exit 3
for d in $2; do [ -e "$d" ] || continue; echo "## $d"
git -c safe.directory='*' --git-dir "$d" for-each-ref --format='%(refname) %(objectname)' || exit 4; done`
	}
	out, err := Exec(e.D, e.service(d.Component), nil, nil, "sh", "-c", script, "sh", d.Root, d.Glob)
	return string(out), err
}

func parseListing(out, strip string) map[string]map[string]string {
	m := map[string]map[string]string{}
	var cur string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "## ") {
			cur = strings.TrimSuffix(l[3:], strip)
			m[cur] = map[string]string{}
		} else if cur != "" && strings.Contains(l, " ") {
			k, v, _ := strings.Cut(l, " ")
			m[cur][k] = v
		}
	}
	return m
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------- host checks ----------

func checkComponentImage(e *Engine, id string, p any) Result {
	x := p.(*ComponentImageParams)
	r := e.start(id, "component_image", verifiers["component_image"].phase(p))
	want, err := e.V.Resolve(x.Expected)
	r.Expected = want
	if err != nil {
		return e.done(r, Err, "valor esperado: "+err.Error())
	}
	var got string
	if x.Source == "configured" {
		got, err = ConfiguredImage(e.D, e.service(x.Component))
	} else {
		var st ContainerState
		st, err = State(e.D, e.service(x.Component))
		got = st.Image
	}
	if err != nil {
		return e.done(r, Err, err.Error())
	}
	r.Observed = got
	if got != want {
		return e.done(r, Fail, fmt.Sprintf("imagen %s %s; esperada %s", x.Source, got, want))
	}
	return e.done(r, Pass, fmt.Sprintf("imagen %s coincide", x.Source))
}

func checkComponentVersion(e *Engine, id string, p any) Result {
	x := p.(*ComponentVersionParams)
	r := e.start(id, "component_version", "pre")
	want, err := e.V.Resolve(x.Expected)
	r.Expected = want
	if err != nil {
		return e.done(r, Err, "valor esperado: "+err.Error())
	}
	img, err := ConfiguredImage(e.D, e.service(x.Component))
	if err != nil {
		return e.done(r, Err, err.Error())
	}
	args := append([]string{"run", "--rm", "--network", "none", "--entrypoint", x.Command[0], img}, x.Command[1:]...)
	out, errb, err := e.D.Docker(args...)
	r.Evidence = []string{e.evidence(id, ".txt", append(out, errb...))}
	if err != nil {
		return e.done(r, Err, fmt.Sprintf("%s en %s: %v", x.Command[0], img, err))
	}
	m := regexp.MustCompile(x.Regex).FindSubmatch(append(out, errb...))
	if m == nil {
		return e.done(r, Err, "la salida no coincide con la regex")
	}
	r.Observed = string(m[1])
	if r.Observed != want {
		return e.done(r, Fail, fmt.Sprintf("versión de la imagen configurada %s; esperada %s", r.Observed, want))
	}
	return e.done(r, Pass, "versión de la imagen configurada coincide")
}

func checkComponentsRunning(e *Engine, id string, p any) Result {
	x := p.(*ComponentsRunningParams)
	r := e.start(id, "components_running", "post")
	names := x.Components
	if len(names) == 0 {
		for _, c := range e.C.Components {
			names = append(names, c.Name)
		}
	}
	var bad, errs, seen []string
	for _, n := range names {
		st, err := State(e.D, e.service(n))
		if err != nil {
			errs = append(errs, n+": "+err.Error())
			continue
		}
		seen = append(seen, fmt.Sprintf("%s=%s/%s", n, st.Status, st.Health))
		if st.Status != "running" || (x.RequireHealthy && st.Health != "healthy") {
			bad = append(bad, fmt.Sprintf("%s: %s %s", n, st.Status, st.Health))
		}
	}
	r.Observed = strings.Join(seen, " ")
	switch {
	case len(bad) > 0:
		return e.done(r, Fail, "no en marcha: "+strings.Join(append(bad, errs...), "; "))
	case len(errs) > 0:
		return e.done(r, Err, strings.Join(errs, "; "))
	}
	return e.done(r, Pass, fmt.Sprintf("%d componentes en marcha", len(names)))
}

func checkStartOrder(e *Engine, id string, p any) Result {
	r := e.start(id, "start_order", "post")
	var names []string
	var prev time.Time
	for i, c := range e.C.Components {
		names = append(names, c.Name)
		st, err := State(e.D, c.Service)
		if err != nil {
			return e.done(r, Err, c.Name+": "+err.Error())
		}
		if st.StartedAt.IsZero() {
			return e.done(r, Err, c.Name+": sin hora de arranque")
		}
		r.Observed += fmt.Sprintf("%s=%s ", c.Name, st.StartedAt.UTC().Format(time.RFC3339Nano))
		if i > 0 && st.StartedAt.Before(prev) {
			return e.done(r, Fail, fmt.Sprintf("%s arrancó antes que %s", c.Name, e.C.Components[i-1].Name))
		}
		prev = st.StartedAt
	}
	r.Expected = strings.Join(names, " → ")
	return e.done(r, Pass, "orden de arranque "+r.Expected)
}

func checkSecretFingerprint(e *Engine, id string, p any) Result {
	x := p.(*SecretFingerprintParams)
	r := e.start(id, "secret_fingerprint", "post")
	want, err := e.V.Resolve(x.Expected)
	r.Expected = want
	if err != nil {
		return e.done(r, Err, "huella esperada: "+err.Error())
	}
	out, err := Exec(e.D, e.service(x.Component), nil, nil, "cat", x.File)
	if err != nil {
		return e.done(r, Err, err.Error())
	}
	val, found := secretValue(string(out), x.Format, x.Section, x.Key)
	if !found {
		r.Observed = "(clave ausente)"
		return e.done(r, Fail, fmt.Sprintf("la clave %s no está en %s", x.Key, x.File))
	}
	r.Observed = sha256hex([]byte(val))
	if r.Observed != want {
		return e.done(r, Fail, "la huella en uso difiere de la declarada")
	}
	return e.done(r, Pass, "la huella en uso coincide con la declarada")
}

func checkSQLScalar(e *Engine, id string, p any) Result {
	x := p.(*SQLScalarParams)
	r := e.start(id, "sql_scalar", "post")
	want, err := e.V.Resolve(x.Expected)
	r.Expected = want
	if err != nil {
		return e.done(r, Err, "valor esperado: "+err.Error())
	}
	got, err := e.psql(x.Database, x.Query, nil)
	if err != nil {
		return e.done(r, Err, err.Error())
	}
	r.Observed = got
	if got != want {
		return e.done(r, Fail, fmt.Sprintf("%s; esperado %s", got, want))
	}
	return e.done(r, Pass, "valor esperado")
}

const countSQL = `select coalesce(json_object_agg(t.table_name, (xpath('/row/c/text()',
  query_to_xml(format('select count(*) as c from %I.%I', t.table_schema, t.table_name), false, true, '')))[1]::text::bigint
  order by t.table_name), '{}'::json) from information_schema.tables t
  where t.table_schema = :'schema' and t.table_type = 'BASE TABLE';`

func checkTableCounts(e *Engine, id string, p any) Result {
	x := p.(*TableCountsParams)
	r := e.start(id, "table_counts_match_inventory", "post")
	refAny, err := e.V.ResolveAny(x.Inventory)
	ref, ok := refAny.(map[string]any)
	if err != nil || !ok {
		return e.done(r, Err, fmt.Sprintf("inventario %s: %v", x.Inventory, err))
	}
	out, err := e.psql(x.Database, countSQL, map[string]string{"schema": x.Schema})
	if err != nil {
		return e.done(r, Err, err.Error())
	}
	r.Evidence = []string{e.evidence(id, ".json", []byte(out))}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		return e.done(r, Err, "conteos ilegibles: "+err.Error())
	}
	ignore := map[string]bool{}
	for _, t := range x.Ignore {
		ignore[t] = true
	}
	all := map[string]bool{}
	for k := range ref {
		all[k] = true
	}
	for k := range got {
		all[k] = true
	}
	var diffs []string
	for _, t := range keys(all) {
		if ignore[t] {
			continue
		}
		a, b := numText(ref[t]), numText(got[t])
		if a != b {
			diffs = append(diffs, fmt.Sprintf("%s: %s→%s", t, a, b))
		}
	}
	r.Expected = fmt.Sprintf("%d tablas del inventario", len(ref))
	r.Observed = fmt.Sprintf("%d tablas restauradas", len(got))
	if diffs != nil {
		return e.done(r, Fail, "diferencias (inventario→restaurada): "+strings.Join(diffs, ", "))
	}
	return e.done(r, Pass, fmt.Sprintf("%d tablas iguales", len(ref)-len(ignore)))
}

func numText(v any) string {
	switch t := v.(type) {
	case nil:
		return "—"
	case float64:
		return strconv.FormatInt(int64(t), 10)
	default:
		return fmt.Sprint(t)
	}
}

func checkEntities(e *Engine, id string, p any) Result {
	x := p.(*EntitiesParams)
	r := e.start(id, "entities_match_disk", "post")
	dbOut, dbErr := e.psql(x.Database, x.Query, nil)
	diskOut, diskErr := e.listDisk(x.Disk, false)
	r.Evidence = []string{e.evidence(id, ".db.txt", []byte(dbOut)), e.evidence(id, ".disk.txt", []byte(diskOut))}
	var errs []string
	if dbErr != nil {
		errs = append(errs, "base: "+dbErr.Error())
	}
	if diskErr != nil {
		errs = append(errs, "disco: "+diskErr.Error())
	}
	if errs != nil {
		return e.done(r, Err, "no se pudo leer: "+strings.Join(errs, "; "))
	}
	db := map[string]bool{}
	for _, l := range strings.Fields(dbOut) {
		db[l] = true
	}
	disk := parseListing(diskOut, x.Disk.StripSuffix)
	var onlyDB, onlyDisk []string
	for k := range db {
		if _, ok := disk[k]; !ok {
			onlyDB = append(onlyDB, k)
		}
	}
	for k := range disk {
		if !db[k] {
			onlyDisk = append(onlyDisk, k)
		}
	}
	sort.Strings(onlyDB)
	sort.Strings(onlyDisk)
	r.Observed = fmt.Sprintf("%d en la base, %d en disco", len(db), len(disk))
	if onlyDB != nil || onlyDisk != nil {
		return e.done(r, Fail, fmt.Sprintf("%s; solo en base %v; solo en disco %v", r.Observed, onlyDB, onlyDisk))
	}
	return e.done(r, Pass, r.Observed)
}

func checkGitRefs(e *Engine, id string, p any) Result {
	x := p.(*GitRefsParams)
	r := e.start(id, "git_refs_match_inventory", "post")
	refAny, err := e.V.ResolveAny(x.Inventory)
	ref, ok := refAny.(map[string]any)
	if err != nil || !ok {
		return e.done(r, Err, fmt.Sprintf("inventario %s: %v", x.Inventory, err))
	}
	out, err := e.listDisk(x.Disk, true)
	r.Evidence = []string{e.evidence(id, ".txt", []byte(out))}
	if err != nil {
		return e.done(r, Err, "no se pudo leer el disco: "+err.Error())
	}
	disk := parseListing(out, x.Disk.StripSuffix)
	all := map[string]bool{}
	for k := range ref {
		all[k] = true
	}
	for k := range disk {
		all[k] = true
	}
	var diffs []string
	for _, name := range keys(all) {
		a, inRef := ref[name].(map[string]any)
		b, inDisk := disk[name]
		switch {
		case !inDisk:
			diffs = append(diffs, name+": falta en disco")
		case !inRef:
			diffs = append(diffs, name+": no está en el inventario")
		default:
			ks := map[string]bool{}
			for k := range a {
				ks[k] = true
			}
			for k := range b {
				ks[k] = true
			}
			var d []string
			for _, k := range keys(ks) {
				if fmt.Sprint(a[k]) != b[k] {
					d = append(d, k)
				}
			}
			if d != nil {
				diffs = append(diffs, fmt.Sprintf("%s: %v", name, d))
			}
		}
	}
	r.Expected = fmt.Sprintf("%d repositorios del inventario", len(ref))
	r.Observed = fmt.Sprintf("%d en disco", len(disk))
	if diffs != nil {
		return e.done(r, Fail, strings.Join(diffs, "; "))
	}
	return e.done(r, Pass, fmt.Sprintf("refs de %d repositorios iguales", len(ref)))
}

func (x *LogPatternsParams) matches(line string) bool {
	for _, rx := range x.rx {
		if rx.MatchString(line) {
			return true
		}
	}
	return false
}

func checkLogPatterns(e *Engine, id string, p any) Result {
	x := p.(*LogPatternsParams)
	r := e.start(id, "log_patterns", "post")
	var wrong []string
	for _, s := range x.MustMatch {
		if !x.matches(s) {
			wrong = append(wrong, "no detecta: "+s)
		}
	}
	for _, s := range x.MustNotMatch {
		if x.matches(s) {
			wrong = append(wrong, "detecta por error: "+s)
		}
	}
	if wrong != nil {
		return e.done(r, Fail, "autoprueba de patrones: "+strings.Join(wrong, "; "))
	}
	var hits []string
	benign := map[string]int{}
	for _, f := range x.Files {
		b, err := os.ReadFile(filepath.Join(e.RunDir, f))
		if err != nil {
			return e.done(r, Err, "log "+f+": "+err.Error())
		}
		r.Evidence = append(r.Evidence, f)
		for i, l := range strings.Split(string(b), "\n") {
			if x.matches(l) {
				hits = append(hits, fmt.Sprintf("%s:%d: %s", f, i+1, truncate(strings.TrimSpace(l), 200)))
			}
			for _, s := range x.MustNotMatch {
				if strings.Contains(l, s) {
					benign[s]++
				}
			}
		}
	}
	var bn []string
	for _, s := range keys(benign) {
		bn = append(bn, fmt.Sprintf("%q ×%d", s, benign[s]))
	}
	note := fmt.Sprintf("autoprueba ok (%d sí, %d no); líneas que no cuentan: %s", len(x.MustMatch), len(x.MustNotMatch), strings.Join(bn, ", "))
	r.Observed = fmt.Sprintf("%d líneas de error", len(hits))
	if hits != nil {
		if len(hits) > 10 {
			hits = hits[:10]
		}
		return e.done(r, Fail, note+"; "+strings.Join(hits, " | "))
	}
	return e.done(r, Pass, note)
}

func checkRestic(e *Engine, id string, p any) Result {
	x := p.(*ResticCheckParams)
	r := e.start(id, "restic_check", "pre")
	args := append([]string{"run", "--rm", "--no-deps", "-T", x.Service}, x.GlobalArgs...)
	args = append(append(args, "check"), x.CheckArgs...)
	out, errb, err := e.D.Compose(x.Profiles, nil, args...)
	r.Evidence = []string{e.evidence(id, ".log", append(out, errb...))}
	if err != nil {
		return e.done(r, Fail, "restic check: "+lastLine(append(out, errb...)))
	}
	return e.done(r, Pass, "restic check sin errores")
}

func checkCustom(e *Engine, id string, p any) Result {
	x := p.(*CustomCheckParams)
	r := e.start(id, "custom_check", "post")
	r.Custom, r.Reason = true, x.Reason
	out, err := Exec(e.D, e.service(x.Component), nil, nil, x.Command...)
	r.Evidence = []string{e.evidence(id, ".txt", out)}
	if err != nil {
		return e.done(r, Fail, err.Error())
	}
	return e.done(r, Pass, "comando terminó con 0")
}

// ---------- probe checks ----------

func (e *Engine) httpOp(key string, h HTTPSpec, item map[string]any) (ProbeOp, error) {
	url := h.URL
	var err error
	if item != nil {
		if url, err = Fill(url, item); err != nil {
			return ProbeOp{}, err
		}
	}
	if url, err = e.V.Resolve(url); err != nil {
		return ProbeOp{}, err
	}
	op := ProbeOp{Key: key, Kind: "http", Method: h.Method, URL: url, FollowJSONField: h.FollowJSONField}
	if h.Auth != nil {
		if op.User, err = e.V.Resolve(h.Auth.User); err != nil {
			return ProbeOp{}, err
		}
		if op.Password, err = e.V.Resolve(h.Auth.Password); err != nil {
			return ProbeOp{}, err
		}
	}
	return op, nil
}

// expectOK compares one probe result with the expectation. ok=false with measured=false
// means the evidence could not be obtained.
func (e *Engine) expectOK(x ExpectSpec, res ProbeResult, item map[string]any) (ok, measured bool, msg string) {
	if res.Error != "" {
		return false, false, res.Error
	}
	want := x.Status
	if want == 0 {
		want = 200
	}
	if res.Status != want {
		d := ""
		if res.Detail != "" {
			d = " (" + res.Detail + ")"
		}
		return false, true, fmt.Sprintf("HTTP %d, esperado %d%s", res.Status, want, d)
	}
	for _, k := range keys(x.JSON) {
		exp := x.JSON[k]
		var err error
		if item != nil {
			if exp, err = Fill(exp, item); err != nil {
				return false, false, err.Error()
			}
		}
		if exp, err = e.V.Resolve(exp); err != nil {
			return false, false, err.Error()
		}
		got, present := res.JSON[k]
		if !present || fmt.Sprint(got) != exp {
			return false, true, fmt.Sprintf("%s=%v, esperado %s", k, got, e.shown(x.JSON[k], exp))
		}
	}
	if x.SHA256OfBody != "" {
		exp := x.SHA256OfBody
		var err error
		if item != nil {
			if exp, err = Fill(exp, item); err != nil {
				return false, false, err.Error()
			}
		}
		if res.BodySHA256 != exp {
			return false, true, fmt.Sprintf("sha256 %s, esperado %s", truncate(res.BodySHA256, 12), truncate(exp, 12))
		}
	}
	return true, true, fmt.Sprintf("HTTP %d", res.Status)
}

func byKey(res []ProbeResult, key string) (ProbeResult, bool) {
	for _, r := range res {
		if r.Key == key {
			return r, true
		}
	}
	return ProbeResult{}, false
}

func (e *Engine) probeResult(id string, verifierName string, res []ProbeResult, keysWanted []string) (Result, []ProbeResult, bool) {
	r := Result{ID: id, Verifier: verifierName, Phase: "post"}
	var got []ProbeResult
	for _, k := range keysWanted {
		pr, ok := byKey(res, k)
		if !ok {
			r.StartedAt = e.Now()
			return e.done(r, Err, "la sonda no devolvió "+k), nil, false
		}
		got = append(got, pr)
	}
	if len(got) > 0 {
		r.StartedAt = got[0].StartedAt
		r.FinishedAt = got[len(got)-1].FinishedAt
	} else {
		r.StartedAt = e.Now()
		r.FinishedAt = r.StartedAt
	}
	return r, got, true
}

func (e *Engine) finishProbe(r Result, status, msg string) Result {
	r.Status, r.Message = status, msg
	if r.FinishedAt.IsZero() {
		r.FinishedAt = e.Now()
	}
	r.DurationMS = r.FinishedAt.Sub(r.StartedAt).Milliseconds()
	return r
}

func (e *Engine) evalSingle(id string, x ExpectSpec, res []ProbeResult) Result {
	vname := ""
	for _, ch := range e.C.Checks {
		if ch.ID == id {
			vname = ch.Verifier
		}
	}
	r, got, ok := e.probeResult(id, vname, res, []string{id})
	if !ok {
		return r
	}
	pass, measured, msg := e.expectOK(x, got[0], nil)
	r.Observed = fmt.Sprintf("HTTP %d", got[0].Status)
	switch {
	case pass:
		return e.finishProbe(r, Pass, msg)
	case measured:
		return e.finishProbe(r, Fail, msg)
	}
	return e.finishProbe(r, Err, "no se pudo verificar: "+msg)
}

func gitPushOps(e *Engine, id string, p any) ([]ProbeOp, error) {
	x := p.(*GitPushParams)
	url, err := e.V.Resolve(x.URL)
	if err != nil {
		return nil, err
	}
	op := ProbeOp{Key: id, Kind: "git_push", URL: url, FilePrefix: x.FilePrefix, Author: x.Author, Env: x.Env}
	if x.Auth != nil {
		if op.User, err = e.V.Resolve(x.Auth.User); err != nil {
			return nil, err
		}
		if op.Password, err = e.V.Resolve(x.Auth.Password); err != nil {
			return nil, err
		}
	}
	return []ProbeOp{op}, nil
}

func evalGitPush(e *Engine, id string, p any, res []ProbeResult) Result {
	r, got, ok := e.probeResult(id, "git_push", res, []string{id})
	if !ok {
		return r
	}
	if got[0].Error != "" {
		if strings.HasPrefix(got[0].Error, "clone:") {
			return e.finishProbe(r, Err, "no se pudo verificar: "+got[0].Error)
		}
		return e.finishProbe(r, Fail, got[0].Error)
	}
	return e.finishProbe(r, Pass, got[0].Detail)
}

func (e *Engine) listRef(ref string) ([]map[string]any, error) {
	v, err := e.V.ResolveAny(ref)
	if err != nil {
		return nil, err
	}
	l, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s no es una lista", ref)
	}
	out := make([]map[string]any, 0, len(l))
	for _, it := range l {
		m, ok := it.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s: elemento que no es un objeto", ref)
		}
		out = append(out, m)
	}
	return out, nil
}

func dependencyOps(e *Engine, id string, p any) ([]ProbeOp, error) {
	x := p.(*DependenciesParams)
	deps, err := e.listRef(x.From)
	if err != nil {
		return nil, err
	}
	var ops []ProbeOp
	for i, d := range deps {
		op, err := e.httpOp(fmt.Sprintf("%s/%d", id, i), x.Request, d)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func evalDependencies(e *Engine, id string, p any, res []ProbeResult) Result {
	x := p.(*DependenciesParams)
	deps, _ := e.listRef(x.From)
	var ks []string
	for i := range deps {
		ks = append(ks, fmt.Sprintf("%s/%d", id, i))
	}
	r, got, ok := e.probeResult(id, "dependencies_reachable", res, ks)
	if !ok {
		return r
	}
	r.Expected = fmt.Sprintf("%d dependencias declaradas", len(deps))
	if len(deps) == 0 {
		return e.finishProbe(r, Pass, "sin dependencias externas declaradas")
	}
	var bad []string
	for i, g := range got {
		if ok, _, msg := e.expectOK(x.Expect, g, deps[i]); !ok {
			u, _ := Fill(x.Request.URL, deps[i])
			bad = append(bad, u+": "+msg)
		}
	}
	if bad != nil {
		return e.finishProbe(r, Fail, strings.Join(bad, "; "))
	}
	return e.finishProbe(r, Pass, fmt.Sprintf("%d dependencias disponibles", len(deps)))
}

type witnessItem struct {
	key  string
	item map[string]any
}

func (e *Engine) witnesses(sources []string, typeField, only string) ([]witnessItem, []map[string]any, error) {
	var items []witnessItem
	var all []map[string]any
	for _, src := range sources {
		l, err := e.listRef(src)
		if err != nil {
			return nil, nil, err
		}
		for _, w := range l {
			all = append(all, w)
			t, _ := w[typeField].(string)
			if _, known := e.C.WitnessTypes[t]; !known || (only != "" && t != only) {
				continue
			}
			items = append(items, witnessItem{item: w})
		}
	}
	return items, all, nil
}

func (e *Engine) witnessOps(prefix string, items []witnessItem, typeField string) ([]ProbeOp, error) {
	var ops []ProbeOp
	for i := range items {
		items[i].key = fmt.Sprintf("%s/%d", prefix, i)
		t := items[i].item[typeField].(string)
		op, err := e.httpOp(items[i].key, e.C.WitnessTypes[t].Request, items[i].item)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	return ops, nil
}

func witnessFetchOps(e *Engine, id string, p any) ([]ProbeOp, error) {
	x := p.(*WitnessFetchParams)
	items, _, err := e.witnesses(x.Sources, "type", x.Type)
	if err != nil {
		return nil, err
	}
	return e.witnessOps(id, items, "type")
}

func evalWitnessFetch(e *Engine, id string, p any, res []ProbeResult) Result {
	x := p.(*WitnessFetchParams)
	items, _, err := e.witnesses(x.Sources, "type", x.Type)
	if err != nil {
		r := e.start(id, "witness_fetch", "post")
		return e.done(r, Err, err.Error())
	}
	var ks []string
	for i := range items {
		ks = append(ks, fmt.Sprintf("%s/%d", id, i))
	}
	r, got, ok := e.probeResult(id, "witness_fetch", res, ks)
	if !ok {
		return r
	}
	r.Expected = fmt.Sprintf("al menos %d testigos %s", x.MinCount, x.Type)
	var bad, errs []string
	for i, g := range got {
		pass, measured, msg := e.expectOK(e.C.WitnessTypes[x.Type].Expect, g, items[i].item)
		wid := fmt.Sprint(items[i].item["id"])
		if !pass && measured {
			bad = append(bad, wid+": "+msg)
		} else if !pass {
			errs = append(errs, wid+": "+msg)
		}
	}
	r.Observed = fmt.Sprintf("%d testigos %s, %d distintos, %d sin verificar", len(items), x.Type, len(bad), len(errs))
	switch {
	case len(items) < x.MinCount:
		return e.finishProbe(r, Fail, fmt.Sprintf("%d testigos %s, se esperaban al menos %d", len(items), x.Type, x.MinCount))
	case bad != nil:
		return e.finishProbe(r, Fail, strings.Join(append(bad, errs...), "; "))
	case errs != nil:
		return e.finishProbe(r, Err, "no se pudo verificar: "+strings.Join(errs, "; "))
	}
	return e.finishProbe(r, Pass, fmt.Sprintf("%d testigos %s recuperados", len(items), x.Type))
}

func rpoOps(e *Engine, id string, p any) ([]ProbeOp, error) {
	x := p.(*RPOParams)
	items, _, err := e.witnesses(x.Sources, x.WitnessType, "")
	if err != nil {
		return nil, err
	}
	return e.witnessOps(id, items, x.WitnessType)
}

func parseTime(v any) (time.Time, error) {
	s, _ := v.(string)
	return time.Parse(time.RFC3339Nano, s)
}

func evalRPO(e *Engine, id string, p any, res []ProbeResult) Result {
	x := p.(*RPOParams)
	r0 := e.start(id, "rpo", "post")
	limitS, err := e.V.Resolve(x.Objective)
	limit, perr := strconv.ParseFloat(limitS, 64)
	if err != nil || perr != nil {
		return e.done(r0, Err, "objetivo RPO ilegible")
	}
	incAny, err := e.V.ResolveAny(x.Incident)
	inc, terr := parseTime(incAny)
	if err != nil || terr != nil {
		return e.done(r0, Err, "hora del incidente ilegible")
	}
	items, all, err := e.witnesses(x.Sources, x.WitnessType, "")
	if err != nil {
		return e.done(r0, Err, err.Error())
	}
	var ks []string
	for i := range items {
		ks = append(ks, fmt.Sprintf("%s/%d", id, i))
	}
	r, got, ok := e.probeResult(id, "rpo", res, ks)
	if !ok {
		return r
	}
	r.Expected = fmt.Sprintf("≤ %.0f s", limit)
	var newest map[string]any
	var newestT time.Time
	recovered, unmeasured := 0, 0
	for i, g := range got {
		t := items[i].item[x.WitnessType].(string)
		pass, measured, _ := e.expectOK(e.C.WitnessTypes[t].Expect, g, items[i].item)
		if !measured {
			unmeasured++
		}
		if !pass {
			continue
		}
		recovered++
		ct, err := parseTime(items[i].item[x.CreatedAt])
		if err == nil && ct.After(newestT) {
			newest, newestT = items[i].item, ct
		}
	}
	if newest == nil {
		if unmeasured == len(got) && len(got) > 0 {
			return e.finishProbe(r, Err, "no se pudo verificar ningún testigo")
		}
		return e.finishProbe(r, Fail, "ningún testigo recuperado")
	}
	secs := inc.Sub(newestT).Seconds()
	r.Observed = fmt.Sprintf("%.1f s", secs)
	msg := fmt.Sprintf("%.1f s (T %s; testigo más reciente recuperado %v %s; %d/%d recuperados); RPO %.0f s",
		secs, inc.UTC().Format(time.RFC3339Nano), newest[x.WitnessType], newestT.UTC().Format(time.RFC3339Nano), recovered, len(all), limit)
	if secs > limit {
		return e.finishProbe(r, Fail, msg)
	}
	return e.finishProbe(r, Pass, msg)
}

// evalRTO: time from the start of the restore to the end of the required checks,
// all of which must have passed.
func evalRTO(e *Engine, id string, p any, done map[string]Result) Result {
	x := p.(*RTOParams)
	r := e.start(id, "rto", "post")
	limitS, err := e.V.Resolve(x.Objective)
	limit, perr := strconv.ParseFloat(limitS, 64)
	if err != nil || perr != nil {
		return e.done(r, Err, "objetivo RTO ilegible")
	}
	r.Expected = fmt.Sprintf("≤ %.0f s", limit)
	startAny, err := e.V.ResolveAny(x.RestoreStarted)
	start, terr := parseTime(startAny)
	if err != nil || terr != nil {
		return e.done(r, Err, "inicio de la restauración ilegible: "+x.RestoreStarted)
	}
	if x.RestoreStatus != "" {
		st, err := e.V.Resolve(x.RestoreStatus)
		if err != nil || st != x.RestoreStatusOK {
			return e.done(r, Fail, fmt.Sprintf("la restauración no terminó bien (%s)", st))
		}
	}
	var end time.Time
	var notOK []string
	for _, rid := range x.Requires {
		d, ok := done[rid]
		if !ok || d.Status != Pass {
			notOK = append(notOK, rid)
			continue
		}
		if d.FinishedAt.After(end) {
			end = d.FinishedAt
		}
	}
	if notOK != nil {
		return e.done(r, Fail, fmt.Sprintf("verificaciones requeridas que no pasaron: %v", notOK))
	}
	secs := end.Sub(start).Seconds()
	r.Observed = fmt.Sprintf("%.1f s", secs)
	msg := fmt.Sprintf("%.1f s desde el inicio de la restauración hasta el fin de %d verificaciones requeridas; RTO %.0f s", secs, len(x.Requires), limit)
	if secs > limit {
		return e.done(r, Fail, msg)
	}
	return e.done(r, Pass, msg)
}
