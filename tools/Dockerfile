# Runner: ejecuta seed, testigos, inventario y los pasos HTTP/git de B0.
# Base fijada por digest; los paquetes apt no lo están (sus versiones van en VERSIONS.md).
FROM python:3.12.16-slim-trixie@sha256:5e2bdb1b0d542d5039cffb8181c65930093949194a65c365dfb1b0fe13a4fa5b

RUN apt-get update \
 && apt-get install -y --no-install-recommends git git-lfs curl ca-certificates postgresql-client \
 && rm -rf /var/lib/apt/lists/*
