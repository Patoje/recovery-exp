"""Cuenta líneas sin comentarios y supuestos "# S:" con el mismo criterio para todos los archivos.

Una línea cuenta como código si no está vacía y, sin espacios iniciales, no empieza con
"#" (YAML, bash, Python) ni con "//" (Go). Las docstrings de Python cuentan como código.
Un supuesto es una línea que contiene "# S:" (uno por línea, aunque la línea tenga varios).
Uso: python3 docs/conteo/contar.py ARCHIVO...
"""
import sys

total_code = total_s = 0
for path in sys.argv[1:]:
    code = s = 0
    with open(path) as f:
        for line in f:
            t = line.strip()
            if t and not t.startswith("#") and not t.startswith("//"):
                code += 1
            if "# S:" in line:
                s += 1
    total_code, total_s = total_code + code, total_s + s
    print(f"{code:6d} {s:4d}  {path}")
print(f"{total_code:6d} {total_s:4d}  total (líneas sin comentarios, supuestos # S:)")
