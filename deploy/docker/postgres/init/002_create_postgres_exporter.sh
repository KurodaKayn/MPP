#!/usr/bin/env sh
set -eu

exporter_user="${POSTGRES_EXPORTER_USER:-postgres_exporter}"
exporter_password="${POSTGRES_EXPORTER_PASSWORD:-postgres_exporter}"

sql_literal() {
	printf "%s" "$1" | sed "s/'/''/g"
}

exporter_user_sql=$(sql_literal "$exporter_user")
exporter_password_sql=$(sql_literal "$exporter_password")

run_psql() {
	if [ -n "${POSTGRES_HOST:-}" ]; then
		export PGPASSWORD="${POSTGRES_PASSWORD:-}"
		psql \
			-v ON_ERROR_STOP=1 \
			-h "$POSTGRES_HOST" \
			-p "${POSTGRES_PORT:-5432}" \
			--username "$POSTGRES_USER" \
			--dbname "$POSTGRES_DB"
		return
	fi

	psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB"
}

run_psql <<SQL
DO \$\$
DECLARE
	exporter_user text := '$exporter_user_sql';
	exporter_password text := '$exporter_password_sql';
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM pg_catalog.pg_roles
		WHERE rolname = exporter_user
	) THEN
		EXECUTE format('CREATE ROLE %I LOGIN PASSWORD %L', exporter_user, exporter_password);
	ELSE
		EXECUTE format('ALTER ROLE %I WITH LOGIN PASSWORD %L', exporter_user, exporter_password);
	END IF;

	EXECUTE format('GRANT CONNECT ON DATABASE %I TO %I', current_database(), exporter_user);
	EXECUTE format('GRANT pg_monitor TO %I', exporter_user);
END
\$\$;
SQL
