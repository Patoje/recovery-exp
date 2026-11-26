#!/bin/sh
# Usuario dedicado de Nextcloud: dueño de la base, sin CREATEROLE ni superusuario. Corre en el primer
# arranque de PostgreSQL, igual en el entorno original y en el de restauración, para que el dump
# (objetos con dueño $NC_DB_USER) se cargue con el mismo rol.
set -e
psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" \
  -v u="$NC_DB_USER" -v p="$NC_DB_PASSWORD" -v db="$POSTGRES_DB" <<'SQL'
CREATE ROLE :"u" LOGIN PASSWORD :'p' NOSUPERUSER NOCREATEDB NOCREATEROLE;
ALTER DATABASE :"db" OWNER TO :"u";
SQL
