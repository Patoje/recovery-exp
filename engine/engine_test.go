package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------- fakes ----------

type fakeDocker struct {
	// handler receives the full argument list (compose or docker) and stdin.
	handler func(args []string, stdin []byte) (string, error)
}

func (f *fakeDocker) Compose(profiles []string, stdin []byte, args ...string) ([]byte, []byte, error) {
	out, err := f.handler(append([]string{"compose"}, args...), stdin)
	if err != nil {
		return []byte(out), []byte(err.Error()), err
	}
	return []byte(out), nil, nil
}

func (f *fakeDocker) Docker(args ...string) ([]byte, []byte, error) {
	out, err := f.handler(args, nil)
	if err != nil {
		return []byte(out), []byte(err.Error()), err
	}
	return []byte(out), nil, nil
}

type fakeProbe struct {
	fn  func(ops []ProbeOp) []ProbeResult
	ops []ProbeOp
}

func (f *fakeProbe) Run(ops []ProbeOp) ([]ProbeResult, error) {
	f.ops = ops
	return f.fn(ops), nil
}

func has(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

const baseContract = `
version: 1
name: demo
environment:
  compose_file: compose.yml
  probe: {service: probe}
components:
  - {name: db, service: dbsvc}
  - {name: app, service: appsvc}
witness_types:
  blob:
    request: {url: "http://app/blob/{id}"}
    expect: {status: 200, sha256_of_body: "{sha256}"}
checks:
`

func newEngine(t *testing.T, checks string, d Docker, p ProbeRunner) *Engine {
	t.Helper()
	c, err := ParseContract([]byte(baseContract + checks))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	v := &Values{RunDir: dir, Env: map[string]string{"PW": "s3cret", "U": "u", "DBU": "dbu", "DBN": "dbn"},
		Objectives: map[string]string{"RTO": "120", "RPO": "600"}, cache: map[string]any{}}
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return &Engine{C: c, V: v, D: d, P: p, RunDir: dir, Now: func() time.Time { clock = clock.Add(time.Second); return clock }}
}

func writeArtifact(t *testing.T, e *Engine, name string, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := os.WriteFile(filepath.Join(e.RunDir, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func only(t *testing.T, e *Engine, phase, id string) Result {
	t.Helper()
	for _, r := range e.RunPhase(phase).Results {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("sin resultado para %s", id)
	return Result{}
}

// ---------- schema: closed vocabulary ----------

func TestSchemaRejectsUnknownVerifierFieldAndParam(t *testing.T) {
	cases := map[string]string{
		"verificador desconocido": "  - {id: a, verifier: run, with: {script: x}}\n",
		"parámetro desconocido":   "  - {id: a, verifier: start_order, with: {script: x}}\n",
		"campo desconocido":       "  - {id: a, verifier: start_order, run: x}\n",
		"id repetido":             "  - {id: a, verifier: start_order}\n  - {id: a, verifier: start_order}\n",
		"componente no declarado": "  - {id: a, verifier: component_image, with: {component: x, source: running, expected: y}}\n",
		"requires inexistente":    "  - {id: a, verifier: rto, with: {objective: '${objective:RTO}', restore_started: '${run:r.json#t}', requires: [nope]}}\n",
	}
	for name, checks := range cases {
		if _, err := ParseContract([]byte(baseContract + checks)); err == nil {
			t.Errorf("%s: el contrato debía rechazarse", name)
		}
	}
	if _, err := ParseContract([]byte(baseContract + "top_level_extra: 1\n")); err == nil {
		t.Error("campo de primer nivel desconocido aceptado")
	}
}

// ---------- values ----------

func TestValuesAndTemplates(t *testing.T) {
	e := newEngine(t, "  - {id: a, verifier: start_order}\n", nil, nil)
	writeArtifact(t, e, "w.json", map[string]any{"list": []any{map[string]any{"type": "x", "v": "1"}, map[string]any{"type": "y", "v": "2"}}})
	got, err := e.V.Resolve("${run:w.json#list[type=y].v}-${env:U}")
	if err != nil || got != "2-u" {
		t.Fatalf("Resolve = %q, %v", got, err)
	}
	if _, err := e.V.Resolve("${run:../etc/passwd#x}"); err == nil {
		t.Error("artefacto fuera de la corrida aceptado")
	}
	if s, err := Fill("/a/{n}/{s}", map[string]any{"n": float64(7), "s": "z"}); err != nil || s != "/a/7/z" {
		t.Errorf("Fill = %q, %v", s, err)
	}
	if _, err := Fill("{missing}", map[string]any{}); err == nil {
		t.Error("campo faltante aceptado")
	}
}

// ---------- TOTP (RFC 6238, apéndice B, SHA1) ----------

func TestTOTPRFC6238(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // "12345678901234567890"
	vectors := map[int64]string{59: "94287082", 1111111109: "07081804", 1111111111: "14050471",
		1234567890: "89005924", 2000000000: "69279037", 20000000000: "65353130"}
	for ts, want := range vectors {
		got, err := totpCode(secret, time.Unix(ts, 0), 8, 30)
		if err != nil || got != want {
			t.Errorf("T=%d: %s, quería %s (%v)", ts, got, want, err)
		}
	}
}

// ---------- secrets ----------

func TestSecretValue(t *testing.T) {
	ini := "A = 1\n[s1]\nK = top\n[s2]\nK = \nOTHER = x\n"
	if v, ok := secretValue(ini, "ini", "s1", "K"); !ok || v != "top" {
		t.Errorf("ini s1: %q %v", v, ok)
	}
	if v, ok := secretValue(ini, "ini", "s2", "K"); !ok || v != "" {
		t.Errorf("ini vacía: %q %v", v, ok)
	}
	if _, ok := secretValue(ini, "ini", "s3", "K"); ok {
		t.Error("clave ausente encontrada")
	}
	if v, ok := secretValue("export K=\"v\"\n", "env", "", "K"); !ok || v != "v" {
		t.Errorf("env: %q %v", v, ok)
	}
	if v, _ := secretValue("abc\n", "raw", "", ""); v != "abc" {
		t.Errorf("raw: %q", v)
	}
}

func TestSecretFingerprintNeverReportsValue(t *testing.T) {
	d := &fakeDocker{handler: func(a []string, _ []byte) (string, error) { return "[sec]\nKEY = hunter2\n", nil }}
	check := "  - {id: k, verifier: secret_fingerprint, with: {component: app, file: /c.ini, format: ini, section: sec, key: KEY, expected: '${run:d.json#fp}'}}\n"
	e := newEngine(t, check, d, nil)
	writeArtifact(t, e, "d.json", map[string]any{"fp": sha256hex([]byte("hunter2"))})
	r := only(t, e, "post", "k")
	if r.Status != Pass {
		t.Fatalf("%s: %s", r.Status, r.Message)
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "hunter2") {
		t.Fatal("el valor del secreto aparece en el resultado")
	}
	writeArtifact(t, e, "d.json", map[string]any{"fp": sha256hex([]byte(""))})
	e.V.cache = map[string]any{}
	if r := only(t, e, "post", "k"); r.Status != Fail {
		t.Fatalf("huella distinta: %s", r.Status)
	}
}

// ---------- log patterns ----------

const logCheck = `  - id: logs
    verifier: log_patterns
    with:
      files: [restore.log]
      patterns: ['tool: error:', '^Fatal: ']
      must_match: ['tool: error: boom', 'Fatal: broken']
      must_not_match: ['no errors were found']
`

func TestLogPatterns(t *testing.T) {
	e := newEngine(t, logCheck, nil, nil)
	write := func(s string) { _ = os.WriteFile(filepath.Join(e.RunDir, "restore.log"), []byte(s), 0o644) }
	if r := only(t, e, "post", "logs"); r.Status != Err {
		t.Errorf("log ausente: %s", r.Status)
	}
	write("check ok\nno errors were found\n")
	r := only(t, e, "post", "logs")
	if r.Status != Pass || !strings.Contains(r.Message, `"no errors were found" ×1`) {
		t.Errorf("'no errors were found' no debe contar: %s %s", r.Status, r.Message)
	}
	write("no errors were found\ntool: error: COPY failed\n")
	if r := only(t, e, "post", "logs"); r.Status != Fail || !strings.Contains(r.Message, "restore.log:2") {
		t.Errorf("error no detectado: %s %s", r.Status, r.Message)
	}
	bad := strings.Replace(logCheck, "'^Fatal: '", "'error'", 1)
	e2 := newEngine(t, bad, nil, nil)
	if r := only(t, e2, "post", "logs"); r.Status != Fail || !strings.Contains(r.Message, "autoprueba") {
		t.Errorf("la autoprueba debía fallar con un patrón demasiado amplio: %s %s", r.Status, r.Message)
	}
}

// ---------- disk and inventories: a read failure is an error, never "empty" ----------

const entitiesCheck = `  - id: ent
    verifier: entities_match_disk
    with:
      database: {component: db, client: psql, user: '${env:DBU}', name: '${env:DBN}'}
      query: select name from t
      disk: {component: app, root: /store, glob: '*/*.d', strip_suffix: .d}
`

func TestEntitiesMatchDisk(t *testing.T) {
	disk := "## a/x.d\n## b/y.d\n"
	var diskErr error
	d := &fakeDocker{handler: func(a []string, _ []byte) (string, error) {
		if has(a, "dbsvc") {
			return "a/x\nb/y\n", nil
		}
		return disk, diskErr
	}}
	e := newEngine(t, entitiesCheck, d, nil)
	if r := only(t, e, "post", "ent"); r.Status != Pass {
		t.Fatalf("iguales: %s %s", r.Status, r.Message)
	}
	disk = "## a/x.d\n"
	if r := only(t, e, "post", "ent"); r.Status != Fail || !strings.Contains(r.Message, "solo en base [b/y]") {
		t.Fatalf("faltante: %s %s", r.Status, r.Message)
	}
	disk, diskErr = "", errors.New(`service "appsvc" is not running`)
	r := only(t, e, "post", "ent")
	if r.Status != Err || strings.Contains(r.Message, "0 en disco") {
		t.Fatalf("lectura fallida debe ser error, no vacío: %s %s", r.Status, r.Message)
	}
}

func TestGitRefsMatchInventory(t *testing.T) {
	listing := "## o/r.git\nrefs/heads/main aaa\nrefs/heads/f bbb\n"
	var listErr error
	d := &fakeDocker{handler: func(a []string, _ []byte) (string, error) { return listing, listErr }}
	check := "  - {id: refs, verifier: git_refs_match_inventory, with: {inventory: '${run:inv.json#refs}', disk: {component: app, root: /r, glob: '*/*.git', strip_suffix: .git}}}\n"
	e := newEngine(t, check, d, nil)
	writeArtifact(t, e, "inv.json", map[string]any{"refs": map[string]any{"o/r": map[string]any{"refs/heads/main": "aaa", "refs/heads/f": "bbb"}}})
	if r := only(t, e, "post", "refs"); r.Status != Pass {
		t.Fatalf("%s %s", r.Status, r.Message)
	}
	listing = "## o/r.git\nrefs/heads/main aaa\n"
	if r := only(t, e, "post", "refs"); r.Status != Fail || !strings.Contains(r.Message, "refs/heads/f") {
		t.Fatalf("%s %s", r.Status, r.Message)
	}
	listing, listErr = "", errors.New("exit 3")
	if r := only(t, e, "post", "refs"); r.Status != Err {
		t.Fatalf("lectura fallida: %s", r.Status)
	}
}

func TestTableCounts(t *testing.T) {
	counts := `{"a": 1, "b": 2}`
	var sql []byte
	d := &fakeDocker{handler: func(a []string, stdin []byte) (string, error) { sql = stdin; return counts, nil }}
	check := "  - {id: rows, verifier: table_counts_match_inventory, with: {database: {component: db, client: psql, user: '${env:DBU}', name: '${env:DBN}'}, schema: public, inventory: '${run:inv.json#counts}', ignore: [c]}}\n"
	e := newEngine(t, check, d, nil)
	writeArtifact(t, e, "inv.json", map[string]any{"counts": map[string]any{"a": 1, "b": 2, "c": 9}})
	if r := only(t, e, "post", "rows"); r.Status != Pass {
		t.Fatalf("%s %s", r.Status, r.Message)
	}
	if !strings.Contains(string(sql), ":'schema'") {
		t.Error("el esquema debe pasar como variable de psql")
	}
	counts = `{"a": 1, "b": 3}`
	if r := only(t, e, "post", "rows"); r.Status != Fail || !strings.Contains(r.Message, "b: 2→3") {
		t.Fatalf("%s %s", r.Status, r.Message)
	}
}

// ---------- components ----------

func TestComponentImageAndOrder(t *testing.T) {
	started := map[string]string{"dbsvc": "2026-01-01T00:00:01Z", "appsvc": "2026-01-01T00:00:05Z"}
	d := &fakeDocker{handler: func(a []string, _ []byte) (string, error) {
		switch {
		case has(a, "config"):
			return `{"services": {"appsvc": {"image": "img@sha256:1"}}}`, nil
		case has(a, "ps"):
			return "id-" + a[len(a)-1] + "\n", nil
		case a[0] == "inspect":
			svc := strings.TrimPrefix(a[len(a)-1], "id-")
			return "img@sha256:1|running|healthy|" + started[svc] + "\n", nil
		}
		return "", errors.New("inesperado")
	}}
	checks := `  - {id: img_cfg, verifier: component_image, with: {component: app, source: configured, expected: 'img@sha256:1'}}
  - {id: img_run, verifier: component_image, with: {component: app, source: running, expected: 'img@sha256:2'}}
  - {id: order, verifier: start_order}
  - {id: up, verifier: components_running, with: {require_healthy: true}}
`
	e := newEngine(t, checks, d, nil)
	if r := only(t, e, "pre", "img_cfg"); r.Status != Pass || r.Phase != "pre" {
		t.Errorf("configurada: %s %s", r.Status, r.Phase)
	}
	if r := only(t, e, "post", "img_run"); r.Status != Fail {
		t.Errorf("en marcha distinta: %s", r.Status)
	}
	if r := only(t, e, "post", "order"); r.Status != Pass {
		t.Errorf("orden: %s %s", r.Status, r.Message)
	}
	if r := only(t, e, "post", "up"); r.Status != Pass {
		t.Errorf("en marcha: %s %s", r.Status, r.Message)
	}
	started["appsvc"] = "2026-01-01T00:00:00Z"
	if r := only(t, e, "post", "order"); r.Status != Fail {
		t.Errorf("orden invertido: %s", r.Status)
	}
}

// ---------- probe operations ----------

func TestProbeHTTPFollowTOTPAndSHA(t *testing.T) {
	var otp string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("X-OTP"); h != "" {
			otp = h
		}
		if u, p, ok := r.BasicAuth(); !ok || u != "u" || p != "pw" {
			w.WriteHeader(401)
			return
		}
		if r.URL.Path == "/meta" {
			_ = json.NewEncoder(w).Encode(map[string]string{"next": "http://" + r.Host + "/file"})
			return
		}
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()
	ops := []ProbeOp{
		{Key: "f", Kind: "http", URL: srv.URL + "/meta", User: "u", Password: "pw", FollowJSONField: "next",
			TOTP: &TOTP{Secret: "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", Header: "X-OTP"}},
		{Key: "noauth", Kind: "http", URL: srv.URL + "/meta"},
	}
	in, _ := json.Marshal(ops)
	var out strings.Builder
	if err := runProbe(strings.NewReader(string(in)), &out); err != nil {
		t.Fatal(err)
	}
	var res []ProbeResult
	_ = json.Unmarshal([]byte(out.String()), &res)
	if res[0].Status != 200 || res[0].BodySHA256 != sha256hex([]byte("content")) || len(otp) != 6 {
		t.Errorf("follow/sha/totp: %+v otp=%q", res[0], otp)
	}
	if res[1].Status != 401 {
		t.Errorf("sin auth: %d", res[1].Status)
	}
}

func TestProbeGitPush(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git no está en esta imagen; la prueba corre con go test en el host")
	}
	dir := t.TempDir()
	bare := filepath.Join(dir, "r.git")
	work := filepath.Join(dir, "w")
	for _, a := range [][]string{{"init", "-q", "--bare", "-b", "main", bare}, {"clone", "-q", bare, work}} {
		if b, err := exec.Command("git", a...).CombinedOutput(); err != nil {
			t.Fatal(string(b))
		}
	}
	g := exec.Command("git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")
	g.Dir = work
	if b, err := g.CombinedOutput(); err != nil {
		t.Fatal(string(b))
	}
	p := exec.Command("git", "push", "-q", "origin", "HEAD:main")
	p.Dir = work
	if b, err := p.CombinedOutput(); err != nil {
		t.Fatal(string(b))
	}
	var r ProbeResult
	doGitPush(ProbeOp{Key: "p", Kind: "git_push", URL: "file://" + bare, FilePrefix: "check-"}, &r)
	if r.Error != "" || r.Status != 200 {
		t.Fatalf("push: %+v", r)
	}
	var r2 ProbeResult
	doGitPush(ProbeOp{Key: "p", Kind: "git_push", URL: "file://" + filepath.Join(dir, "missing.git")}, &r2)
	if !strings.HasPrefix(r2.Error, "clone:") {
		t.Fatalf("clone fallido: %+v", r2)
	}
}

// ---------- witnesses, RPO and RTO ----------

func TestWitnessFetchRPOAndRTO(t *testing.T) {
	checks := `  - {id: blobs, verifier: witness_fetch, with: {type: blob, sources: ['${run:pre.json#w}']}}
  - {id: rpo, verifier: rpo, with: {objective: '${objective:RPO}', incident: '${run:post.json#t}', sources: ['${run:pre.json#w}', '${run:post.json#w}']}}
  - {id: rto, verifier: rto, with: {objective: '${objective:RTO}', restore_started: '${run:restore.json#started}', restore_status: '${run:restore.json#status}', restore_status_ok: ok, requires: [blobs]}}
`
	good := map[string]string{}
	p := &fakeProbe{fn: func(ops []ProbeOp) []ProbeResult {
		var out []ProbeResult
		at := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
		for _, o := range ops {
			r := ProbeResult{Key: o.Key, Status: 200, StartedAt: at, FinishedAt: at.Add(time.Second)}
			id := o.URL[strings.LastIndex(o.URL, "/")+1:]
			if s, ok := good[id]; ok {
				r.BodySHA256 = s
			} else {
				r.Status = 404
			}
			out = append(out, r)
		}
		return out
	}}
	e := newEngine(t, checks, nil, p)
	w := func(id, created string) map[string]any {
		return map[string]any{"type": "blob", "id": id, "sha256": sha256hex([]byte(id)), "created_at": created}
	}
	writeArtifact(t, e, "pre.json", map[string]any{"w": []any{w("p1", "2026-01-01T00:00:00Z")}})
	writeArtifact(t, e, "post.json", map[string]any{"t": "2026-01-01T00:20:00Z",
		"w": []any{w("q1", "2026-01-01T00:12:00Z"), w("q2", "2026-01-01T00:15:00Z"), map[string]any{"type": "other", "id": "z"}}})
	writeArtifact(t, e, "restore.json", map[string]any{"started": "2026-01-01T00:00:30Z", "status": "ok"})
	good["p1"], good["q1"] = sha256hex([]byte("p1")), sha256hex([]byte("q1"))
	res := map[string]Result{}
	for _, r := range e.RunPhase("post").Results {
		res[r.ID] = r
	}
	if res["blobs"].Status != Pass {
		t.Errorf("witness_fetch: %s %s", res["blobs"].Status, res["blobs"].Message)
	}
	if res["rpo"].Status != Pass || !strings.Contains(res["rpo"].Message, "480.0 s") || !strings.Contains(res["rpo"].Message, "2/4") {
		t.Errorf("rpo (T - q1 = 8 min): %s %s", res["rpo"].Status, res["rpo"].Message)
	}
	if res["rto"].Status != Pass || !strings.Contains(res["rto"].Message, "31.0 s") {
		t.Errorf("rto: %s %s", res["rto"].Status, res["rto"].Message)
	}
	delete(good, "p1")
	res = map[string]Result{}
	for _, r := range e.RunPhase("post").Results {
		res[r.ID] = r
	}
	if res["blobs"].Status != Fail || res["rto"].Status != Fail {
		t.Errorf("sin testigo: blobs %s, rto %s", res["blobs"].Status, res["rto"].Status)
	}
	writeArtifact(t, e, "post.json", map[string]any{"t": "2026-01-01T00:40:00Z", "w": []any{}})
	e.V.cache = map[string]any{}
	good["p1"] = sha256hex([]byte("p1"))
	if r := only(t, e, "post", "rpo"); r.Status != Fail {
		t.Errorf("rpo 40 min > 10 min: %s %s", r.Status, r.Message)
	}
}

func TestProbeTransportErrorIsError(t *testing.T) {
	p := &fakeProbe{fn: func(ops []ProbeOp) []ProbeResult {
		var out []ProbeResult
		for _, o := range ops {
			out = append(out, ProbeResult{Key: o.Key, Error: "dial tcp: lookup app: no such host"})
		}
		return out
	}}
	e := newEngine(t, "  - {id: h, verifier: http_request, with: {request: {url: 'http://app/'}, expect: {status: 200}}}\n", nil, p)
	if r := only(t, e, "post", "h"); r.Status != Err {
		t.Errorf("sin conexión: %s", r.Status)
	}
}

func TestLoginTOTPSendsSecretOnlyToProbe(t *testing.T) {
	p := &fakeProbe{fn: func(ops []ProbeOp) []ProbeResult {
		return []ProbeResult{{Key: ops[0].Key, Status: 200, JSON: map[string]any{"login": "u"}}}
	}}
	check := "  - {id: totp, verifier: login_totp, with: {request: {url: 'http://app/me', auth: {user: '${env:U}', password: '${env:PW}'}}, totp: {secret: '${run:s.json#k}', header: X-OTP}, expect: {status: 200, json: {login: '${env:U}'}}}}\n"
	e := newEngine(t, check, nil, p)
	writeArtifact(t, e, "s.json", map[string]any{"k": "GEZDGNBVGY3TQOJQ"})
	r := only(t, e, "post", "totp")
	if r.Status != Pass {
		t.Fatalf("%s %s", r.Status, r.Message)
	}
	if p.ops[0].TOTP == nil || p.ops[0].TOTP.Secret != "GEZDGNBVGY3TQOJQ" || p.ops[0].Password != "s3cret" {
		t.Error("la sonda no recibió las credenciales")
	}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "s3cret") || strings.Contains(string(b), "GEZDGNBVGY3TQOJQ") {
		t.Error("credenciales en el resultado")
	}
}

func TestDependenciesEmptyListPasses(t *testing.T) {
	p := &fakeProbe{fn: func(ops []ProbeOp) []ProbeResult { return nil }}
	e := newEngine(t, "  - {id: deps, verifier: dependencies_reachable, with: {from: '${run:d.json#deps}', request: {url: 'http://{endpoint}/h'}}}\n", nil, p)
	writeArtifact(t, e, "d.json", map[string]any{"deps": []any{}})
	if r := only(t, e, "post", "deps"); r.Status != Pass {
		t.Errorf("%s %s", r.Status, r.Message)
	}
}

// ---------- report ----------

func TestReportCountsCustomCheckAndVerdict(t *testing.T) {
	d := &fakeDocker{handler: func(a []string, _ []byte) (string, error) { return "ok", nil }}
	checks := `  - {id: escape, verifier: custom_check, with: {component: app, command: [true], reason: "no expresable"}}
  - {id: order, verifier: start_order}
`
	e := newEngine(t, checks, d, nil)
	post := e.RunPhase("post")
	rep := buildReport(e, "c.yaml", []byte("x"), e.missingPhase("pre", errors.New("x")), post)
	if len(rep.CustomChecks) != 1 || rep.CustomChecks[0].ID != "escape" || rep.CustomChecks[0].Reason != "no expresable" {
		t.Errorf("custom_checks: %+v", rep.CustomChecks)
	}
	if rep.Result != "fail" || rep.FirstFailedStep != "order" || rep.ExitCode != 1 {
		t.Errorf("veredicto: %s %s %d", rep.Result, rep.FirstFailedStep, rep.ExitCode)
	}
	if err := writeMarkdown(filepath.Join(e.RunDir, "r.md"), rep); err != nil {
		t.Fatal(err)
	}
}

// ---------- principles ----------

func engineSources(t *testing.T) map[string]string {
	t.Helper()
	files, _ := filepath.Glob("*.go")
	out := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		out[f] = strings.ToLower(string(b))
	}
	if len(out) == 0 {
		t.Fatal("sin fuentes")
	}
	return out
}

// Principle 1: nothing specific to the system under test inside the engine.
func TestEngineIsGeneral(t *testing.T) {
	forbidden := []string{"gitea", "app.ini", "secret_key", "/data/", ":3000", "minio", "lfs", "x-gitea",
		"postgres:", "erin", "alice", "issue", "attachment", "repository_refs", "table_row_counts", "28.4"}
	for f, src := range engineSources(t) {
		for _, w := range forbidden {
			if strings.Contains(src, w) {
				t.Errorf("%s contiene %q", f, w)
			}
		}
	}
}

// Principle 3: the engine reads nothing from the baselines, the fault scenarios or B0.
func TestEngineIsIndependent(t *testing.T) {
	for f, src := range engineSources(t) {
		for _, w := range []string{"baselines/", "faults/", "scripts/", "b0.json", "b1.json", "fault_dir", "fault_evidence"} {
			if strings.Contains(src, w) {
				t.Errorf("%s contiene %q", f, w)
			}
		}
	}
}

// The real contract must load (closed vocabulary) and use no custom_check.
func TestGiteaContractLoads(t *testing.T) {
	path := filepath.Join("..", "contracts", "gitea.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Skip("contracts/gitea.yaml no está montado en este entorno")
	}
	c, _, err := LoadContract(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range c.Checks {
		if ch.Verifier == "custom_check" {
			t.Errorf("el contrato de Gitea usa custom_check: %s", ch.ID)
		}
	}
}
