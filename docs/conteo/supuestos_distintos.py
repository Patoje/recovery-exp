"""Segundo conteo: supuestos distintos sobre Gitea (criterio en docs/esfuerzo.md).

Asigna cada línea marcada "# S:" a uno o más hechos y cuenta los hechos distintos.
Falla si una línea marcada no está asignada.
Uso: python3 docs/conteo/supuestos_distintos.py
"""
import sys

# Fragmento de la línea marcada -> hechos que contiene.
CONTRACT = {
    "Gitea arranca después de su base de datos": ["ORDEN_ARRANQUE"],
    "/git/commits/{commit_sha}": ["EP_COMMITS"],
    "/issues/{issue_number}\" # S": ["EP_ISSUES"],
    "json: {title:": ["CAMPO_TITLE"],
    "/assets/{attachment_id}": ["EP_ADJUNTOS"],
    "follow_json_field: browser_download_url": ["CAMPO_BROWSER_DOWNLOAD_URL"],
    "/media/commit/": ["EP_MEDIA_LFS"],
    "command: [gitea, --version]": ["CMD_VERSION"],
    "regex: 'version": ["FORMATO_SALIDA_VERSION"],
    "file: /data/gitea/conf/app.ini": ["CFG_RUTA"],
    "format: ini": ["CFG_FORMATO"],
    "section: security": ["CFG_SECCION"],
    "key: SECRET_KEY": ["CFG_CLAVE"],
    "schema: public": ["DB_ESQUEMA"],
    "from repository r join": ["DB_MODELO_REPOS"],
    "root: /data/git/repositories # S": ["DISCO_RAIZ"],
    'glob: "*/*.git" # S': ["DISCO_DISENO"],
    "strip_suffix: .git # S": ["DISCO_NOMBRES"],
    "mismo diseño de repositorios en disco": ["DISCO_RAIZ", "DISCO_DISENO", "DISCO_NOMBRES"],
    'url: "http://gitea:3000/api/v1/version"': ["EP_VERSION"],
    "json: {version:": ["CAMPO_VERSION"],
    "url: \"http://gitea:3000/api/v1/user\"": ["EP_USUARIO"],
    "autenticación básica más el código TOTP": ["AUTH_API_BASICA"],
    "header: X-Gitea-OTP": ["ENCABEZADO_OTP"],
    "json: {login:": ["CAMPO_LOGIN"],
    "URL de Git por HTTP": ["GIT_URL_HTTP"],
    "push por HTTP con autenticación básica": ["GIT_AUTH_BASICA"],
    "la API acepta autenticación básica": ["AUTH_API_BASICA"],
    "la página principal responde sin autenticación": ["EP_INICIO"],
    "/api/v1/repos/search": ["EP_BUSQUEDA_REPOS"],
    'la búsqueda responde "ok": true': ["CAMPO_OK"],
}
B1 = {
    "binario gitea y --version": ["CMD_VERSION"],
    "esquema public": ["DB_ESQUEMA"],
    'tablas repository y "user"': ["DB_MODELO_REPOS"],
    "raíz y diseño de repositorios en disco": ["DISCO_RAIZ", "DISCO_DISENO"],
    "nombre en disco = lower_name + .git": ["DISCO_NOMBRES"],
    "app.ini, [security], SECRET_KEY": ["CFG_RUTA", "CFG_FORMATO", "CFG_SECCION", "CFG_CLAVE"],
    "la API acepta autenticación básica": ["AUTH_API_BASICA"],
    "encabezado del código TOTP": ["ENCABEZADO_OTP"],
    "formato de gitea --version": ["FORMATO_SALIDA_VERSION"],
    "endpoint de versión": ["EP_VERSION"],
    'campo "version"': ["CAMPO_VERSION"],
    "API de commits": ["EP_COMMITS"],
    "API de issues": ["EP_ISSUES"],
    'campo "title"': ["CAMPO_TITLE"],
    "API de adjuntos": ["EP_ADJUNTOS"],
    'campo "browser_download_url"': ["CAMPO_BROWSER_DOWNLOAD_URL"],
    "/media/ sirve el contenido LFS": ["EP_MEDIA_LFS"],
    "endpoint del usuario autenticado": ["EP_USUARIO"],
    'campo "login"': ["CAMPO_LOGIN"],
    "URL de Git por HTTP con autenticación básica": ["GIT_URL_HTTP", "GIT_AUTH_BASICA"],
}


def facts(paths, mapping):
    found, lines = set(), 0
    for path in paths:
        for n, line in enumerate(open(path), 1):
            if "# S:" not in line or line.strip().startswith("#"):
                continue
            lines += 1
            hit = [f for frag, fs in mapping.items() if frag in line for f in fs]
            if not hit:
                sys.exit(f"{path}:{n}: línea # S: sin asignar")
            found.update(hit)
    return lines, found


cl, cf = facts(["contracts/gitea.yaml"], CONTRACT)
bl, bf = facts(["docs/conteo/b1/b1.sh", "docs/conteo/b1/checks.py"], B1)
print(f"contrato: {cl} líneas # S:, {len(cf)} supuestos distintos")
print(f"B1:       {bl} líneas # S:, {len(bf)} supuestos distintos")
print("solo en el contrato:", sorted(cf - bf))
print("solo en B1:", sorted(bf - cf))
