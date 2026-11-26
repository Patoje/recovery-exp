#!/usr/bin/env bash
# Valores declarados de la corrida de Nextcloud, tomados del entorno original después del backup:
# imagen y versión de Nextcloud, huella (sha256) de 'secret' de config.php, codificación de la base.
# Salida: <run>/declared.json. No forma parte de los procedimientos de referencia ni de B0.
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

RUN_DIR="${1:-$(current_run_dir)}"

cid="$("${ORIG[@]}" ps -q nextcloud)"
image_ref="$(docker inspect --format '{{.Config.Image}}' "$cid")"
status="$("${ORIG[@]}" --profile tools run --rm --no-deps -T runner curl -fsS http://nextcloud/status.php)"
version="$(printf '%s' "$status" | grep -o '"version":"[^"]*"' | cut -d'"' -f4)"
versionstring="$(printf '%s' "$status" | grep -o '"versionstring":"[^"]*"' | cut -d'"' -f4)"
secret_fp="$("${ORIG[@]}" exec -T -u www-data nextcloud php -r \
  'include "/var/www/html/config/config.php"; echo hash("sha256", $CONFIG["secret"] ?? "");')"
encoding="$("${ORIG[@]}" exec -T postgres sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Atc "show server_encoding"')"

[ -n "$version" ] && [ -n "$secret_fp" ] && [ -n "$encoding" ] || { echo "declare: faltan valores" >&2; exit 1; }

cat > "$RUN_DIR/declared.json" <<EOF
{
  "taken_at": $(json_str "$(iso_ms "$(now_ms)")"),
  "nextcloud_image": $(json_str "$image_ref"),
  "nextcloud_version": $(json_str "$version"),
  "nextcloud_versionstring": $(json_str "$versionstring"),
  "secret_sha256": $(json_str "$secret_fp"),
  "database_encoding": $(json_str "$encoding"),
  "external_dependencies": []
}
EOF
log "declared.json: nextcloud $version, encoding $encoding"
