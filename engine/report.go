package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Report is <run>/contract.json. result, exit_code and first_failed_step have the
// same meaning as in the other baselines' reports.
type Report struct {
	Baseline        string                 `json:"baseline"`
	RunID           string                 `json:"run_id"`
	Result          string                 `json:"result"`
	ExitCode        int                    `json:"exit_code"`
	FirstFailedStep string                 `json:"first_failed_step"`
	StartedAt       time.Time              `json:"started_at"`
	FinishedAt      time.Time              `json:"finished_at"`
	Contract        map[string]string      `json:"contract"`
	Engine          map[string]string      `json:"engine"`
	Objectives      map[string]string      `json:"objectives"`
	Phases          map[string]PhaseTiming `json:"phases"`
	Counts          map[string]int         `json:"counts"`
	CustomChecks    []CustomUse            `json:"custom_checks"`
	Checks          []Result               `json:"checks"`
}

type PhaseTiming struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`
}

type CustomUse struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
	Status string `json:"status"`
}

func buildReport(e *Engine, contractPath string, contractRaw []byte, phases ...PhaseRecord) Report {
	rep := Report{
		Baseline: "contract", RunID: filepath.Base(e.RunDir),
		Contract:     map[string]string{"name": e.C.Name, "path": contractPath, "sha256": sha256hex(contractRaw)},
		Engine:       map[string]string{"version": EngineVersion, "commit": engineCommit},
		Objectives:   e.V.Objectives,
		Phases:       map[string]PhaseTiming{},
		Counts:       map[string]int{Pass: 0, Fail: 0, Err: 0},
		CustomChecks: []CustomUse{},
	}
	byID := map[string]Result{}
	for _, ph := range phases {
		rep.Phases[ph.Phase] = PhaseTiming{ph.StartedAt, ph.FinishedAt, ph.DurationMS}
		if rep.StartedAt.IsZero() || (!ph.StartedAt.IsZero() && ph.StartedAt.Before(rep.StartedAt)) {
			rep.StartedAt = ph.StartedAt
		}
		if ph.FinishedAt.After(rep.FinishedAt) {
			rep.FinishedAt = ph.FinishedAt
		}
		for _, r := range ph.Results {
			byID[r.ID] = r
		}
	}
	for _, ch := range e.C.Checks {
		r, ok := byID[ch.ID]
		if !ok {
			r = e.done(e.start(ch.ID, ch.Verifier, phaseOf(ch)), Err, "no se ejecutó")
		}
		rep.Checks = append(rep.Checks, r)
		rep.Counts[r.Status]++
		if r.Custom {
			rep.CustomChecks = append(rep.CustomChecks, CustomUse{r.ID, r.Reason, r.Status})
		}
		if r.Status != Pass && rep.FirstFailedStep == "" {
			rep.FirstFailedStep = r.ID
		}
	}
	rep.Result, rep.ExitCode = "pass", 0
	if rep.FirstFailedStep != "" {
		rep.Result, rep.ExitCode = "fail", 1
	}
	return rep
}

func writeMarkdown(path string, rep Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Contrato %s — corrida %s\n\n", rep.Contract["name"], rep.RunID)
	fmt.Fprintf(&b, "**Veredicto: %s**", rep.Result)
	if rep.FirstFailedStep != "" {
		fmt.Fprintf(&b, " (primera verificación que no pasó: `%s`)", rep.FirstFailedStep)
	}
	fmt.Fprintf(&b, "\n\n- Verificaciones: %d pass, %d fail, %d error\n", rep.Counts[Pass], rep.Counts[Fail], rep.Counts[Err])
	fmt.Fprintf(&b, "- `custom_check` usados: %d", len(rep.CustomChecks))
	for _, c := range rep.CustomChecks {
		fmt.Fprintf(&b, "\n  - `%s` (%s): %s", c.ID, c.Status, c.Reason)
	}
	fmt.Fprintf(&b, "\n- Contrato: `%s` (sha256 `%s`)\n- Motor: versión %s, commit `%s`\n",
		rep.Contract["path"], truncate(rep.Contract["sha256"], 12), rep.Engine["version"], rep.Engine["commit"])
	for _, ph := range []string{"pre", "post"} {
		if t, ok := rep.Phases[ph]; ok {
			fmt.Fprintf(&b, "- Fase %s: %.1f s\n", ph, float64(t.DurationMS)/1000)
		}
	}
	b.WriteString("\n| Verificación | Verificador | Fase | Estado | Esperado | Observado | Detalle |\n|---|---|---|---|---|---|---|\n")
	esc := func(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "|", "\\|"), "\n", " ") }
	for _, r := range rep.Checks {
		st := r.Status
		if st != Pass {
			st = "**" + st + "**"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s | %s | %s |\n", r.ID, r.Verifier, r.Phase, st,
			esc(truncate(r.Expected, 80)), esc(truncate(r.Observed, 80)), esc(truncate(r.Message, 300)))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
