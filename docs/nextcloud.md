# Etapa 5: entorno de Nextcloud

Mismo esquema que el entorno de Gitea, en `nc/`: proyecto original (`ncexp`), proyecto de restauración
aislado (`ncexp-restore`, red interna, volúmenes nuevos, repositorio de Restic en solo lectura), Restic,
backup y restauración de referencia, seed determinista, testigos e inventario. Nada de Gitea cambia
(`scripts/`, `compose/`, `baselines/b1`, `engine/`, `contracts/gitea.yaml`, escenarios 00–16 y O*).

| | Gitea | Nextcloud |
|---|---|---|
| Imágenes | `gitea/gitea` 28.4.0, `postgres` 16.16-alpine, `restic` 0.20.0 | `nextcloud` 34.0.5-apache (`sha256:d8ad0b77…`), mismos `postgres` y `restic` |
| Datos | `/data` (volumen `gitea-data`) | `config/` (`nc-config`) y el directorio de datos (`nc-data`); el código sale de la imagen (`nc-html`, nuevo en cada restauración) |
| Backup de referencia | Gitea detenido → `pg_dump` → `restic backup /dump /data` → Gitea arriba | modo mantenimiento activado → `pg_dump` → `restic backup /dump /data /config` → modo mantenimiento desactivado (`nc/scripts/backup.sh`) |
| Restauración de referencia | volúmenes nuevos → `restic restore` → `pg_restore --exit-on-error` → arrancar Gitea | ídem, arrancar Nextcloud → `occ maintenance:mode --off` (el backup se tomó en mantenimiento) → `occ maintenance:data-fingerprint` (`nc/scripts/restore.sh`) |
| Seed | usuarios, repositorios, issues, LFS | 5 usuarios; por usuario `Documents/`, `Notes/` (6 textos cada una) y `Photos/` (3 binarios de 256 KiB); `Documents` compartida con el usuario siguiente; enlace público de `alice/Notes` (`nc/seed/seed.py`) |
| Testigos | commit, issue, adjunto, LFS | archivo de texto, archivo binario y archivo compartido con `bob` (`nc/witness/witness.py`) |
| Inventario | conteos por tabla, refs, testigos | conteos por tabla, archivos de usuario en la base (`oc_filecache`) y en disco, testigos (`nc/witness/inventory.py`) |
| Valores declarados | imagen, versión, huella de `SECRET_KEY`, codificación | imagen, versión, huella de `secret` de `config.php`, codificación (`nc/scripts/declare.sh`) |
| B0 | `scripts/b0.sh` | `nc/scripts/b0.sh`: la B0 de Gitea adaptada lo mínimo (mismos 8 pasos; página de login, `status.php`, login por OCS, listado WebDAV, descarga de un archivo) |
| Runner de escenarios | `faults/lib/runner.sh` | `faults/lib/nc/runner.sh` (copia con los scripts de `nc/`); mismo shim y mismos criterios de invalidez |

Uso: `make nc-fault N=nc-00 BASELINE=b0|b1|contract`. Los valores de prueba están en `nc/.env.example`
(se copia a `nc/.env`).

## Decisiones del entorno

- **Usuario de base dedicado.** Si el instalador de Nextcloud recibe un superusuario de PostgreSQL, crea
  su propio rol (`oc_<admin>`) y le da las tablas. Los roles no van en `pg_dump`, así que la restauración
  de referencia falla en `pg_restore` (`role "oc_admin" does not exist`). Como en una instalación de
  producción, Nextcloud usa un usuario dedicado (`NC_DB_USER`, dueño de la base, sin `CREATEROLE`),
  creado por `nc/compose/initdb/10-nextcloud-user.sh` en el primer arranque de PostgreSQL, igual en el
  original y en la restauración.
- **Sin esqueleto.** `skeletondirectory` vacío: sin eso cada usuario nuevo recibe ~63 MB de archivos de
  ejemplo. El administrador (instalación) sí los tiene.
- **Sin salida a internet.** `has_internet_connection=false`, `appstoreenabled=false`, tareas de fondo
  por cron (no hay cron: no corren).
- **URLs sin reescritura.** Sin `htaccess.RewriteBase`, la página de login es `/index.php/login`.
