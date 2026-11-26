# Preguntas pendientes

Decisiones que no están cubiertas por lo ya aprobado. Ninguna se tomó; el trabajo siguió con lo que no depende de ellas.

## 1. Falla 08: Gitea 28.4.0 regenera los hooks al arrancar

**Decidido: opción A.** El 08 queda `invalid` y se registra en "Hallazgos". Sin desviaciones nuevas.

**Qué pasó.** En las dos corridas del escenario 08 (`results/faults/08-binary-path-changed/`) los 27 hooks restaurados apuntaban a `/usr/local/bin/gitea` antes del primer arranque (`evidence/hooks_before_first_start.txt`). Después del arranque apuntan a `/opt/gitea/bin/gitea`. El log de Gitea muestra:

```
routers/init.go:97:syncAppConfForGit() [I] re-sync repository hooks ...
```

Gitea detecta al arrancar que cambió la ruta del ejecutable y reescribe los hooks. La falla "sin regenerar hooks" no ocurre, y `confirm` deja las dos corridas en `invalid`. Se conservan.

**Opciones:**

- **A.** Dejar el 08 como `invalid`: la falla no se puede reproducir con Gitea 28.4.0. Registrarlo como hallazgo ("Gitea 28 regenera los hooks al arrancar si cambia la ruta del ejecutable").
- **B.** Mantener la falla con una desviación adicional en el escenario que impida la regeneración. Por ejemplo, volver a escribir los hooks viejos después del primer arranque, simulando que la regeneración no ocurrió. La falla cambiaría de "otra ruta sin regenerar hooks" a "hooks desactualizados después del arranque".
- **C.** Otra variante que definas vos.

Mientras no decidas, el 08 queda `invalid` en `results/summary.md`.

## 2. SECRET_KEY vacía en el entorno original

**Decidido: opción B, con condiciones.** `make up` genera la clave una vez y la escribe solo en `app.ini` (entorno v2). B0 no cambia. Los resultados anteriores quedan como entorno v1 y la parte C se repite en v2.

**Qué pasó.** Al probar B1 en `00-healthy` (`results/faults/00-healthy/20261124T063110Z-r01`), la huella de `SECRET_KEY` declarada y la que está en uso son iguales (`e3b0c44298fc…`), pero esa es la sha256 de un texto vacío. En `app.ini`, `[security] SECRET_KEY =` está vacía. El entorno de la etapa 1 arranca Gitea con `INSTALL_LOCK=true` y sin `SECRET_KEY`, y Gitea 28 no la genera (sí genera `INTERNAL_TOKEN` y los JWT). Con la clave vacía, Gitea usa su clave interna por defecto, la misma en todas las instalaciones. El check de B1 cumple la especificación (compara la huella declarada con la que está en uso) y detecta un cambio como el de la falla 07, que sigue siendo válida. Pero en una corrida sana compara vacío contra vacío.

**Opciones:**

- **A.** Dejarlo así y documentar que la clave declarada está vacía y Gitea usa la clave interna por defecto.
- **B.** Fijar una `SECRET_KEY` de prueba en `.env.example` y en `compose/original.yml` y `compose/restore.yml`. Cambia el entorno congelado con B0 (`b0-v1`) y obliga a repetir la parte C.
- **C.** Que B1 además falle si la clave declarada está vacía. Cambia el criterio de B1 y daría fail en todas las corridas actuales.

## 3. Falla 01 en v2: no hay evidencia directa de la SECRET_KEY restaurada

**Resuelto (opción B).** Se repitió el 01 con B0 en v2 con un `[obs]` de la huella en uso (`results/faults/01-appini-excluded/20261125T073123Z-r01`). La clave en uso está vacía y la huella difiere de la declarada. Ver "Hallazgos" en `results/summary.md`.

**Qué pasó.** Antes de correr B1 en v2 había que confirmar, con la evidencia del 01 en v2 (`results/faults/01-appini-excluded/20261124T154829Z-r01`), que la `SECRET_KEY` restaurada es distinta de la declarada. Esa corrida no registró la clave en uso: `confirm` solo verifica que el snapshot no tiene `app.ini` y que el volumen restaurado no tenía ningún `.ini` antes del primer arranque. El volumen de restauración de esa corrida ya no existe; el actual es de la corrida de B1 en 00-healthy.

Evidencia disponible, indirecta:
- Huella declarada al hacer el backup: `5d4ba5960562…` (`runs/20261124T154831Z/declared.json`), no vacía.
- El volumen restaurado no tenía ningún `.ini` antes del primer arranque (`evidence/ini_files_after_restore.txt`).
- El entorno de restauración no define `GITEA__security__SECRET_KEY` (`[obs]` en `evidence/confirm.log`). Por lo tanto, la imagen crea `app.ini` desde su plantilla con `SECRET_KEY` vacía (`e3b0c442…`).

No se corrió nada de B1. La tanda espera esta decisión.

**Opciones:**

- **A.** Aceptar la evidencia indirecta y empezar la tanda de B1.
- **B.** Repetir el 01 con B0 en v2, agregando al `confirm` del 01 una observación (`[obs]`, no un criterio) con la huella de la `SECRET_KEY` en uso. La corrida actual se conserva. Si las huellas difieren, empezar la tanda.
- **C.** Otra forma que definas vos.

## 4. Error de B1: `repositories_db_vs_disk` informa "0 en disco" cuando no pudo leer el disco

**Qué pasó.** En la tanda de B1 en v2 (`b1-v1` sin taggear), cuando Gitea no está en marcha después de la restauración, `b1.sh` no puede listar los repositorios del disco: `b1/raw/disk_refs.err` contiene `service "gitea" is not running` y `disk_refs.txt` queda vacío. `checks.py` solo distingue el caso en que faltan la base **y** el disco ("no se pudo leer la base ni el disco restaurados"). Si falta solo el disco, lo toma como una lista vacía e informa, por ejemplo, `9 en la base, 0 en disco; solo en base [...]`.

Corridas afectadas: `results/faults/11-minio-bucket-excluded/20261125T094017Z-r01` y `results/faults/14-latin1-database/20261125T103716Z-r01`. En las dos el paso falla, que es el resultado correcto (y B1 ya había fallado en `b0`); lo incorrecto es el detalle, que afirma algo que no se midió. No cambia ningún resultado ni el paso de la primera falla. No afecta a las restauraciones sanas.

B1 no se modificó durante la tanda. La corrección iría en una versión nueva de B1. **Opciones:**

- **A.** Corregirlo en B1 v2: si `disk_refs.err` no está vacío o Gitea no está en marcha, informar "no se pudo leer el disco restaurado" (falla igual), y revisar el mismo caso en `refs_vs_inventory`.
- **B.** Dejarlo documentado en B1 v1 sin versión nueva.

## 5. Contrato: qué verificaciones definen el fin de la recuperación para el RTO

**Decidido: opción B.** Se agregaron `home_page` y `repo_list` al contrato y a `requires` antes de congelar (`contract-gitea-v1`).

**Qué pasó.** Para el RTO, B1 toma el fin de sus pruebas funcionales: B0 completa (incluye la página principal, la versión, el login por API, la lista de repositorios y un clon) más adjuntos, LFS, issues, TOTP y push. El contrato usa la misma definición (desde `restore_started_at` hasta el fin de las verificaciones requeridas, todas en `pass`), con `requires: [components_up, version_running, witness_attachment, witness_lfs, witness_issue, totp_login, git_push]`. El login por API y el clon quedan cubiertos (los testigos usan la API autenticada; `git_push` clona antes de empujar), pero la página principal y la lista de repositorios no están en el contrato. En 00-healthy: RTO del contrato 13,3 s; B1 en las 11 sanas v2, 15,4–16,8 s (media 16,3). Parte de la diferencia es que B1 cuenta también la duración de las pruebas de B0.

No bloquea nada: el motor y el contrato están listos para congelar tal como están.

**Opciones** (antes de congelar, porque después cualquier cambio es una versión nueva):

- **A.** Congelar así.
- **B.** Agregar al contrato dos `http_request` (página principal y lista de repositorios) y sumarlas a `requires`, sin cambiar el motor.

## 6. Falla 14 con el contrato: `invalid` porque el log de `restore.sh` no llega a `run.log`

**Decidido: opción A.** Envoltorio v2: `baselines/contract/contract.sh` copia la salida de `restore.sh` también a stderr con `tee` (llega a `run.log`), sin otros cambios; `engine/` y `contracts/` siguen idénticos a `engine-v1` y `contract-gitea-v1`. El 14 se repitió solo con el contrato desde `make reset`: `results/faults/14-latin1-database/20261126T042333Z-r01`, `confirm` válido, contrato `fail` (primer paso `components_up`), sin suspensión. La corrida `invalid` anterior se conserva.

**Qué pasó.** En la tanda del contrato (`engine-v1`, `contract-gitea-v1`), la corrida `results/faults/14-latin1-database/20261125T201048Z-r01` quedó `invalid` ("confirm no se cumplió"). El `confirm` del 14 busca `has no equivalent in encoding "LATIN1"` en `$FAULT_RUN_LOG` (`run.log` del resultado). Con B0 y B1 la salida de `pg_restore` va a ese log. El envoltorio del contrato (`baselines/contract/contract.sh`) redirige la salida de `scripts/restore.sh` a `runs/<id>/contract/restore.log`, que no se copia a `run.log`. La falla sí ocurrió: ese archivo tiene el mensaje dos veces y `pg_restore: warning: errors ignored on restore: 2`; las otras verificaciones del `confirm` (LATIN1, carga sin `--exit-on-error`) pasaron. El contrato dio `fail` (primer paso `components_up`; también fallaron `restore_tool_errors` y `database_encoding`).

El error está en el envoltorio, no en el motor ni en el contrato. No se modificó nada durante la tanda; la corrida se conserva y cuenta como `invalid`. Ninguna otra falla del contrato depende del log de la restauración en `run.log` (las demás quedaron válidas, el 08 es `invalid` por la causa ya conocida).

**Observación menor (motor).** Cuando un servicio no tiene contenedor, el motor informa `sin contenedor para gitea:` seguido de la última línea de stderr de `docker compose ps`. Con el shim activo esa línea es la traza `[shim] … ps -a -q gitea`, no un mensaje de Docker (14 y 15). El estado (`error`) y el veredicto no cambian; solo el detalle es poco útil.

**Opciones:**

- **A.** Corregir el envoltorio (versión nueva, sin tocar motor ni contrato): copiar la salida de `restore.sh` también a stderr (`tee`), para que llegue a `run.log`, y repetir solo el 14 con el contrato desde `make reset`.
- **B.** Dejar el 14 del contrato como `invalid` y documentarlo.

