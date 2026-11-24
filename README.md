# recovery-exp

Entorno de prueba, datos y línea base B0 para un experimento que compara dos formas de verificar que un backup se puede restaurar. Esta etapa no incluye el motor de contratos, el contrato, B1 ni la inyección de fallas.

Flujo completo:

```
reset → up → seed → testigos previos → backup (+ inventario) → testigos posteriores → B0 (incluye restauración aislada)
```

## Requisitos del host

- Linux (en Mac o Windows, dentro de una VM Linux).
- Docker Engine con Compose v2, GNU make, bash, coreutils.

Todo lo demás corre en contenedores: Gitea, PostgreSQL, Restic, MinIO y un contenedor *runner* (Python, git, git-lfs, curl, psql) que se construye desde `tools/Dockerfile`. Las versiones y digests están en [VERSIONS.md](VERSIONS.md).

Los secretos son valores de prueba, definidos en `.env.example`. `make reset` copia ese archivo a `.env` si `.env` no existe.

## Corrida completa

```bash
make reset
```

```bash
make run
```

`make run` exige un estado limpio: si el entorno original ya tiene volúmenes, falla y pide `make reset`. Duración aproximada: 3 minutos más `INCIDENT_MINUTES` (5 por defecto).

Para cambiar T:

```bash
INCIDENT_MINUTES=10 make run
```

## Pasos por separado

Cada paso es un objetivo de `make` y un script en `scripts/`. Los pasos trabajan sobre la corrida actual, cuyo ID está en `runs/CURRENT` (lo crea `make up`). Los scripts también aceptan la carpeta de la corrida como argumento.

| Comando | Script | Qué hace | Salida |
|---|---|---|---|
| `make reset` | `reset.sh` | Borra contenedores, redes y volúmenes de los dos proyectos, incluido el repositorio de Restic, y `runs/CURRENT`. No borra resultados de corridas. | — |
| `make up` | `up.sh` | Crea una corrida nueva, construye el runner, levanta PostgreSQL + Gitea, crea el administrador e inicializa el repositorio de Restic. | `run.json` |
| `make seed` | `seed.sh` → `seed/seed.py` | Carga el dataset determinista (semilla `SEED`). | `dataset.json`, `secrets/totp.json` |
| `make witness-pre` | `witness.sh pre` | Un commit, un issue, un adjunto y un objeto LFS testigo. | `witnesses_pre.json` |
| `make backup` | `backup.sh` (+ `inventory.sh` como hook) | Backup de referencia. El inventario se toma con Gitea detenido, antes de `pg_dump`. | `backup.json`, `inventory.json`, `restic-backup.jsonl` |
| `make inventory` | `inventory.sh` | Solo el inventario, fuera de la ventana de backup. | `inventory.json` |
| `make witness-post` | `witness.sh post` | Un testigo por minuto, rotando commit → issue → adjunto → LFS, hasta T. | `witnesses_post.json` |
| `make restore [SNAPSHOT=<id>]` | `restore.sh` | Restauración de referencia en el proyecto aislado. Por defecto, `latest`. | `restore.json` |
| `make b0 [SNAPSHOT=<id>]` | `b0.sh` | Línea base B0 (incluye `restic check` y la restauración). | `b0.json` (+ `restore.json`) |
| `make run` | `run.sh` | Todo lo anterior, en orden. | todo lo anterior |

Para usar la variante MinIO, anteponé `MINIO=1` a cada comando, por ejemplo `MINIO=1 make run`.

## Entornos

### Original (`compose/original.yml`, proyecto `recexp`)

- `postgres` y `gitea`. Gitea se configura contra PostgreSQL con variables `GITEA__*`, que la imagen escribe en `/data/gitea/conf/app.ini`.
- `restic` (perfil `tools`), que se ejecuta a demanda. Su repositorio es el volumen dedicado `recexp_restic-repo`.
- `runner` (perfil `tools`), que se ejecuta a demanda.
- `minio` (perfil `minio`, inactivo por defecto). `compose/original.minio.yml` configura Gitea con `STORAGE_TYPE = minio` en `[attachment]` y `[lfs]`.
  - El backup de referencia no incluye el bucket de MinIO. Es una brecha intencional del experimento.
  - Las imágenes oficiales de MinIO ya no se publican en Docker Hub ni en quay.io. Por eso se usa `bitnamilegacy/minio`, que es una imagen congelada.

### Restauración (`compose/restore.yml`, proyecto `recexp-restore`)

- Proyecto separado, con volúmenes propios que se recrean en cada restauración.
- Red `internal: true`, sin salida a internet. Por eso los pasos HTTP y git de B0 corren dentro del runner conectado a esa red, y no desde el host.
- Monta el repositorio de Restic del entorno original en solo lectura y usa `--no-lock --no-cache`.

## Procedimientos de referencia

**Backup** (`scripts/backup.sh`):

1. Detiene Gitea.
2. Ejecuta `pg_dump -Fc`.
3. Ejecuta `restic backup` sobre el dump y **todo** `/data` (`/data/git`, con `.ssh`, repositorios y LFS, y `/data/gitea` completo, incluido `app.ini`). Solo excluye `/data/gitea/log` y `/data/gitea/indexers`.
4. Arranca Gitea.

Usa `set -euo pipefail`: si falla cualquier paso, el script falla. Si falla con Gitea detenido, vuelve a arrancarlo, pero sale igual con error. `backup.json` registra el ID y la hora del snapshot y la duración de cada paso.

**Restauración** (`scripts/restore.sh [SNAPSHOT]`):

1. Recrea los volúmenes del proyecto aislado.
2. Resuelve el snapshot (`latest` o un ID).
3. Ejecuta `restic restore`.
4. Levanta PostgreSQL.
5. Ejecuta `pg_restore --exit-on-error`.
6. Levanta Gitea hasta que pasa su healthcheck.

`restore.json` registra `restore_started_at`, `restore_finished_at` (después de `pg_restore`) y `first_start_at` (Gitea sano).

## Datos y verdad de referencia

- **Dataset** (`seed/seed.py`): 5 usuarios, 8 repositorios (2 con LFS) con commits y ramas, issues, comentarios y adjuntos. El usuario `erin` tiene TOTP. Las fechas de commit son fijas, así que los SHA son reproducibles. No son deterministas los IDs y timestamps internos de Gitea ni el secreto TOTP (lo genera Gitea). El secreto se guarda solo en `runs/<RUN_ID>/secrets/totp.json`. `dataset.json` registra el tamaño total (directorio de datos + base).
- **Testigos** (`witness/witness.py`), en el repositorio `alice/witness`: cada uno tiene tipo, UUID, hora de creación y sha256. Los posteriores también registran el minuto, y el archivo registra `incident_minutes` e `incident_at`.
- **Inventario** (`witness/inventory.py`): conteo de filas por tabla, refs de cada repositorio (nombre → SHA) y sha256 de los archivos testigo previos tal como están en `/data`. Es de solo lectura: `/data` se monta `ro` y la sesión de PostgreSQL usa `default_transaction_read_only=on`.

Los testigos y el inventario son la verdad de referencia. **B0 no los usa.**

## Línea base B0

`scripts/b0.sh` ejecuta exactamente estos pasos y se detiene en la primera falla. Los pasos que quedan se marcan `skipped`.

1. `restic check --read-data`
2. Restauración con `restore.sh`.
3. `docker compose up --wait` (healthchecks).
4. `GET /` → 200.
5. `GET /api/v1/version` → 200.
6. Login por API (`GET /api/v1/user`, basic auth) con `B0_USER`, que no tiene 2FA.
7. `GET /api/v1/repos/search` devuelve al menos un repositorio.
8. `git clone` de `B0_CLONE_REPO`.

Sale con 0 si todo pasa y con un código distinto de 0 si algo falla. `b0.json` registra el resultado, la duración y el detalle de cada paso.

## Resultados

Cada corrida queda en `runs/<RUN_ID>/`: `run.json` (incluye `incident_minutes`, versiones e imágenes), `dataset.json`, `witnesses_pre.json`, `inventory.json`, `backup.json`, `witnesses_post.json`, `restore.json`, `b0.json` y `secrets/totp.json`.

## Esfuerzo

[docs/esfuerzo.md](docs/esfuerzo.md): tiempo dedicado a cada componente.

## Etapa 2: escenarios de falla

Cada escenario vive en `faults/NN-nombre/` y arranca siempre desde `make reset`:

```bash
make fault N=07
```

```bash
make fault N=00-healthy
```

```bash
make faults
```

```bash
make summary
```

Pipeline de `faults/lib/runner.sh`: reset → `prepare` → up → seed → testigos previos → `before_backup` → backup → declare → `after_backup` → testigos posteriores → línea base (`BASELINE=b0` por defecto) → `confirm`.

Archivos de cada escenario:

- `scenario.env`: nombre, repeticiones (`REPEAT`), `SHIM`, `MINIO` y las variables `FAULT_*` que lee el shim.
- `inject`: biblioteca bash con los ganchos `prepare`, `before_backup`, `backup` (si el escenario reemplaza el backup), `after_backup`, `witness_post` y `on_event` (eventos del shim).
- `confirm`: verificación objetiva de que la falla ocurrió. Sale con 0 si ocurrió. Si no, la corrida queda `invalid` y no cuenta.
- Archivos propios (overrides de Compose, Dockerfile, scripts en Python).

### Shim de docker

`faults/lib/shim/docker` se antepone al `PATH` durante todo el escenario, de modo que B0, B1 y el futuro contrato corren con el mismo shim. Reenvía cada llamada al docker real y aplica solo las desviaciones que declara el escenario:

- `-f` extra para cada proyecto.
- Otro `--env-file` y variables quitadas del entorno.
- Argumentos extra para `restic backup` y `restic` de restauración.
- Quitar `--exit-on-error` de `pg_restore`.
- Omitir `stop gitea`.
- Eventos `after_pg_dump`, `after_restic_restore` y `before_gitea_start`.

Cada modificación se registra con el prefijo `[shim]` en el log de la corrida y en `evidence/shim.log`. Los scripts de referencia y B0 no se modifican. En `00-healthy` y `00-healthy-large` el shim está inactivo, y su `confirm` lo verifica.

### Resultados

`results/faults/<escenario>/<lote>-rNN/` guarda `run.log`, `result.json` (estado `pass`, `fail` o `invalid`, fases, resultado de la línea base y checks de `confirm`) y `evidence/`. Los artefactos de la corrida siguen en `runs/<RUN_ID>/`, que ahora incluye `declared.json`. Nada se borra. `make summary` genera `results/summary.md` con el último lote de cada escenario.

`scripts/declare.sh` registra en `declared.json` los valores declarados al hacer el backup: imagen y versión de Gitea, sha256 de `SECRET_KEY`, codificación de la base y dependencias externas. No forma parte de los procedimientos de referencia.

Los objetivos declarados están en `objectives.env`.

## Línea base B1

`baselines/b1/b1.sh [SNAPSHOT] RUN_DIR` ejecuta B0 completa y además verificaciones convencionales. Es independiente de B0, del shim y de los escenarios: solo usa los artefactos de la corrida (`runs/<RUN_ID>/*.json`, `secrets/`), los valores declarados (`declared.json`, `objectives.env`) y el entorno (`.env`, `compose/restore.yml`). Con `BASELINE=b1`, el runner de escenarios la usa como línea base:

```bash
BASELINE=b1 make fault N=00-healthy
```

Verificaciones, en orden:

1. Antes de B0, es decir, antes de cualquier migración: imagen de Gitea configurada para la restauración y su versión (`gitea --version`), comparadas con las declaradas.
2. B0 completa (`b0.sh`), con su log en `runs/<id>/b1/b0.log`.
3. Autoprueba de los patrones de error y búsqueda en el log de B0 con patrones exactos: `pg_restore: error:`, `pg_restore: warning: errors ignored on restore:`, `Fatal: `, `ignoring error for `, `error: ` y `error for tree ` (restic). Nunca usa la palabra "error" suelta, y `no errors were found` no cuenta. Un error en el log es fallo aunque el código de salida sea 0.
4. Imagen y versión de Gitea en marcha, huella de `SECRET_KEY`, codificación de la base y dependencias externas declaradas.
5. Conteo de filas por tabla, repositorios de la base frente a los del disco y refs (nombre → SHA) frente a `inventory.json`.
6. Hash del adjunto testigo, contenido del objeto LFS testigo, existencia del issue testigo, login de la cuenta con TOTP y `git push` al repositorio testigo.
7. RTO: desde `restore_started_at` hasta el fin de las pruebas funcionales. RPO: T menos la hora del testigo más reciente recuperado. Los dos se comparan con `objectives.env`.

Salida: `runs/<id>/b1.json`, con el resultado de cada paso, y `runs/<id>/b1/raw/`, con los datos crudos leídos del sistema restaurado. Sale con 0 solo si todo pasa.
