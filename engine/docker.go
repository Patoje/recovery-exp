package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Docker runs every command through the `docker` found in PATH, so a wrapper placed
// first in PATH (the fault harness shim) sees and can alter every call.
type Docker interface {
	// Compose runs `docker compose -f <file> --env-file <env> [--profile p]... args...`.
	Compose(profiles []string, stdin []byte, args ...string) (stdout, stderr []byte, err error)
	// Docker runs `docker args...`.
	Docker(args ...string) (stdout, stderr []byte, err error)
}

type realDocker struct {
	composeFile, envFile string
	timeout              time.Duration
}

func (d *realDocker) Compose(profiles []string, stdin []byte, args ...string) ([]byte, []byte, error) {
	a := []string{"compose", "-f", d.composeFile}
	if d.envFile != "" {
		a = append(a, "--env-file", d.envFile)
	}
	for _, p := range profiles {
		a = append(a, "--profile", p)
	}
	return d.run(stdin, append(a, args...)...)
}

func (d *realDocker) Docker(args ...string) ([]byte, []byte, error) { return d.run(nil, args...) }

func (d *realDocker) run(stdin []byte, args ...string) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	err := cmd.Run()
	return out.Bytes(), errb.Bytes(), err
}

// Exec runs argv inside a running service.
func Exec(d Docker, service string, env map[string]string, stdin []byte, argv ...string) ([]byte, error) {
	a := []string{"exec", "-T"}
	for k, v := range env {
		a = append(a, "-e", k+"="+v)
	}
	a = append(a, service)
	out, errb, err := d.Compose(nil, stdin, append(a, argv...)...)
	if err != nil {
		return out, fmt.Errorf("exec en %s: %v: %s", service, err, lastLine(errb))
	}
	return out, nil
}

// ConfiguredImage returns the image the compose configuration assigns to a service.
func ConfiguredImage(d Docker, service string) (string, error) {
	out, errb, err := d.Compose(nil, nil, "config", "--format", "json")
	if err != nil {
		return "", fmt.Errorf("compose config: %v: %s", err, lastLine(errb))
	}
	var cfg struct {
		Services map[string]struct {
			Image string `json:"image"`
		} `json:"services"`
	}
	if err := json.Unmarshal(out, &cfg); err != nil {
		return "", fmt.Errorf("compose config: %w", err)
	}
	s, ok := cfg.Services[service]
	if !ok || s.Image == "" {
		return "", fmt.Errorf("compose config: el servicio %s no tiene imagen", service)
	}
	return s.Image, nil
}

type ContainerState struct {
	ID, Image, Status, Health string
	StartedAt                 time.Time
}

// State inspects the container of a service in the compose project.
func State(d Docker, service string) (ContainerState, error) {
	out, errb, err := d.Compose(nil, nil, "ps", "-a", "-q", service)
	id := strings.TrimSpace(string(out))
	if err != nil || id == "" {
		return ContainerState{}, fmt.Errorf("sin contenedor para %s: %s", service, lastLine(errb))
	}
	id = strings.Fields(id)[0]
	out, errb, err = d.Docker("inspect", "--format",
		"{{.Config.Image}}|{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{end}}|{{.State.StartedAt}}", id)
	if err != nil {
		return ContainerState{}, fmt.Errorf("inspect %s: %s", service, lastLine(errb))
	}
	f := strings.SplitN(strings.TrimSpace(string(out)), "|", 4)
	if len(f) != 4 {
		return ContainerState{}, fmt.Errorf("inspect %s: salida inesperada", service)
	}
	t, _ := time.Parse(time.RFC3339Nano, f[3])
	return ContainerState{ID: id, Image: f[0], Status: f[1], Health: f[2], StartedAt: t}, nil
}

func lastLine(b []byte) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return truncate(lines[len(lines)-1], 300)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
