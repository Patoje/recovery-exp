# fallas-ocultas.md

## Convenciones comunes

Estas convenciones valen para las cuatro fallas. Todo vive en un único host de laboratorio bajo `$LAB`. No hace falta tocar nada fuera de los proyectos Docker Compose del laboratorio ni del repositorio de Restic del laboratorio.

```bash
export LAB=/srv/lab
export RESTIC_REPOSITORY=$LAB/restic-repo
export RESTIC_PASSWORD_FILE=$LAB/restic.pass
export OWNER=lab REPO=testigos          # repositorio donde viven los testigos
export URL_ORIG=http://127.0.0.1:3000   # Gitea de origen (proyecto compose "orig")
export URL_REST=http://127.0.0.1:3100   # Gitea restaurado (proyecto compose "rest")
export TOKEN=...                        # token de API de un usuario de prueba, creado en origen ANTES del backup
# $LAB/backup-ref.sh <dir-stack>        -> procedimiento de backup de referencia
# $LAB/restore-ref.sh <snapshot> <dir>  -> restauración aislada de referencia
# $WID                                  -> identificador único de la tanda de testigos
```

- **Rutas.** Se usa el layout de la imagen oficial `gitea/gitea` (no rootless): `/data/gitea/conf/app.ini`, `/data/gitea/attachments`, `/data/git/repositories`. Si la imagen cambia, hay que ajustar las rutas.
- **Restic.** Se fija una única versión de Restic para todo el experimento.

---

## Falla A: Secuencias de PostgreSQL desfasadas tras la restauración

### 1. Nombre y clase

- **Nombre:** «Datos completos, secuencias en cero».
- **Clase:** datos (integridad de la base).

Todas las filas se restauran y los conteos coinciden con el inventario. Sin embargo, las secuencias que generan los `id` vuelven a 1. El servicio arranca y toda lectura funciona. La primera escritura que necesita un `id` nuevo, sea un issue, un comentario, una acción del feed o un usuario, falla por clave duplicada.

**Causa realista.** El operador recupera la base por un camino que copia filas pero no el estado de las secuencias. Dos ejemplos:

- Promover una réplica por replicación lógica, que no replica secuencias.
- Migrar con una herramienta de exportación por tabla (CSV, ETL) hacia un esquema creado de antemano por `gitea migrate`.

La inyección reproduce el estado final de esos caminos.

### 2. Inyección reproducible

1. Ejecutar el ciclo normal:
   1. crear los testigos «antes»,
   2. correr `backup-ref.sh`,
   3. crear los testigos «después»,
   4. correr `restore-ref.sh latest $LAB/rest`,
   5. levantar el proyecto `rest`.
2. Con el proyecto `rest` en marcha, reiniciar todas las secuencias del esquema `public`:

```bash
docker compose -p rest exec -T db psql -U gitea -d gitea -v ON_ERROR_STOP=1 <<'SQL'
DO $$
DECLARE r record;
BEGIN
  FOR r IN SELECT schemaname, sequencename FROM pg_sequences WHERE schemaname = 'public' LOOP
    EXECUTE format('SELECT setval(%L::regclass, 1, false)',
                   format('%I.%I', r.schemaname, r.sequencename));
  END LOOP;
END $$;
SQL
```

3. Reiniciar Gitea para descartar estado en memoria: `docker compose -p rest restart gitea`.

### 3. Condición objetiva de confirmación

La falla se confirma si se cumplen las tres condiciones siguientes.

**a) Hay al menos una secuencia cuyo próximo valor es menor o igual que `max(id)` de su tabla, y entre ellas está la de `issue`.** Esta consulta lo verifica:

```bash
docker compose -p rest exec -T db psql -U gitea -d gitea <<'SQL'
DO $$
DECLARE r record; m bigint; nxt bigint; n int := 0;
BEGIN
  FOR r IN
    SELECT t.relname AS tbl, a.attname AS col, s.relname AS seq
    FROM pg_class s
    JOIN pg_depend d    ON d.objid = s.oid AND d.classid = 'pg_class'::regclass AND d.deptype IN ('a','i')
    JOIN pg_class t     ON t.oid = d.refobjid
    JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = d.refobjsubid
    WHERE s.relkind = 'S'
  LOOP
    EXECUTE format('SELECT coalesce(max(%I),0) FROM %I', r.col, r.tbl) INTO m;
    EXECUTE format('SELECT CASE WHEN is_called THEN last_value+1 ELSE last_value END FROM %I', r.seq) INTO nxt;
    IF m >= nxt THEN n := n + 1; RAISE NOTICE 'desfasada: %.% max=% proximo=%', r.tbl, r.col, m, nxt; END IF;
  END LOOP;
  RAISE NOTICE 'secuencias_desfasadas=%', n;
END $$;
SQL
```

**b) La creación de un issue por API devuelve un código 5xx:**

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST "$URL_REST/api/v1/repos/$OWNER/$REPO/issues" \
  -H "Authorization: token $TOKEN" -H 'Content-Type: application/json' \
  -d "{\"title\":\"sonda-$(cat /proc/sys/kernel/random/uuid)\"}"
```

**c) El log de Gitea o de PostgreSQL contiene `duplicate key value violates unique constraint` para una clave primaria:**

```bash
docker compose -p rest logs db gitea | grep 'duplicate key value violates unique constraint'
```

Además, como control, el inventario de conteos por tabla coincide con el de referencia. Ese control demuestra que no se perdieron filas.

### 4. Por qué no repite ninguna conocida

- **No es la 14.** No se pierde ninguna tabla ni ninguna fila, y la codificación es idéntica. Los conteos coinciden.
- **No es la 2 ni la 10.** Las credenciales funcionan, la conexión a la base funciona y la versión de Gitea es la misma.
- **No es la 3 ni la 6.** No hay inconsistencia temporal entre base y repositorios: el dump es coherente.
- **No es una exclusión del backup.** El dump contiene las secuencias correctas. El daño ocurre en el camino de recuperación de la base.

### 5. Qué debería detectar un verificador general

- **Lecturas sanas, escrituras rotas.** Las sondas de solo lectura (páginas, API GET, clones) pasan. Una sonda de escritura de punta a punta sobre la instancia restaurada falla: crear un issue, comentar o subir un adjunto.
- **Errores de clave duplicada en claves primarias** en los logs de la aplicación o de la base, en una instancia recién restaurada que todavía casi no recibió tráfico.
- **Contadores desfasados.** En una o más tablas, el siguiente identificador que entregaría la base es menor o igual que el identificador más alto ya existente.
- **Conteos que coinciden pero no alcanzan.** Que el conteo de filas coincida con la referencia no alcanza para declarar la restauración sana.

> **Supuestos a verificar**
> - Gitea (vía xorm) usa columnas `BIGSERIAL` con secuencias ligadas por dependencia `'a'` en PostgreSQL. Si alguna versión usa columnas `IDENTITY`, la consulta las cubre con `'i'`.
> - La replicación lógica de PostgreSQL no replica el estado de las secuencias. Esto es cierto hasta PostgreSQL 17. Hay que verificarlo en la versión usada si se cita como causa.
> - Que la API devuelva exactamente 500 depende de la versión. La condición es «5xx», más el mensaje en el log.

---

## Falla B: La instancia restaurada sigue usando el almacenamiento de objetos original

### 1. Nombre y clase

- **Nombre:** «Restauración que se apoya en el original».
- **Clase:** dependencia externa.

En el escenario con MinIO, el bucket sí está en el backup y sí se restaura en un MinIO propio del entorno restaurado. Pero el `app.ini` restaurado tal cual sigue apuntando al endpoint de MinIO de origen, y desde el entorno restaurado ese endpoint es alcanzable. Por eso:

- todo funciona y los adjuntos y el LFS se sirven bien;
- en realidad se sirven desde el original;
- lo que se escribe en la instancia restaurada contamina el original.

En un desastre real, con el original caído, la restauración no sirve.

**Causa realista.** El endpoint configurado es un nombre DNS o una IP de la red interna, por ejemplo `s3.casa.lan`, y el host de restauración vive en la misma red.

### 2. Inyección reproducible

Se usan dos pilas del laboratorio:

- `orig`, con su propio `minio-orig` en la red Docker `lab_orig`;
- `rest`, con su propio servicio `minio` en la red `rest_default`.

El `app.ini` de origen tiene `[storage]` con `STORAGE_TYPE = minio` y `MINIO_ENDPOINT = minio-orig:9000`.

1. Hacer el ciclo normal, con testigos antes y después del backup. El backup incluye el contenido del bucket, por ejemplo con un `mc mirror` hacia un directorio que entra en Restic.
2. Restaurar con `restore-ref.sh`, que carga el bucket en el `minio` de `rest`. **No** se edita `MINIO_ENDPOINT`: el `app.ini` queda tal cual se restauró.
3. Dar alcance de red desde el Gitea restaurado hacia la red de origen, para simular que el host de restauración comparte red o DNS con producción:

```bash
docker network connect lab_orig rest-gitea-1
docker compose -p rest restart gitea
```

### 3. Condición objetiva de confirmación

La falla se confirma si se cumplen las tres condiciones siguientes.

**a) Escritura que aterriza en el original.**

1. Subir un adjunto sonda a un issue en la instancia restaurada:

```bash
U=$(curl -s -X POST "$URL_REST/api/v1/repos/$OWNER/$REPO/issues/1/assets" \
      -H "Authorization: token $TOKEN" -F "attachment=@/etc/hostname;filename=sonda-$WID.txt" | jq -r .uuid)
```

2. Comprobar en qué MinIO quedó el objeto: debe existir en `minio-orig` y **no** en el `minio` de `rest`.

```bash
docker run --rm --network lab_orig minio/mc sh -c \
  "mc alias set o http://minio-orig:9000 \$AK \$SK >/dev/null && mc ls -r o/gitea/ | grep $U"   # debe encontrarlo
docker run --rm --network rest_default minio/mc sh -c \
  "mc alias set r http://minio:9000 \$AK \$SK >/dev/null && mc ls -r r/gitea/ | grep $U"        # no debe encontrarlo
```

**b) Lectura que depende del original.**

1. Cortar el alcance hacia el original y reiniciar Gitea:

```bash
docker network disconnect lab_orig rest-gitea-1
docker compose -p rest restart gitea
```

2. Pedir el adjunto testigo:

```bash
curl -s -o /dev/null -w '%{http_code}\n' "$URL_REST/attachments/<uuid-testigo>"
```

El resultado debe ser distinto de 200, o directamente Gitea no arranca porque no inicializa el almacenamiento. Mientras tanto, el objeto testigo sigue existiendo en el `minio` de `rest`, lo que se comprueba con `mc stat`.

**c) Estado previo al corte.** Antes del corte, en el paso 3 de la inyección, la descarga del mismo testigo devolvía 200 con el hash del inventario.

### 4. Por qué no repite ninguna conocida

- **No es la 11.** El bucket sí está en el backup y sí se restaura correctamente. El problema es a qué instancia se conecta la restauración, no qué se copió.
- **No es la 1 ni la 7.** El `app.ini` está presente y es el original, y la `SECRET_KEY` es la correcta. Justamente, restaurarlo fielmente es lo que produce la falla.
- **No es la 4 ni la 5.** Adjuntos y LFS existen en la copia restaurada.

### 5. Qué debería detectar un verificador general

- **Tráfico fuera del perímetro.** Durante la verificación, la instancia restaurada genera tráfico hacia hosts que no forman parte del conjunto restaurado: almacenamiento de objetos, base de datos, caché o SMTP.
- **Funcionalidad condicionada al original.** Los adjuntos y el LFS funcionan solo mientras el sistema original es alcanzable, y dejan de funcionar al aislar por completo la red de la restauración.
- **Escrituras que aparecen en el original.** Los objetos o registros creados en la instancia restaurada aparecen en el almacenamiento original y no en el restaurado.
- **Endpoints fuera del inventario.** La configuración efectiva tiene endpoints que no coinciden con ninguno de los componentes levantados por la restauración.
- **Variante equivalente:** `[database] HOST` apuntando a la base original. Se manifiesta igual: la base restaurada no recibe consultas.

> **Supuestos a verificar**
> - En las versiones con `[storage]`, Gitea guarda adjuntos bajo `attachments/<uuid[0]>/<uuid[1]>/<uuid>`, con el prefijo `MINIO_BASE_PATH`.
> - Si el endpoint no es alcanzable al arrancar, según la versión Gitea puede no iniciar o iniciar y fallar al servir. Ambos resultados confirman la dependencia.
> - La ruta de API `/repos/{owner}/{repo}/issues/{index}/assets` existe en Gitea ≥ 1.17.

---

## Falla C: Restauración coherente de la instancia equivocada

### 1. Nombre y clase

- **Nombre:** «`latest` es de otro».
- **Clase:** otro (identidad y selección del punto de restauración).

Dos instancias comparten el mismo repositorio de Restic, las mismas rutas y la misma contraseña:

- producción;
- un staging clonado de producción hace un tiempo.

El staging hace su backup más tarde que producción. La restauración usa `restic restore latest` sin filtrar por host o etiqueta y recupera el staging.

El resultado es una instancia completamente sana y autoconsistente: misma `SECRET_KEY`, mismos usuarios y repositorios plausibles. Pero no es la que se quería recuperar.

### 2. Inyección reproducible

1. Correr el clon de staging dentro del laboratorio:
   1. Restaurar un snapshot viejo de producción en `$LAB/b`.
   2. Levantarlo como proyecto `stg` en otro puerto.
   3. Crear allí un canario único: un issue `CANARIO-STAGING-<uuid>`.
2. Para que ambas instancias produzcan snapshots con rutas idénticas, Restic corre en contenedor y monta cada pila en `/srv/gitea`:

```bash
r() { docker run --rm -v $LAB/restic-repo:/repo -v $LAB/restic.pass:/pass:ro \
        -e RESTIC_REPOSITORY=/repo -e RESTIC_PASSWORD_FILE=/pass "$@"; }
```

   Dentro de `backup-ref.sh`, el paso de Restic queda así:

```bash
r -v "$STACK":/srv/gitea:ro restic/restic:<ver> backup --host "$RESTIC_HOST" /srv/gitea
```

3. Hacer el ciclo normal de producción, con testigos antes y después:

```bash
RESTIC_HOST=gitea-prod $LAB/backup-ref.sh $LAB/a
```

4. **Después** del backup de producción, correr el de staging:

```bash
RESTIC_HOST=gitea-staging $LAB/backup-ref.sh $LAB/b
```

5. Restaurar como lo haría un runbook sin filtros:

```bash
r -v $LAB/rest:/restore restic/restic:<ver> restore latest --target /restore
```

   Luego levantar el proyecto `rest`.

### 3. Condición objetiva de confirmación

La falla se confirma si se cumplen las cuatro condiciones siguientes.

**a) El snapshot usado no es de producción:**

```bash
r restic/restic:<ver> snapshots latest --json | jq -r '.[0].hostname'
```

La salida es `gitea-staging`, y el ID coincide con el snapshot restaurado.

**b) Los testigos de producción no están y el canario de staging sí:**

```bash
docker compose -p rest exec -T db psql -U gitea -d gitea -Atc \
 "SELECT (SELECT count(*) FROM issue WHERE name LIKE '%$WID%'), (SELECT count(*) FROM issue WHERE name LIKE 'CANARIO-STAGING-%');"
```

El resultado esperado es `0|1`.

**c) Las refs no coinciden.** Las refs del repositorio de testigos difieren del inventario de referencia: falta el commit testigo.

**d) La instancia está sana.** Gitea responde 200 en `/api/healthz`, y el inicio de sesión y la navegación funcionan.

### 4. Por qué no repite ninguna conocida

- **No es la 13.** Ningún trabajo falla y el snapshot elegido es reciente. Su edad está dentro del RPO.
- **No es la 6.** El snapshot es internamente consistente: base, repositorios y archivos son de la misma instancia y del mismo momento.
- **No es la 15.** `restic check --read-data` pasa.
- **No es una exclusión de directorios.** Todo lo que se esperaba está, pero pertenece a otra fuente.

### 5. Qué debería detectar un verificador general

- **Metadatos del snapshot ajenos a la fuente.** El host, las rutas, las etiquetas o el ID del snapshot restaurado no corresponden a la fuente declarada que se quería recuperar.
- **Testigos ausentes, datos ajenos presentes.** Ninguno de los testigos de la fuente está en la instancia. Sí hay datos que nunca existieron en la fuente: canarios, issues o usuarios que no figuran en el inventario.
- **Divergencias amplias con el inventario.** Los conteos por tabla y las refs difieren del inventario de referencia en muchas tablas y repositorios a la vez, no en un punto aislado.
- **Hallazgo grave pese a la salud.** El reporte de salud del servicio es perfecto mientras la verificación de identidad falla. El sistema debe tratar esa combinación como un hallazgo grave, no como un éxito con advertencias.

> **Supuestos a verificar**
> - `restic restore latest` y `restic snapshots latest`, sin `--host`, `--path` ni `--tag`, eligen el snapshot más nuevo entre todos los hosts del repositorio. Hay que confirmarlo en la versión fijada.

---

## Falla D: Corrupción silenciosa en origen propagada a toda la ventana de retención

### 1. Nombre y clase

- **Nombre:** «Todos los snapshots están bien guardados y todos están mal».
- **Clase:** objetivo de recuperación (retención), con origen en datos.

Un objeto Git y un adjunto se corrompen en el disco de origen. Los backups siguientes terminan sin error y guardan fielmente el contenido ya corrupto. La política de retención termina por eliminar el único snapshot limpio.

El repositorio de Restic está íntegro y cualquier snapshot restaura sin error, y Gitea arranca. Pero no queda ningún punto de restauración que contenga los datos correctos.

### 2. Inyección reproducible

1. **Testigos y snapshot S1 (el último limpio).** Crear los testigos de la tanda `$WID` con un único push de un commit chico, para que el blob quede suelto. Después correr el backup:

```bash
RESTIC_HOST=gitea-prod $LAB/backup-ref.sh $LAB/a
```

2. **Corromper el blob testigo dentro del contenedor**, como usuario `git`:

```bash
BLOB=$(docker compose -p orig exec -T -u git gitea \
        git -C /data/git/repositories/$OWNER/$REPO.git rev-parse "HEAD:testigo-$WID.txt")
P=/data/git/repositories/$OWNER/$REPO.git/objects/${BLOB:0:2}/${BLOB:2}
docker compose -p orig exec -T -u git gitea sh -c \
  "test -f $P && chmod u+w $P && printf '\377\377\377\377' | dd of=$P bs=1 seek=8 conv=notrunc 2>/dev/null && chmod a-w $P"
```

   Si `test -f` falla, el objeto quedó empaquetado. En ese caso, repetir el paso 1 con un push que genere menos objetos que `transfer.unpackLimit`.

3. **Corromper el adjunto testigo**, mismo tamaño y contenido aleatorio. No hay que preservar el `mtime`: así Restic relee el archivo.

```bash
UUID=$(docker compose -p orig exec -T db psql -U gitea -d gitea -Atc \
        "SELECT uuid FROM attachment WHERE name='testigo-$WID.bin'")
A=/data/gitea/attachments/${UUID:0:1}/${UUID:1:1}/$UUID
docker compose -p orig exec -T -u git gitea sh -c \
  "head -c \$(stat -c%s $A) /dev/urandom > /tmp/x && cat /tmp/x > $A && rm /tmp/x"
```

4. **Tres backups normales más:** S2, S3 y S4. Cada uno termina con código 0.
5. **Aplicar la retención:**

```bash
restic forget --host gitea-prod --keep-last 3 --prune
```

### 3. Condición objetiva de confirmación

La falla se confirma si se cumplen las cuatro condiciones siguientes.

**a) El único snapshot limpio ya no existe:**

```bash
restic snapshots --host gitea-prod --json | jq -r '.[].short_id'
```

La salida no incluye S1.

**b) El repositorio de Restic está íntegro:**

```bash
restic check --read-data
```

Termina con código 0.

**c) Todos los snapshots que quedan tienen los testigos corruptos.** Para **cada** snapshot restante:

1. Restaurar en un directorio temporal.
2. Correr `git fsck --full` sobre `$OWNER/$REPO.git`. Termina con código distinto de 0 y reporta el blob `$BLOB` como corrupto.
3. Calcular el `sha256` del adjunto testigo. Es distinto del hash registrado en el inventario de referencia.

**d) El servicio responde pese al daño.** Tras restaurar cualquiera de esos snapshots:

- Gitea responde 200 en `/api/healthz` y en la página del repositorio.
- Ver `testigo-$WID.txt` en la interfaz web o hacer `git clone` falla: 500 o `fatal: loose object ... is corrupt`.
- Descargar el adjunto devuelve 200 con bytes incorrectos.

### 4. Por qué no repite ninguna conocida

- **No es la 15.** El repositorio de Restic no tiene ningún daño, y `check --read-data` pasa. La corrupción entró por la fuente y se guardó correctamente.
- **No es la 13.** Ningún trabajo falla y el último snapshot es reciente, dentro del RPO.
- **No es la 3 ni la 6.** No interviene la concurrencia ni el desfase base/repositorios: el daño es previo y persiste en todos los snapshots.
- **No es la 4 ni la 5.** El adjunto está en el backup; lo que está mal es su contenido.

### 5. Qué debería detectar un verificador general

- **Testigos alterados en todos los puntos.** El contenido de los testigos restaurados no coincide con los hashes registrados **en el momento de su creación**, y eso se repite en todos los snapshots retenidos, no en uno solo.
- **Integridad lógica rota.** Los repositorios Git restaurados no superan una verificación completa de objetos, aunque el servicio responda y el almacén de backups se declare íntegro.
- **Integridad del almacén distinta de corrección del contenido.** El sistema debe distinguir «el almacén de backups está íntegro» de «el contenido respaldado es correcto», y reportarlas como dos señales separadas.
- **Sin ningún punto válido de restauración.** Ningún punto retenido satisface el inventario. El snapshot más antiguo disponible es posterior a la primera divergencia observada. El resultado debe ser «no existe ningún punto de restauración válido dentro de la retención», no «falló esta restauración».
- **Retención versus detección.** Conviene señalar que la ventana de retención resultó más corta que el tiempo que tardó en detectarse la corrupción.

> **Supuestos a verificar**
> - Un push con menos objetos que `transfer.unpackLimit` (100 por defecto) deja objetos sueltos en el repositorio. Si Gitea o la versión de Git configuran otro valor, el paso 2 lo detecta con `test -f`.
> - La ruta local de adjuntos es `attachments/<uuid[0]>/<uuid[1]>/<uuid>`.
> - Restic vuelve a leer un archivo cuyo `mtime` cambió aunque el tamaño y el inodo sean los mismos. Si se preservara el `mtime`, Restic podría reutilizar el contenido del snapshot padre y la corrupción no entraría al backup.