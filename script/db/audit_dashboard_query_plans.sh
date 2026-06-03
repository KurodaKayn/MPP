#!/usr/bin/env sh
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
sql_file="$script_dir/audit_dashboard_query_plans.sql"

if [ -n "${DATABASE_URL:-}" ]; then
	psql "$@" "$DATABASE_URL" -f "$sql_file"
	exit 0
fi

PGPASSWORD="${DB_PASSWORD:-postgres}" psql \
	"$@" \
	-h "${DB_HOST:-127.0.0.1}" \
	-p "${DB_PORT:-5432}" \
	-U "${DB_USER:-postgres}" \
	-d "${DB_NAME:-poster_db}" \
	-f "$sql_file"
