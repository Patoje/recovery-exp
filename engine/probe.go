package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ProbeOp is one network operation run by the probe inside the restored network.
// Requests travel through stdin, so credentials never appear in a command line.
type ProbeOp struct {
	Key  string `json:"key"`
	Kind string `json:"kind"` // http | git_push

	Method          string `json:"method,omitempty"`
	URL             string `json:"url,omitempty"`
	User            string `json:"user,omitempty"`
	Password        string `json:"password,omitempty"`
	FollowJSONField string `json:"follow_json_field,omitempty"`
	TOTP            *TOTP  `json:"totp,omitempty"`

	// git_push
	FilePrefix string            `json:"file_prefix,omitempty"`
	Author     string            `json:"author,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
}

type TOTP struct {
	Secret string `json:"secret"`
	Header string `json:"header"`
	Digits int    `json:"digits"`
	Period int    `json:"period"`
}

type ProbeResult struct {
	Key        string         `json:"key"`
	Status     int            `json:"status"`
	BodySHA256 string         `json:"body_sha256,omitempty"`
	JSON       map[string]any `json:"json,omitempty"`
	Error      string         `json:"error,omitempty"`
	Detail     string         `json:"detail,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt time.Time      `json:"finished_at"`
}

// runProbe is the `engine probe` subcommand: ops in (stdin), results out (stdout).
func runProbe(in io.Reader, out io.Writer) error {
	var ops []ProbeOp
	if err := json.NewDecoder(in).Decode(&ops); err != nil {
		return err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	res := make([]ProbeResult, 0, len(ops))
	for _, op := range ops {
		r := ProbeResult{Key: op.Key, StartedAt: time.Now().UTC().Truncate(time.Millisecond)}
		switch op.Kind {
		case "http":
			doHTTP(client, op, &r)
		case "git_push":
			doGitPush(op, &r)
		default:
			r.Error = "operación desconocida " + op.Kind
		}
		r.FinishedAt = time.Now().UTC().Truncate(time.Millisecond)
		res = append(res, r)
	}
	return json.NewEncoder(out).Encode(res)
}

func doHTTP(c *http.Client, op ProbeOp, r *ProbeResult) {
	body, status, err := httpOnce(c, op, op.URL)
	if err != nil {
		r.Error = err.Error()
		return
	}
	r.Status = status
	var obj map[string]any
	_ = json.Unmarshal(body, &obj)
	if op.FollowJSONField != "" {
		next, _ := obj[op.FollowJSONField].(string)
		if status/100 != 2 || next == "" {
			r.Detail = fmt.Sprintf("sin %s en la primera respuesta (HTTP %d)", op.FollowJSONField, status)
			return
		}
		body, status, err = httpOnce(c, op, next)
		if err != nil {
			r.Error = err.Error()
			return
		}
		r.Status = status
		obj = nil
		_ = json.Unmarshal(body, &obj)
	}
	sum := sha256.Sum256(body)
	r.BodySHA256 = hex.EncodeToString(sum[:])
	r.JSON = obj
}

func httpOnce(c *http.Client, op ProbeOp, url string) ([]byte, int, error) {
	m := op.Method
	if m == "" {
		m = http.MethodGet
	}
	req, err := http.NewRequest(m, url, nil)
	if err != nil {
		return nil, 0, err
	}
	if op.User != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(op.User+":"+op.Password)))
	}
	if op.TOTP != nil {
		code, err := totpCode(op.TOTP.Secret, time.Now(), op.TOTP.Digits, op.TOTP.Period)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set(op.TOTP.Header, code)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return b, resp.StatusCode, err
}

// totpCode implements RFC 6238 with HMAC-SHA1.
func totpCode(secretB32 string, at time.Time, digits, period int) (string, error) {
	if digits == 0 {
		digits = 6
	}
	if period == 0 {
		period = 30
	}
	s := strings.ToUpper(strings.TrimSpace(secretB32))
	if pad := len(s) % 8; pad != 0 {
		s += strings.Repeat("=", 8-pad)
	}
	key, err := base32.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("secreto TOTP inválido")
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(at.Unix()/int64(period)))
	h := hmac.New(sha1.New, key)
	h.Write(msg[:])
	d := h.Sum(nil)
	o := d[len(d)-1] & 0x0f
	v := binary.BigEndian.Uint32(d[o:o+4]) & 0x7fffffff
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, v%mod), nil
}

// doGitPush clones, commits one new file and pushes; then checks the remote ref.
func doGitPush(op ProbeOp, r *ProbeResult) {
	wd, err := os.MkdirTemp("", "probe-push-")
	if err != nil {
		r.Error = err.Error()
		return
	}
	defer os.RemoveAll(wd)
	url, err := withCredentials(op.URL, op.User, op.Password)
	if err != nil {
		r.Error = err.Error()
		return
	}
	author := op.Author
	if author == "" {
		author = "contract-engine"
	}
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME="+author, "GIT_AUTHOR_EMAIL="+author+"@example.test",
		"GIT_COMMITTER_NAME="+author, "GIT_COMMITTER_EMAIL="+author+"@example.test")
	for k, v := range op.Env {
		env = append(env, k+"="+v)
	}
	git := func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = wd, env
		b, err := cmd.CombinedOutput()
		return redact(string(b), op.Password), err
	}
	if out, err := git("clone", "-q", url, "."); err != nil {
		r.Error = "clone: " + lastLine([]byte(out))
		return
	}
	name := fmt.Sprintf("%s%d.txt", op.FilePrefix, time.Now().UnixNano())
	if err := os.WriteFile(filepath.Join(wd, name), []byte("push check\n"), 0o644); err != nil {
		r.Error = err.Error()
		return
	}
	for _, a := range [][]string{{"add", name}, {"commit", "-q", "-m", "push check"}} {
		if out, err := git(a...); err != nil {
			r.Error = a[0] + ": " + lastLine([]byte(out))
			return
		}
	}
	head, _ := git("rev-parse", "HEAD")
	head = strings.TrimSpace(head)
	if out, err := git("push", "-q", "origin", "HEAD"); err != nil {
		r.Error = "push: " + lastLine([]byte(out))
		return
	}
	branch, _ := git("rev-parse", "--abbrev-ref", "HEAD")
	remote, err := git("ls-remote", "origin", "refs/heads/"+strings.TrimSpace(branch))
	if err != nil || !strings.HasPrefix(remote, head) {
		r.Error = "ls-remote no muestra el commit empujado"
		return
	}
	r.Status = 200
	r.Detail = fmt.Sprintf("commit %s en refs/heads/%s", head[:12], strings.TrimSpace(branch))
}

func withCredentials(url, user, password string) (string, error) {
	scheme, rest, ok := strings.Cut(url, "://")
	if !ok {
		return "", fmt.Errorf("URL sin esquema")
	}
	if user == "" {
		return url, nil
	}
	return scheme + "://" + urlEscape(user) + ":" + urlEscape(password) + "@" + rest, nil
}

func urlEscape(s string) string {
	r := strings.NewReplacer("%", "%25", "@", "%40", ":", "%3A", "/", "%2F", "#", "%23", "?", "%3F")
	return r.Replace(s)
}

func redact(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, urlEscape(secret), "***"), secret, "***")
}

// ProbeRunner runs a batch of ops in the probe service.
type ProbeRunner interface {
	Run(ops []ProbeOp) ([]ProbeResult, error)
}

type composeProbe struct {
	d       Docker
	probe   Probe
	binPath string // engine binary for the probe's platform, on the host
}

func (p *composeProbe) Run(ops []ProbeOp) ([]ProbeResult, error) {
	in, _ := json.Marshal(ops)
	out, errb, err := p.d.Compose(p.probe.Profiles, in, "run", "--rm", "--no-deps", "-T",
		"-v", p.binPath+":/opt/contract-engine-probe:ro", "--entrypoint", "/opt/contract-engine-probe",
		p.probe.Service, "probe")
	if err != nil {
		return nil, fmt.Errorf("sonda: %v: %s", err, lastLine(errb))
	}
	var res []ProbeResult
	if err := json.Unmarshal(lastJSONLine(out), &res); err != nil {
		return nil, fmt.Errorf("sonda: respuesta inválida: %w", err)
	}
	return res, nil
}

func lastJSONLine(b []byte) []byte {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.HasPrefix(lines[i], "[") {
			return []byte(lines[i])
		}
	}
	return b
}
