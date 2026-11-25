// Command engine verifies a restored system against a declarative contract.
//
//	engine verify --phase pre|post --contract FILE --run RUN_DIR [--root DIR] [--probe-bin FILE]
//	engine check-contract FILE
//	engine probe            (inside the probe service: ops on stdin, results on stdout)
//
// "pre" runs before the restore (configuration as it will start, backup integrity) and
// writes RUN_DIR/contract/pre.json. "post" runs after the restore and writes the report:
// RUN_DIR/contract.json and RUN_DIR/contract.md. Exit code: 0 pass, 1 fail, 2 usage or contract error.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const EngineVersion = "1"

var engineCommit = "unknown" // set with -ldflags at build time

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "probe":
		if err := runProbe(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "probe:", err)
			os.Exit(2)
		}
	case "check-contract":
		if len(os.Args) != 3 {
			usage()
		}
		c, _, err := LoadContract(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Printf("contrato %s válido: %d componentes, %d verificaciones\n", c.Name, len(c.Components), len(c.Checks))
	case "verify":
		os.Exit(verify(os.Args[2:]))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "uso: engine verify --phase pre|post --contract FILE --run RUN_DIR [--root DIR] [--probe-bin FILE] | engine check-contract FILE | engine probe")
	os.Exit(2)
}

func verify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	phase := fs.String("phase", "", "pre | post")
	contractPath := fs.String("contract", "", "contract file")
	runDir := fs.String("run", "", "run directory")
	root := fs.String("root", ".", "directory the contract's paths are relative to")
	probeBin := fs.String("probe-bin", "", "engine binary for the probe's platform")
	timeout := fs.Duration("timeout", 15*time.Minute, "timeout of each docker command")
	_ = fs.Parse(args)
	if (*phase != "pre" && *phase != "post") || *contractPath == "" || *runDir == "" {
		usage()
	}
	c, raw, err := LoadContract(*contractPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	abs := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(*root, p)
	}
	v, err := NewValues(*runDir, abs(c.Environment.EnvFile), abs(c.ObjectivesFile))
	if err != nil {
		fmt.Fprintln(os.Stderr, "valores:", err)
		return 2
	}
	d := &realDocker{composeFile: abs(c.Environment.ComposeFile), envFile: abs(c.Environment.EnvFile), timeout: *timeout}
	e := &Engine{C: c, V: v, D: d, RunDir: *runDir, Now: func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) },
		P: &composeProbe{d: d, probe: c.Environment.Probe, binPath: *probeBin}}
	dir := filepath.Join(*runDir, "contract")
	if err := os.MkdirAll(filepath.Join(dir, "evidence"), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if *phase == "pre" {
		rec := e.RunPhase("pre")
		if err := writeJSON(filepath.Join(dir, "pre.json"), rec); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		logResults(rec)
		return 0 // the verdict is given once, after "post"
	}
	pre, err := readPhase(filepath.Join(dir, "pre.json"))
	if err != nil {
		pre = e.missingPhase("pre", err)
	}
	post := e.RunPhase("post")
	_ = writeJSON(filepath.Join(dir, "post.json"), post)
	logResults(post)
	rep := buildReport(e, *contractPath, raw, pre, post)
	if err := writeJSON(filepath.Join(*runDir, "contract.json"), rep); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	_ = writeMarkdown(filepath.Join(*runDir, "contract.md"), rep)
	fmt.Fprintf(os.Stderr, "[contract] %s (%d pass, %d fail, %d error; custom_check: %d)\n",
		rep.Result, rep.Counts[Pass], rep.Counts[Fail], rep.Counts[Err], len(rep.CustomChecks))
	return rep.ExitCode
}

func logResults(rec PhaseRecord) {
	for _, r := range rec.Results {
		fmt.Fprintf(os.Stderr, "[contract] %-5s %-5s %s: %s\n", rec.Phase, r.Status, r.ID, truncate(r.Message, 300))
	}
}
