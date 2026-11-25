# Resumen de escenarios de falla

Generado por `make summary` a partir de `results/faults/*/*/result.json`. Para cada versión del entorno, escenario y línea base se muestra el último lote (en escenarios repetidos, todas las repeticiones). Nada se borra: las corridas anteriores y las invalid se conservan.

Entornos: **v1** (`b0-v1`, `SECRET_KEY` vacía en `app.ini`) y **v2** (`make up` genera una `SECRET_KEY` aleatoria y la escribe solo en `app.ini`). Los resultados sin `env_version` son del entorno v1.

`confirm`: la falla ocurrió de verdad; si no, la corrida es `invalid` y no cuenta. Controles: 02 y 15. `Suspensión`: corridas invalid por "harness: suspensión del host" (evento Sleep de `pmset` o hueco > 90 s entre intentos consecutivos de testigos posteriores, exitosos o fallidos), sobre todos los lotes.

## Entorno v2

| Falla | confirm | B0 | B1 | Suspensión (invalid / total) | t restauración→fin B0 (s) | Resultados |
|---|---|---|---|---|---|---|
| 00-healthy | valid | pass | pass | 0 / 2 | 14.2 | `results/faults/00-healthy/20261124T153613Z-r01`<br>`results/faults/00-healthy/20261124T185739Z-r01` |
| 00-healthy-large | valid | pass | — | 0 / 1 | 15.3 | `results/faults/00-healthy-large/20261124T154215Z-r01` |
| 01-appini-excluded | valid | pass | — | 0 / 2 | 14.5 | `results/faults/01-appini-excluded/20261125T073123Z-r01` |
| 02-db-creds-missing (control) | valid | fail en `restore` | — | 0 / 1 | 2.6 | `results/faults/02-db-creds-missing/20261124T155431Z-r01` |
| 03-concurrent-push | valid 10/10 corridas | pass 10/10, fail 0/10 (válidas) | — | 0 / 10 | — | `results/faults/03-concurrent-push/` (10 corridas v2) |
| 04-attachments-excluded | valid | pass | — | 0 / 1 | 14.1 | `results/faults/04-attachments-excluded/20261124T170006Z-r01` |
| 05-lfs-excluded | valid | pass | — | 0 / 1 | 14.3 | `results/faults/05-lfs-excluded/20261124T170609Z-r01` |
| 06-repo-after-dump | valid | pass | — | 0 / 1 | 14.2 | `results/faults/06-repo-after-dump/20261124T171213Z-r01` |
| 07-secret-key-changed | valid | pass | — | 0 / 1 | 14.4 | `results/faults/07-secret-key-changed/20261124T171821Z-r01` |
| 08-binary-path-changed | invalid (confirm no se cumplió) | pass | — | 0 / 1 | 14.6 | `results/faults/08-binary-path-changed/20261124T172425Z-r01` |
| 09-chown-root | valid | pass | — | 0 / 1 | 14.4 | `results/faults/09-chown-root/20261124T173027Z-r01` |
| 10-newer-gitea | valid | pass | — | 0 / 1 | 15.3 | `results/faults/10-newer-gitea/20261124T173629Z-r01` |
| 11-minio-bucket-excluded | valid | fail en `restore` | — | 0 / 1 | 6.2 | `results/faults/11-minio-bucket-excluded/20261124T174232Z-r01` |
| 12-slow-restore | valid | pass | — | 0 / 1 | 272.2 | `results/faults/12-slow-restore/20261124T174828Z-r01` |
| 13-readonly-repo-cron | valid | pass | — | 0 / 1 | 14.4 | `results/faults/13-readonly-repo-cron/20261124T180318Z-r01` |
| 14-latin1-database | valid | fail en `restore` | — | 0 / 1 | 5.6 | `results/faults/14-latin1-database/20261124T183921Z-r01` |
| 15-corrupt-pack (control) | valid | fail en `restic_check` | — | 0 / 1 | — | `results/faults/15-corrupt-pack/20261124T184523Z-r01` |
| 16-casefold-refs | valid | pass | — | 0 / 1 | 15.0 | `results/faults/16-casefold-refs/20261124T185120Z-r01` |

## Entorno v1

| Falla | confirm | B0 | B1 | Suspensión (invalid / total) | t restauración→fin B0 (s) | Resultados |
|---|---|---|---|---|---|---|
| 00-healthy | valid | pass | pass | 0 / 3 | 14.4 | `results/faults/00-healthy/20261124T054323Z-r01`<br>`results/faults/00-healthy/20261124T063110Z-r01` |
| 00-healthy-large | valid | pass | — | 0 / 1 | 14.8 | `results/faults/00-healthy-large/20261123T233848Z-r01` |
| 01-appini-excluded | valid | pass | — | 0 / 1 | 14.3 | `results/faults/01-appini-excluded/20261123T234452Z-r01` |
| 02-db-creds-missing (control) | valid | fail en `restore` | — | 0 / 1 | 2.7 | `results/faults/02-db-creds-missing/20261123T235047Z-r01` |
| 03-concurrent-push | valid 10/18 corridas | pass 10/10, fail 0/10 (válidas) | — | 8 / 18 | — | `results/faults/03-concurrent-push/` (18 corridas v1) |
| 04-attachments-excluded | valid | pass | — | 0 / 1 | 13.9 | `results/faults/04-attachments-excluded/20261124T025430Z-r01` |
| 05-lfs-excluded | valid | pass | — | 0 / 1 | 14.0 | `results/faults/05-lfs-excluded/20261124T030028Z-r01` |
| 06-repo-after-dump | valid | pass | — | 0 / 1 | 14.4 | `results/faults/06-repo-after-dump/20261124T030626Z-r01` |
| 07-secret-key-changed | valid | pass | — | 0 / 1 | 14.0 | `results/faults/07-secret-key-changed/20261124T031230Z-r01` |
| 08-binary-path-changed | invalid (confirm no se cumplió) | pass | — | 0 / 2 | 14.3 | `results/faults/08-binary-path-changed/20261124T053713Z-r01` |
| 09-chown-root | valid | pass | — | 0 / 1 | 13.9 | `results/faults/09-chown-root/20261124T032424Z-r01` |
| 10-newer-gitea | valid | pass | — | 0 / 1 | 14.3 | `results/faults/10-newer-gitea/20261124T033020Z-r01` |
| 11-minio-bucket-excluded | valid | fail en `restore` | — | 0 / 1 | 6.8 | `results/faults/11-minio-bucket-excluded/20261124T033622Z-r01` |
| 12-slow-restore | valid | pass | — | 0 / 1 | 271.7 | `results/faults/12-slow-restore/20261124T034213Z-r01` |
| 13-readonly-repo-cron | valid | pass | — | 0 / 2 | 13.9 | `results/faults/13-readonly-repo-cron/20261124T054919Z-r01` |
| 14-latin1-database | valid | fail en `restore` | — | 0 / 1 | 5.7 | `results/faults/14-latin1-database/20261124T043205Z-r01` |
| 15-corrupt-pack (control) | valid | fail en `restic_check` | — | 0 / 1 | — | `results/faults/15-corrupt-pack/20261124T043800Z-r01` |
| 16-casefold-refs | valid | pass | — | 0 / 1 | 14.4 | `results/faults/16-casefold-refs/20261124T044351Z-r01` |

## Comparación de B0: entorno v1 contra v2

| Falla | confirm v1 | B0 v1 | confirm v2 | B0 v2 | t v1 (s) | t v2 (s) | ¿Cambió B0? |
|---|---|---|---|---|---|---|---|
| 00-healthy | valid | pass | valid | pass | 14.4 | 14.2 | no |
| 00-healthy-large | valid | pass | valid | pass | 14.8 | 15.3 | no |
| 01-appini-excluded | valid | pass | valid | pass | 14.3 | 14.5 | no |
| 02-db-creds-missing | valid | fail en `restore` | valid | fail en `restore` | 2.7 | 2.6 | no |
| 03-concurrent-push | valid 10/18 corridas | pass 10/10, fail 0/10 (válidas) | valid 10/10 corridas | pass 10/10, fail 0/10 (válidas) | — | — | no |
| 04-attachments-excluded | valid | pass | valid | pass | 13.9 | 14.1 | no |
| 05-lfs-excluded | valid | pass | valid | pass | 14.0 | 14.3 | no |
| 06-repo-after-dump | valid | pass | valid | pass | 14.4 | 14.2 | no |
| 07-secret-key-changed | valid | pass | valid | pass | 14.0 | 14.4 | no |
| 08-binary-path-changed | invalid (confirm no se cumplió) | pass | invalid (confirm no se cumplió) | pass | 14.3 | 14.6 | no |
| 09-chown-root | valid | pass | valid | pass | 13.9 | 14.4 | no |
| 10-newer-gitea | valid | pass | valid | pass | 14.3 | 15.3 | no |
| 11-minio-bucket-excluded | valid | fail en `restore` | valid | fail en `restore` | 6.8 | 6.2 | no |
| 12-slow-restore | valid | pass | valid | pass | 271.7 | 272.2 | no |
| 13-readonly-repo-cron | valid | pass | valid | pass | 13.9 | 14.4 | no |
| 14-latin1-database | valid | fail en `restore` | valid | fail en `restore` | 5.7 | 5.6 | no |
| 15-corrupt-pack | valid | fail en `restic_check` | valid | fail en `restic_check` | — | — | no |
| 16-casefold-refs | valid | pass | valid | pass | 14.4 | 15.0 | no |

## 03-concurrent-push: repeticiones (B0, entorno v2)

| Corrida | Estado | confirm | B0 | Detalle |
|---|---|---|---|---|
| `20261124T160018Z-r01` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T160018Z-r02` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T160018Z-r03` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T160018Z-r04` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T160018Z-r05` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T160018Z-r06` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T160018Z-r07` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T160018Z-r08` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T160018Z-r09` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T160018Z-r10` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |

## 03-concurrent-push: repeticiones (B0, entorno v1)

| Corrida | Estado | confirm | B0 | Detalle |
|---|---|---|---|---|
| `20261123T235629Z-r01` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (4 diferencias) |
| `20261123T235629Z-r02` | invalid | valid | pass | suspensión: pmset_sleep, witness_gap; 2 Sleep; hueco máx. 1078.8 s |
| `20261123T235629Z-r03` | invalid | valid | pass | suspensión: pmset_sleep; 3 Sleep; hueco máx. 0.5 s |
| `20261123T235629Z-r04` | invalid | valid | pass | suspensión: pmset_sleep, witness_gap; 6 Sleep; hueco máx. 1490.4 s |
| `20261123T235629Z-r05` | invalid | valid | pass | suspensión: pmset_sleep; 3 Sleep; hueco máx. 0.5 s |
| `20261123T235629Z-r06` | invalid | valid | pass | suspensión: pmset_sleep, witness_gap; 5 Sleep; hueco máx. 1766.3 s |
| `20261123T235629Z-r07` | invalid | valid | pass | suspensión: pmset_sleep; 3 Sleep; hueco máx. 0.5 s |
| `20261123T235629Z-r08` | invalid | valid | pass | suspensión: pmset_sleep, witness_gap; 4 Sleep; hueco máx. 929.5 s |
| `20261123T235629Z-r09` | invalid | valid | pass | suspensión: pmset_sleep; 2 Sleep; hueco máx. 0.5 s |
| `20261123T235629Z-r10` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T045008Z-r01` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T045008Z-r02` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T045008Z-r03` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T045008Z-r04` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T045008Z-r05` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T045008Z-r06` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (4 diferencias) |
| `20261124T045008Z-r07` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |
| `20261124T045008Z-r08` | pass | valid | pass | [ok] inconsistencia base/disco en el sistema restaurado (5 diferencias) |

## Hallazgos

- **13 — repositorio de Restic en solo lectura.** Con el repositorio en solo lectura, restic reintenta crear el lock unos 14 min antes de fallar (`Save(<lock/…>) returned error, retrying after …: read-only file system`). Durante todo ese tiempo el procedimiento de referencia deja Gitea detenido, y el cron no lo nota porque no propaga el código de salida. Evidencia: `results/faults/13-readonly-repo-cron/20261124T035656Z-r01/evidence/cron.log` (corrida invalid por la fase backup, conservada).

- **08 — ruta del ejecutable distinta en la restauración.** Gitea 28.4.0 regenera los hooks al arrancar (`routers/init.go:97:syncAppConfForGit() [I] re-sync repository hooks ...`). La advertencia de la documentación sobre regenerar los hooks después de cambiar la ruta del ejecutable no aplica a esta versión. La falla no se produce y el 08 queda `invalid`; no se agregaron desviaciones. Evidencia: `results/faults/08-binary-path-changed/*/evidence/hooks_before_first_start.txt` y el log de Gitea en `run.log`.
- **SECRET_KEY vacía con `INSTALL_LOCK=true`.** La instalación de Gitea 28.4.0 con `GITEA__security__INSTALL_LOCK=true` y sin `SECRET_KEY` deja `[security] SECRET_KEY =` vacía en `app.ini`, y Gitea usa su clave interna por defecto, la misma en todas las instalaciones. Sí genera en el primer arranque `INTERNAL_TOKEN`, `[server] LFS_JWT_SECRET`, `[oauth2] JWT_SECRET` y `jwt/private.pem`. Afecta a todos los resultados del entorno v1. En el entorno v2, `make up` genera la clave una vez y la escribe solo en `app.ini`. Evidencia: `results/faults/00-healthy/20261124T063110Z-r01` (huella `e3b0c442…`, sha256 de un texto vacío).
- **01 en v2 — qué `SECRET_KEY` queda en uso cuando falta `app.ini`.** Queda **vacía**, no se genera: la imagen crea `app.ini` desde su plantilla con `SECRET_KEY =` (huella `e3b0c442…`), distinta de la declarada al hacer el backup (`a46d3ff6…`). Gitea sí genera de nuevo `INTERNAL_TOKEN`, `LFS_JWT_SECRET` y `[oauth2] JWT_SECRET`. B0 da `pass` igual. Evidencia: `results/faults/01-appini-excluded/20261125T073123Z-r01/evidence/` (`confirm.log` y `secret_key_manual_check.txt`). El `[obs]` de esa corrida muestra `01ba4719…` porque hasheó el valor vacío con un salto de línea; el `[obs]` se corrigió después de esa corrida.
