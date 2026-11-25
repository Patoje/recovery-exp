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
- Líneas sin comentarios: 161. Supuestos sobre Gitea (`# S:`): 32.

### B1, medido con el mismo criterio

- Tiempo: 11 min (03:27–03:38 -03 del 2026-11-24, ver arriba). Iteraciones: 1 corrección antes de la primera prueba y 1 prueba.
- Líneas sin comentarios: 384 (`b1.sh` 59, `checks.py` 325). Supuestos sobre Gitea (`# S:`): 20 (`b1.sh` 6, `checks.py` 14).
- Nota: contar por línea favorece al código compacto. El contrato repite supuestos en líneas separadas (por ejemplo, la autenticación básica en cada tipo de testigo) que B1 escribe una sola vez dentro de una función.
