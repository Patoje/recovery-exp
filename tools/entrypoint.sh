#!/bin/sh
# Ejecuta el comando como root (para leer /data) y devuelve runs/ al usuario del host.
"$@"
rc=$?
[ -d /runs ] && chown -R "${HOST_UID:-1000}:${HOST_GID:-1000}" /runs
exit $rc
