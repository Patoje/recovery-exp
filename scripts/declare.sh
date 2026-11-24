#!/usr/bin/env bash
# Valores declarados de la corrida, tomados del entorno original después del backup:
# imagen y versión de Gitea, huella (sha256) de SECRET_KEY, codificación de la base
# y dependencias externas de almacenamiento. Salida: <run>/declared.json.
# No forma parte de los procedimientos de referencia ni de B0.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

RUN_DIR="${1:-$(current_run_dir)}"

cid="$("${ORIG[@]}" ps -q gitea)"
image_ref="$(docker inspect --format '{{.Config.Image}}' "$cid")"
version="$("${ORIG[@]}" --profile tools run --rm --no-deps -T runner curl -fsS http://gitea:3000/api/v1/version \
  | grep -o '"version":"[^"]*"' | cut -d'"' -f4)"
app_ini="$("${ORIG[@]}" exec -T gitea cat /data/gitea/conf/app.ini)"
secret_fp="$(printf '%s' "$app_ini" | awk -F' *= *' '/^\[/{s=$0} s=="[security]" && $1=="SECRET_KEY"{printf "%s", $2}' \
  | "${ORIG[@]}" exec -T gitea sha256sum | cut -d' ' -f1)"
encoding="$("${ORIG[@]}" exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "show server_encoding"')"

deps=""
for section in attachment lfs; do
  st="$(printf '%s' "$app_ini" | awk -F' *= *' -v sec="[$section]" '/^\[/{s=$0} s==sec && $1=="STORAGE_TYPE"{print $2}')"
  if [ "$st" = "minio" ]; then
    ep="$(printf '%s' "$app_ini" | awk -F' *= *' -v sec="[$section]" '/^\[/{s=$0} s==sec && $1=="MINIO_ENDPOINT"{print $2}')"
    obj="{\"section\":$(json_str "$section"),\"type\":\"minio\",\"endpoint\":$(json_str "$ep")}"
    deps="${deps:+$deps,}$obj"
  fi
done

[ -n "$version" ] && [ -n "$secret_fp" ] && [ -n "$encoding" ] || { echo "declare: faltan valores" >&2; exit 1; }

cat > "$RUN_DIR/declared.json" <<EOF
{
  "taken_at": $(json_str "$(iso_ms "$(now_ms)")"),
  "gitea_image": $(json_str "$image_ref"),
  "gitea_version": $(json_str "$version"),
  "secret_key_sha256": $(json_str "$secret_fp"),
  "database_encoding": $(json_str "$encoding"),
  "external_dependencies": [$deps]
}
EOF
log "declared.json: gitea $version, encoding $encoding"
