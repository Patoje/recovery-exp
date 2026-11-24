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
