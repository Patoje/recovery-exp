# Fallas ocultas: registro de implementación

Fuente: `docs/fallas-ocultas.md` (sha256 `f705b55b6debac97e63f901b3d89cd58a2094a6b0e6deeb9cc9c3216d9ff593e`).
Escenarios: `faults/O1-secuencias-en-cero` (A), `faults/O2-minio-original` (B, no implementado),
`faults/O3-latest-de-otro` (C), `faults/O4-corrupcion-en-retencion` (D).

## Sello

El sello (repositorio `recovery-lab-sellado`, commit `403d641`, revelación en `0b3ed3b`) no se pudo
verificar desde este entorno: la API pública de GitHub responde 404 para ese repositorio y `gh` no
tiene sesión. El commit `0b3ed3b` no está en este repositorio. Queda el hash local de arriba para
compararlo con el sello.

## Líneas base y motor sin cambios

`docs/fallas-ocultas/verificacion-tags.txt`: `git diff` contra `b0-v1` (`scripts/b0.sh`,
`scripts/restore.sh`, `scripts/backup.sh`), `b1-v1` (`baselines/b1`), `engine-v1` (`engine/`) y
`contract-gitea-v1` (`contracts/`) vacío antes de correr; envoltorio del contrato idéntico a la v2
(`e10a404`); sha256 de los binarios del motor y del contrato.

## Cambios en el harness (no en las líneas base)

- `faults/lib/shim/docker`: evento nuevo `after_gitea_start` (después de un `up -d --wait gitea`
  exitoso del proyecto de restauración). Inactivo salvo que el escenario lo habilite en
  `FAULT_EVENTS`; ningún escenario 00–16 lo usa. Lo usa O1.
- `faults/lib/runner.sh`: fase nueva `after_seed` (después de `seed`, antes de los testigos
  previos), vacía por defecto. Lo usa O3. Desde este cambio todas las corridas registran esa fase en
  `result.json`.

Las tres líneas base corren cada escenario con este mismo shim y los mismos criterios de invalidez.

## Equivalencias con las convenciones del archivo

| Archivo | Este entorno |
|---|---|
| `$LAB`, pilas `$LAB/a`, `$LAB/rest` | proyectos Compose `recexp` (original) y `recexp-restore` |
| proyecto `orig` / `rest` | `compose/original.yml` / `compose/restore.yml` |
| servicio `db`, `psql -U gitea -d gitea` | servicio `postgres`, `$POSTGRES_USER` / `$POSTGRES_DB` |
| `$URL_ORIG`, `$URL_REST` (puertos del host) | `http://gitea:3000` desde el runner de cada red (la restauración no publica puertos) |
| `$OWNER/$REPO` = `lab/testigos` | `alice/witness` (repositorio de los testigos) |
| `$TOKEN` | token de API de `alice` creado en origen antes del backup (`faults/lib/api_token.py`), guardado en `runs/<id>/secrets/` |
| `backup-ref.sh` / `restore-ref.sh` | `scripts/backup.sh` / `scripts/restore.sh` (sin cambios) |
| host de Restic `gitea-prod` | `gitea-original` (el que pone `scripts/backup.sh`) |
| rutas `/srv/gitea` | `/dump` y `/data` (las del procedimiento de referencia, iguales para producción y staging) |
| `$WID` | A, C: identificadores de los testigos de la corrida; D: uuid propio de la tanda de testigos de D |

## O1 (A): secuencias en cero

- Inyección: evento `after_gitea_start` (la restauración de referencia ya levantó el proyecto):
  `setval(..., 1, false)` sobre todas las secuencias de `public`, tal como está en el archivo, y
  `restart gitea`. El harness además espera a que Gitea vuelva a estar healthy antes de seguir con la
  línea base (el archivo solo pide el reinicio).
- Control: los conteos por tabla se miden justo después de la inyección (antes de la línea base) y se
  comparan con `inventory.json`.
- La primera corrida (B0, `20261126T044906Z-r01`) tuvo dos observaciones de supuestos vacías por un
  error en la consulta (`"char" || unknown`); se corrigió la consulta antes de B1. Las condiciones de
  `confirm` no cambiaron.

## O2 (B): no implementada

Ver la pregunta 7 de `docs/preguntas-pendientes.md`: Gitea 28.4.0 no arranca con el endpoint de
almacenamiento inalcanzable (falla 11), así que la restauración del paso 2 falla antes del alcance de
red del paso 3.

## O3 (C): `latest` es de otro

- Snapshot viejo de producción: `scripts/backup.sh` en la fase `after_seed` (antes de los testigos).
- Staging: `scripts/restore.sh` de ese snapshot como proyecto `recexp-stg` (override
  `staging.override.yml`, que solo cambia el nombre del proyecto y deja escribir el dump propio), con
  el canario `CANARIO-STAGING-<uuid>` en `alice/notes` (staging no tiene `alice/witness`, que se crea
  con los testigos).
- Backup de staging en `after_backup` (después del de producción): mismos pasos y la misma línea de
  Restic que `scripts/backup.sh` (rutas, exclusiones, etiqueta), con `--host gitea-staging`.
- Restauración: la de referencia sin cambios. `scripts/restore.sh` resuelve `latest` con
  `snapshots --latest 1`, que devuelve el último por host y toma el último de la lista.

## O4 (D): corrupción en toda la retención

- Testigos de D (`testigos.py`): un push de un commit con `testigo-$WID.txt` y el adjunto
  `testigo-$WID.bin`, con sus sha256 al crearlos (`evidence/testigos_d.json`, la referencia contra la
  que se comparan).
- Gitea 28.4.0 rechaza adjuntos `.bin` con la lista de tipos por defecto (HTTP 422). Para subir el
  adjunto con el nombre del archivo, el origen de O4 admite todos los tipos
  (`original.override.yml`: `GITEA__attachment__ALLOWED_TYPES=*/*`). La primera corrida
  (`20261126T050256Z-r01`) quedó `invalid` por eso, en la fase `before_backup`.
- Corrupciones, tres backups más con `scripts/backup.sh` y `restic forget --host gitea-original
  --keep-last 3 --prune`, como en el archivo.

## Supuestos a verificar

Resultados en `evidence/confirm.log` de cada corrida (líneas `[obs] supuesto: …`); resumen en la
sección de resultados más abajo.

## Resultados

Una corrida por escenario y línea base, cada una desde `make reset`. Ninguna corrida tuvo suspensión
del host (sin eventos Sleep; hueco máximo entre testigos 60,1 s).

| Falla | confirm | B0 | B1 | Contrato (primer paso que falla) |
|---|---|---|---|---|
| A (O1) | valid ×3 | pass | pass | pass |
| B (O2) | no implementada (pregunta 7) | — | — | — |
| C (O3) | valid ×3 | pass | fail en `table_row_counts_vs_inventory` | fail en `table_counts` |
| D (O4) | valid ×3 (más una `invalid` de harness, ver arriba) | pass | fail en `git_push_witness_repo` | fail en `git_push` (`error`: el clon falla) |

- A: ninguna línea base hace una escritura que necesite un `id` nuevo en una tabla con filas
  (`git_push` de B1 y del contrato pasan), así que ninguna la detecta.
- C: B1 y el contrato fallan por los conteos por tabla contra el inventario (y las refs); B0 no.
- D: B1 y el contrato fallan porque su prueba de push clona `alice/witness`, el repositorio que tiene
  el blob corrupto (`fatal: … is corrupt` / `invalid index-pack output`). Ninguna línea base compara
  los testigos de D (son del escenario); los testigos propios de B1 y del contrato pasan. B0 no clona
  ese repositorio (clona `alice/notes`).

### Supuestos a verificar

| Falla | Supuesto | Resultado |
|---|---|---|
| A | Columnas `BIGSERIAL` con secuencias ligadas por dependencia `'a'` | Sí: 106 secuencias, todas `deptype = 'a'`, columnas `bigint` con `nextval`; ninguna `IDENTITY` |
| A | La replicación lógica no replica secuencias (PostgreSQL 16.16) | En 16.16 una publicación no admite secuencias: `cannot add relation "issue_id_seq" to publication`, y `FOR ALL SEQUENCES` es un error de sintaxis (prueba en transacción revertida) |
| A | La API devuelve 5xx | 500 en las tres corridas; el log tiene `duplicate key value violates unique constraint "issue_pkey"` |
| B | — | No se verificaron (O2 no implementada) |
| C | `snapshots latest` / `restore latest` sin filtros eligen el más nuevo entre hosts (restic 0.20.0) | Sí: `snapshots latest` devuelve el de `gitea-staging`. `snapshots --latest 1` (lo que usa `scripts/restore.sh`) devuelve el último de cada host y `restore.sh` toma el último de la lista, también el de staging |
| D | Un push chico deja objetos sueltos (`transfer.unpackLimit` 100) | Sí: `test -f` encontró el blob suelto; sin `transfer.unpackLimit` ni `receive.unpackLimit` configurados (valor por defecto) |
| D | Ruta local `attachments/<uuid[0]>/<uuid[1]>/<uuid>` | Sí |
| D | Restic relee un archivo con `mtime` cambiado | Sí: `restic diff S1 S2` marca como modificados el objeto y el adjunto |
| D | Ver el archivo en la web da 500 | No: la página del archivo devuelve 404. La condición d) se cumplió por `git clone`, que falla con `… is corrupt` |
