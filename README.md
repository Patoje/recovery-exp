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
| `make reset` | `reset.sh` | Borra contenedores, redes y volúmenes de los dos proyectos, incluido el repositorio de Restic, y `runs/CURRENT`. No borra resultados anteriores. | — |
| `make up` | `up.sh` | Crea una corrida nueva, construye el runner, levanta PostgreSQL + Gitea, crea el administrador e inicializa el repositorio de Restic. | `run.json` |
| `make seed` | `seed.sh` → `seed/seed.py` | Carga el dataset determinista (semilla `SEED`). | `dataset.json`, `secrets/totp.json` |
| `make witness-pre` | `witness.sh pre` | Un commit, un issue, un adjunto y un objeto LFS testigo. | `witnesses_pre.json` |
| `make backup` | `backup.sh` (+ `inventory.sh` como hook) | Backup de referencia. El inventario se toma con Gitea detenido, antes de `pg_dump`. | `backup.json`, `inventory.json`, `restic-backup.jsonl` |
| `make inventory` | `inventory.sh` | Solo el inventario, fuera de la ventana de backup. | `inventory.json` |
| `make witness-post` | `witness.sh post` | Un testigo por minuto, rotando commit → issue → adjunto → LFS, hasta T. | `witnesses_post.json` |
| `make restore [SNAPSHOT=<id>]` | `restore.sh` | Restauración de referencia en el proyecto aislado. Por defecto, `latest`. | `restore.json` |
| `make b0 [SNAPSHOT=<id>]` | `b0.sh` | Línea base B0 (incluye `restic check` y la restauración). | `b0.json` (+ `restore.json`) |
| `make run` | `run.sh` | Todo lo anterior, en orden. | todo lo anterior |
| `make clean-runs` | — | Borra los resultados de corridas anteriores. | — |

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
