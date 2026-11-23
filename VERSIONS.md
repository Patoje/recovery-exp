# Versiones

Las imágenes se fijan por digest del *manifest list* (multi-arquitectura: amd64 y arm64). La etiqueta solo figura como referencia.

## Imágenes

| Componente | Imagen | Versión (etiqueta) | Digest |
|---|---|---|---|
| Gitea | `gitea/gitea` | `28.4.0` | `sha256:aa90e8f00b755b0d665214ab9c14447ea44d3190dbe6b6d0cb46cd9c4f55d309` |
| PostgreSQL | `postgres` | `16.16-alpine` | `sha256:f2f98726820d0342055351c6e9d36e705f5bfffb3dd53ff556325114b08afb6d` |
| Restic | `restic/restic` | `0.20.0` | `sha256:93999df80d4ac6cb148e82d2ef1fe6a81996f17ff535e772723538211b31555a` |
| MinIO (opcional) | `bitnamilegacy/minio` | `2025.7.23-debian-12-r5` | `sha256:6dabb4a2088c9a79908de3bc05f4586c23ad2182c8908e7e3acbf61c1467fb20` |
| Base del runner | `python` | `3.12.16-slim-trixie` | `sha256:5e2bdb1b0d542d5039cffb8181c65930093949194a65c365dfb1b0fe13a4fa5b` |

MinIO: las imágenes oficiales (`minio/minio`, `quay.io/minio/minio`) ya no están disponibles públicamente. `bitnamilegacy/minio` es una imagen congelada.

## Paquetes del runner (apt, no fijados; registrados al construir)

| Paquete | Versión |
|---|---|
| Python | 3.12.16 |
| git | 2.47.3 |
| git-lfs | 3.6.1 |
| curl | 8.14.1 |
| psql (postgresql-client) | 17.12 (Debian 17.12-0+deb13u1) |

## Herramientas

| Herramienta | Versión |
|---|---|
| Restic | 0.20.0 (go1.27.4, linux/arm64) |
| Docker Engine | 29.5.2 (linux/arm64, VM Colima en macOS) |
| Docker Compose | 5.8.0 |

Cada corrida registra en `runs/<RUN_ID>/run.json` las versiones de Docker, Compose y Restic, y las referencias de imagen que usó.

Para obtener los digests:

```bash
docker buildx imagetools inspect gitea/gitea:28.4.0
```
