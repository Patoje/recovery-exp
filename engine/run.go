package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PhaseRecord is what a phase leaves on disk (contract/<phase>.json).
type PhaseRecord struct {
	Phase      string    `json:"phase"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64     `json:"duration_ms"`
	Results    []Result  `json:"results"`
}

// RunPhase runs every check of the phase, in contract order. In "post", host checks
// run first (before any write), then one probe batch, then the final checks (rto).
func (e *Engine) RunPhase(phase string) PhaseRecord {
	rec := PhaseRecord{Phase: phase, StartedAt: e.Now()}
	byID := map[string]Result{}
	var probeChecks []Check
	var ops []ProbeOp
	for _, ch := range e.C.Checks {
		if phaseOf(ch) != phase {
			continue
		}
		v := verifiers[ch.Verifier]
		switch {
		case v.host != nil:
			byID[ch.ID] = v.host(e, ch.ID, ch.params)
		case v.probe != nil:
			o, err := v.probe(e, ch.ID, ch.params)
			if err != nil {
				r := e.start(ch.ID, ch.Verifier, phase)
				byID[ch.ID] = e.done(r, Err, "no se pudo preparar: "+err.Error())
				continue
			}
			probeChecks = append(probeChecks, ch)
			ops = append(ops, o...)
		}
	}
	if len(probeChecks) > 0 {
		var res []ProbeResult
		var perr error
		if len(ops) > 0 {
			res, perr = e.P.Run(ops)
		}
		if perr == nil {
			if b, err := json.MarshalIndent(res, "", "  "); err == nil {
				_ = os.WriteFile(filepath.Join(e.RunDir, "contract", "evidence", "probe_"+phase+".json"), b, 0o644)
			}
		}
		for _, ch := range probeChecks {
			if perr != nil {
				r := e.start(ch.ID, ch.Verifier, phase)
				byID[ch.ID] = e.done(r, Err, perr.Error())
				continue
			}
			r := verifiers[ch.Verifier].evaluate(e, ch.ID, ch.params, res)
			if ev := filepath.Join("contract", "evidence", "probe_"+phase+".json"); r.Status != "" {
				r.Evidence = append(r.Evidence, ev)
			}
			byID[ch.ID] = r
		}
	}
	for _, ch := range e.C.Checks {
		if phaseOf(ch) == phase && verifiers[ch.Verifier].final {
			byID[ch.ID] = evalRTO(e, ch.ID, ch.params, byID)
		}
	}
	for _, ch := range e.C.Checks {
		if r, ok := byID[ch.ID]; ok {
			rec.Results = append(rec.Results, r)
		}
	}
	rec.FinishedAt = e.Now()
	rec.DurationMS = rec.FinishedAt.Sub(rec.StartedAt).Milliseconds()
	return rec
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

func readPhase(path string) (PhaseRecord, error) {
	var rec PhaseRecord
	b, err := os.ReadFile(path)
	if err != nil {
		return rec, err
	}
	return rec, json.Unmarshal(b, &rec)
}

// missingPhase stands for every check of a phase whose record is missing.
func (e *Engine) missingPhase(phase string, why error) PhaseRecord {
	rec := PhaseRecord{Phase: phase}
	for _, ch := range e.C.Checks {
		if phaseOf(ch) == phase {
			r := e.start(ch.ID, ch.Verifier, phase)
			rec.Results = append(rec.Results, e.done(r, Err, fmt.Sprintf("fase %s sin registro: %v", phase, why)))
		}
	}
	return rec
}
