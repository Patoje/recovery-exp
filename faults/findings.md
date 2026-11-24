## Hallazgos

- **13 — repositorio de Restic en solo lectura.** Con el repositorio en solo lectura, restic reintenta crear el lock unos 14 min antes de fallar (`Save(<lock/…>) returned error, retrying after …: read-only file system`). Durante todo ese tiempo el procedimiento de referencia deja Gitea detenido, y el cron no lo nota porque no propaga el código de salida. Evidencia: `results/faults/13-readonly-repo-cron/20261124T035656Z-r01/evidence/cron.log` (corrida invalid por la fase backup, conservada).
