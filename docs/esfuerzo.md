# Registro de esfuerzo

| Componente | Tiempo | Notas |
|------------|--------|-------|
|            |        |       |

## Línea base B1

- Inicio: 2026-11-24 03:27 -03 (06:27 UTC)
- Fin: 2026-11-24 03:38 -03 (06:38 UTC)
- Iteraciones de corrección: 1, antes de la primera prueba: la lectura de la imagen configurada con `docker compose config --images gitea` también devolvía la de postgres (dependencia), y se reemplazó por la lectura de `services.gitea.image` del JSON de configuración. Pruebas de B1 en 00-healthy: 1 (pass a la primera, sin falsas alarmas).
- `git diff --cached --stat` del commit de B1 (sin este archivo):

```
 README.md                                          |  20 +
 baselines/b1/b1.sh                                 |  78 ++
 baselines/b1/checks.py                             | 394 +++++++++
 docs/preguntas-pendientes.md                       |  10 +
 faults/lib/runner.sh                               |   7 +-
 .../20261124T063110Z-r01/evidence/confirm.log      |   4 +
 .../20261124T063110Z-r01/evidence/pmset_sleep.txt  | 936 +++++++++++++++++++++
 .../00-healthy/20261124T063110Z-r01/result.json    | 126 +++
 .../faults/00-healthy/20261124T063110Z-r01/run.log | 220 +++++
 results/summary.md                                 |   2 +-
 10 files changed, 1794 insertions(+), 3 deletions(-)
```

## Etapa 3: motor de contratos y contrato de Gitea

Criterio de conteo, igual para todo (`docs/conteo/contar.py`): una línea cuenta si no está vacía y no empieza con `#` ni `//`; las docstrings de Python cuentan como código. Un supuesto es una línea con `# S:` (uno por línea aunque tenga varios). Lo específico de Gitea es todo `contracts/gitea.yaml` en el contrato y todos los scripts en B1, que no separa lo general de lo específico. Las marcas de B1 están en una copia de conteo (`docs/conteo/b1/`); B1 no se modificó.

### Motor (`engine/`)

- Inicio: 2026-11-25 12:49 -03 (15:49 UTC). Fin: 13:00 -03 (motor compilado y pruebas unitarias en verde), más la integración al arnés hasta la primera corrida (13:02 -03).
- Iteraciones:
  1. Compilación y `go vet`: sin errores en el primer intento.
  2. Pruebas unitarias: una ronda con 2 fallas, las dos en las pruebas mismas. La prueba de TOTP leía el encabezado de la última petición, y la prueba de independencia confundía `default_transaction_read_only` con `fault_`.
  3. Generalidad: `GIT_LFS_SKIP_SMUDGE=1` estaba en el código de la sonda y pasó a ser un parámetro (`env`) de `git_push` en el contrato.
  4. Compatibilidad con el arnés: marcas de tiempo con milisegundos, para el cálculo del runner.
  5. Integración: `pass` en la primera corrida de 00-healthy.
- Líneas sin comentarios: 2373 de código (sin pruebas, sin `vendor/`), 545 de pruebas unitarias, 41 del envoltorio y del script de compilación (`baselines/contract/contract.sh`, `engine/build.sh`).

### Contrato de Gitea (`contracts/gitea.yaml`)

- Inicio: 13:00 -03. Fin: 13:10 -03.
- Iteraciones:
  1. Primera versión: válida para el motor en el primer intento y `pass` en la primera corrida (`results/faults/00-healthy/20261125T160253Z-r01`, conservada).
  2. Revisión de marcas: `# S:` en todas las líneas con un supuesto, incluidas las repetidas, y la consulta SQL en una sola línea para poder marcarla. Nueva corrida, también `pass` (`results/faults/00-healthy/20261125T161115Z-r01`).
- Líneas sin comentarios: 161. Supuestos sobre Gitea (`# S:`): 31. (Primero se informaron 32: `contar.py` contaba también la línea de comentario del encabezado que explica la marca. Corregido: solo cuentan las líneas de código con `# S:`; B1 no cambia, porque sus marcas están solo en líneas de código.)

### B1, medido con el mismo criterio

- Tiempo: 11 min (03:27–03:38 -03 del 2026-11-24, ver arriba). Iteraciones: 1 corrección antes de la primera prueba y 1 prueba.
- Líneas sin comentarios: 384 (`b1.sh` 59, `checks.py` 325). Supuestos sobre Gitea (`# S:`): 20 (`b1.sh` 6, `checks.py` 14).
- Nota: contar por línea favorece al código compacto. El contrato repite supuestos en líneas separadas (por ejemplo, la autenticación básica en cada tipo de testigo) que B1 escribe una sola vez dentro de una función.

### Segundo conteo: supuestos distintos (criterio escrito antes de contar)

Criterio, fijado antes de hacer el conteo:

1. Un supuesto distinto es un hecho sobre Gitea (su comportamiento, su interfaz o la disposición de sus datos) que, si fuera falso en otra versión o instalación, obligaría a cambiar el contrato o B1.
2. El mismo hecho repetido en varias líneas cuenta una vez.
3. Una línea con varios hechos cuenta cada uno por separado.
4. Granularidad de "un hecho":
   - cada endpoint (ruta) de la API o de la web;
   - cada nombre de campo de una respuesta;
   - cada encabezado;
   - el método de autenticación de la API y el de Git por HTTP, por separado;
   - la forma de la URL de Git;
   - la ruta del archivo de configuración, su formato, la sección y el nombre de la clave, por separado;
   - el esquema de la base;
   - el modelo de datos que usa una consulta (tablas y columnas), uno por consulta;
   - la raíz de los repositorios en disco, el diseño de directorios y la regla de nombres, por separado;
   - el comando y el formato de salida de la versión, por separado;
   - el orden de arranque.
5. Solo se cuentan las líneas marcadas `# S:`, con la misma marca del primer conteo. En B1 se usa la copia de conteo `docs/conteo/b1/`.
6. No se cuentan los hechos que B1 hereda de B0 al ejecutarlo, porque B0 no es parte de los scripts de B1. Tampoco se cuentan los hechos sobre otras herramientas (Restic, PostgreSQL, MinIO).
7. La asignación de cada línea a sus hechos está en `docs/conteo/supuestos_distintos.py`, que falla si una línea `# S:` queda sin asignar.

Resultado del segundo conteo (`python3 docs/conteo/supuestos_distintos.py`), con el contrato después de la pregunta 5:

| | Líneas sin comentarios | Líneas `# S:` (medida principal) | Supuestos distintos |
|---|---|---|---|
| Contrato de Gitea | 173 | 35 | 29 |
| B1 (`b1.sh` + `checks.py`) | 384 | 20 | 25 |

Los 25 supuestos distintos de B1 también están en el contrato. Solo en el contrato hay 4:
- el orden de arranque;
- la página principal;
- el endpoint de búsqueda de repositorios;
- el campo `ok` de esa búsqueda.

B1 no comprueba el orden de arranque. Los otros tres los hereda de B0, que no se cuenta.

### Pregunta 5 (opción B): dos verificaciones más en el contrato

- `home_page` (GET `/`, 200) y `repo_list` (GET `/api/v1/repos/search?limit=50` autenticado, 200 y `ok: true`), sumadas a `requires` del RTO. El motor no cambió.
- Limitación: B0 exige además que la búsqueda devuelva al menos un repositorio. El vocabulario del motor no tiene una aserción de tamaño de lista, así que el contrato comprueba el estado y el campo `ok`, no la cantidad.
- 00-healthy: `pass`, 23 de 23 (`results/faults/00-healthy/20261125T163844Z-r01`). RTO 13,5 s (antes 13,3 s); fase pre 1,1 s; fase post 2,2 s.
- Esfuerzo: 1 iteración, unos 5 min. Contrato: 173 líneas sin comentarios, 35 líneas `# S:`.

## Etapa 5: port a Nextcloud

Mismo criterio de conteo que en la etapa 3 (`docs/conteo/contar.py`): líneas sin comentarios y líneas
con `# S:` (supuestos sobre Nextcloud). Tiempos en UTC del 2026-11-26, tomados al empezar y al
terminar cada port. El entorno (`env-nc-v1`) y las fallas nc-01 a nc-04 se hicieron y se commitearon
antes de empezar a portar (`6bed8ce`, `9866c4d`).

Advertencia sobre los tiempos: el mismo agente escribió B1, el motor y los dos ports, y leyó el código
de B1 y el vocabulario del motor mientras esperaba otras corridas (antes de empezar a medir). Los
tiempos miden la escritura y la prueba, no el aprendizaje. Gran parte de cada tiempo es la espera de la
corrida sana de prueba (unos 6,5 min cada una, con los 5 min de testigos posteriores).

### B1 (`baselines/b1-nc/`, copia de `b1-v1`)

- Inicio: 06:30:42. Fin: 06:46:04 (`pass` en la segunda corrida sana). Total: 15 min 22 s.
- Iteraciones:
  1. Primera versión (06:32): `fail` en la sana (`results/faults/nc-00-healthy/20261126T063204Z-r01`,
     conservada) por `table_row_counts_vs_inventory`: B0 escribe en la base antes de que B1 cuente
     filas (`oc_filecache` +2 por la appdata de theming al servir la página de login, `oc_preferences`
     +1 por el último login). En Gitea B0 no escribe.
  2. El conteo por tabla deja fuera esas dos tablas (supuesto marcado); los archivos de usuario se
     comparan aparte (base contra disco y disco contra inventario). `pass`
     (`results/faults/nc-00-healthy/20261126T063910Z-r01`).
- Líneas cambiadas (`git diff --stat b1-v1:baselines/b1 5acea8e:baselines/b1-nc`):

```
 b1.sh     |  45 ++++++------
 checks.py | 229 ++++++++++++++++++++++++--------------------------------------
 2 files changed, 112 insertions(+), 162 deletions(-)
```

- Líneas sin comentarios: 336 (`b1.sh` 58, `checks.py` 278); B1 de Gitea: 384.
- Supuestos sobre Nextcloud (`# S:`): 17 (`b1.sh` 6, `checks.py` 11).
- Qué cambió: imagen y versión antes de B0 (versión desde `version.php` de la imagen), B0 de
  Nextcloud, estado de `status.php` (nuevo: `installed`, `maintenance`, `needsDbUpgrade`), huella de
  `secret` de `config.php` en lugar de `SECRET_KEY`, archivos de usuario en la base contra el disco y
  contra el inventario en lugar de repositorios y refs, testigos por WebDAV (archivo, binario y
  compartido, este visto por el destinatario), PUT de WebDAV en lugar de `git push`. Sin TOTP (la
  instalación no tiene la app de segundo factor). Se mantienen sin cambios la autoprueba y la búsqueda
  de errores en el log, la codificación, las dependencias externas, RTO y RPO.

### Contrato (`contracts/nextcloud.yaml`, motor `engine-v1` sin cambios)

- Inicio: 06:46:51. Fin: 07:00:59 (`pass` en la segunda corrida sana). Total: 14 min 8 s.
- Iteraciones:
  1. Primera versión, válida para el motor en el primer intento (`engine check-contract`): `fail` en
     la sana (`results/faults/nc-00-healthy/20261126T064725Z-r01`, conservada) por `files_db_vs_disk`:
     el motor separa por espacios los nombres que devuelve la consulta, y los archivos de ejemplo del
     administrador tienen espacios ("Nextcloud Manual.pdf"); en disco no los separa.
  2. Consulta y glob limitados a los usuarios del seed (cuyos archivos no tienen espacios). `pass`, 19
     de 19 (`results/faults/nc-00-healthy/20261126T065422Z-r01`).
- Líneas: el contrato es un archivo nuevo, 136 líneas sin comentarios (Gitea: 173). Envoltorio
  (`baselines/contract-nc/contract.sh`, copia del v2): `git diff --stat 2ca370a 939c0ec --
  baselines/contract-nc` = 5 inserciones, 4 eliminaciones; 26 líneas sin comentarios.
- Supuestos sobre Nextcloud (`# S:`): 21 (Gitea: 35).
- `custom_check`: 1 (`write_probe`: escritura y lectura de un archivo con `occ files:put` y
  `files:get` como www-data).
- `git diff engine-v1 -- engine/`: vacío (motor sin cambios).

### Lo que Nextcloud necesitaría cambiar en el motor (criterio de descarte 3)

No se cambió el motor. Estas son las cosas que el vocabulario de `engine-v1` no puede expresar, con lo
que se hizo en cada caso:

1. **Huella de un secreto en un archivo PHP.** `secret_fingerprint` lee `ini`, `env` o `raw`;
   `config.php` es un arreglo PHP. `custom_check` no sirve de reemplazo: su comando no resuelve
   referencias (`${run:declared.json#…}`) y corre dentro del contenedor, sin acceso a la corrida. El
   contrato **no verifica** la huella de `secret` (B1 sí). Necesitaría un formato nuevo o un comando
   con valor esperado.
2. **Escritura por HTTP con cuerpo.** `http_request` no manda cuerpo (no hay PUT con datos) y
   `git_push` es solo para Git. Se usó un `custom_check` (cuenta como derrota).
3. **Nombres con espacios en `entities_match_disk`.** El motor separa por espacios la salida de la
   consulta. Se limitó la comparación a los usuarios del seed: los archivos de ejemplo del
   administrador quedan fuera de esa verificación.
4. **Autenticación por testigo.** `auth` no admite plantillas `{campo}`. Alcanzó con usuarios fijos
   porque los testigos son de `alice` y se comparten con `bob`; con testigos de varios usuarios no
   alcanzaría.
5. **Encabezados.** `http_request` no manda encabezados, y la API OCS de Nextcloud exige
   `OCS-APIRequest: true`. El contrato usa WebDAV y `status.php`, que no lo exigen; no hay
   verificación por OCS (B0 y B1 sí la hacen en el login).
