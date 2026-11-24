# Resumen de escenarios de falla

Generado por `make summary` a partir de `results/faults/*/*/result.json`. Para cada escenario y línea base se muestra el último lote (en escenarios repetidos, todas las repeticiones). Nada se borra: las corridas anteriores y las invalid se conservan.

`confirm`: la falla ocurrió de verdad; si no, la corrida es `invalid` y no cuenta. Controles: 02 y 15. `Suspensión`: corridas invalid por "harness: suspensión del host" (evento Sleep de `pmset` o hueco > 90 s entre intentos consecutivos de testigos posteriores, exitosos o fallidos), sobre todos los lotes.

| Falla | confirm | B0 | B1 | Suspensión (invalid / total) | t restauración→fin B0 (s) | Resultados |
|---|---|---|---|---|---|---|
| 00-healthy | valid | pass | — | 0 / 2 | 14.4 | `results/faults/00-healthy/20261124T054323Z-r01` |
| 00-healthy-large | valid | pass | — | 0 / 1 | 14.8 | `results/faults/00-healthy-large/20261123T233848Z-r01` |
| 01-appini-excluded | valid | pass | — | 0 / 1 | 14.3 | `results/faults/01-appini-excluded/20261123T234452Z-r01` |
| 02-db-creds-missing (control) | valid | fail en `restore` | — | 0 / 1 | 2.7 | `results/faults/02-db-creds-missing/20261123T235047Z-r01` |
| 03-concurrent-push | valid 10/18 corridas | pass 10/10, fail 0/10 (válidas) | — | 8 / 18 | — | `results/faults/03-concurrent-push/` (18 corridas) |
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

## 03-concurrent-push: repeticiones (B0)

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
