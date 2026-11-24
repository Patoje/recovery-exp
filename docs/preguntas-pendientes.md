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
