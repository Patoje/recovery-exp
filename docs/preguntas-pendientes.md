# Preguntas pendientes

Decisiones que no están cubiertas por lo ya aprobado. Ninguna se tomó; el trabajo siguió con lo que no depende de ellas.

## 1. Falla 08: Gitea 28.4.0 regenera los hooks al arrancar

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

**Qué pasó.** Al probar B1 en `00-healthy` (`results/faults/00-healthy/20261124T063110Z-r01`), la huella de `SECRET_KEY` declarada y la que está en uso son iguales (`e3b0c44298fc…`), pero esa es la sha256 de un texto vacío. En `app.ini`, `[security] SECRET_KEY =` está vacía. El entorno de la etapa 1 arranca Gitea con `INSTALL_LOCK=true` y sin `SECRET_KEY`, y Gitea 28 no la genera (sí genera `INTERNAL_TOKEN` y los JWT). Con la clave vacía, Gitea usa su clave interna por defecto, la misma en todas las instalaciones. El check de B1 cumple la especificación (compara la huella declarada con la que está en uso) y detecta un cambio como el de la falla 07, que sigue siendo válida. Pero en una corrida sana compara vacío contra vacío.

**Opciones:**

- **A.** Dejarlo así y documentar que la clave declarada está vacía y Gitea usa la clave interna por defecto.
- **B.** Fijar una `SECRET_KEY` de prueba en `.env.example` y en `compose/original.yml` y `compose/restore.yml`. Cambia el entorno congelado con B0 (`b0-v1`) y obliga a repetir la parte C.
- **C.** Que B1 además falle si la clave declarada está vacía. Cambia el criterio de B1 y daría fail en todas las corridas actuales.
