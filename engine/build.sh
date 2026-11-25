#!/usr/bin/env bash
# Compila y prueba el motor en la imagen golang fijada:
#   engine/bin/engine-darwin-arm64 (host) y engine/bin/engine-linux-arm64 (sonda).
# La prueba de git push corre en la imagen del runner (la de golang no trae git).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
GOIMG=golang@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648 # 1.26.4-alpine
commit="$(git rev-parse --short HEAD)$(git diff --quiet HEAD -- engine contracts 2>/dev/null || echo -dirty)"
docker run --rm -v "$ROOT:/repo" -w /repo/engine -e GOFLAGS=-mod=vendor -e CGO_ENABLED=0 -e GOCACHE=/tmp/gocache "$GOIMG" sh -c "
  test -z \"\$(gofmt -l *.go)\" || { gofmt -l *.go; echo 'gofmt: archivos sin formato' >&2; exit 1; }
  go vet . && go test -count=1 . &&
  GOOS=linux GOARCH=arm64 go test -c -o bin/engine-linux-arm64.test . &&
  GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags '-X main.engineCommit=$commit' -o bin/engine-darwin-arm64 . &&
  GOOS=linux  GOARCH=arm64 go build -trimpath -ldflags '-X main.engineCommit=$commit' -o bin/engine-linux-arm64 ."
docker run --rm -v "$ROOT/engine:/e:ro" -w /e --entrypoint /e/bin/engine-linux-arm64.test recexp-runner:local \
  -test.run 'TestProbeGitPush' -test.v | grep -E '^(--- |ok|PASS|FAIL)'
rm -f engine/bin/engine-linux-arm64.test
echo "engine: compilado ($commit)"
