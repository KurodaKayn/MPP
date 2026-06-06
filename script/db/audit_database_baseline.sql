\set ON_ERROR_STOP on
\pset pager off

\if :{?query_limit}
\else
\set query_limit 20
\endif

\if :{?table_limit}
\else
\set table_limit 30
\endif

\if :{?index_limit}
\else
\set index_limit 30
\endif

\echo '0. ensure pg_stat_statements extension'
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

\echo '1. pg_stat_statements settings'
SELECT name, setting
FROM pg_settings
WHERE name IN ('shared_preload_libraries', 'pg_stat_statements.track')
ORDER BY name;

\echo '2. pg_stat_statements extension'
SELECT extname, extversion
FROM pg_extension
WHERE extname = 'pg_stat_statements';

\echo '3. top query fingerprints by total execution time'
SELECT
  calls,
  round(total_exec_time::numeric, 2) AS total_exec_ms,
  round(mean_exec_time::numeric, 2) AS mean_exec_ms,
  rows,
  left(regexp_replace(query, '\s+', ' ', 'g'), 180) AS query
FROM pg_stat_statements
WHERE dbid = (
  SELECT oid
  FROM pg_database
  WHERE datname = current_database()
)
ORDER BY total_exec_time DESC
LIMIT :query_limit;

\echo '4. largest user tables'
SELECT
  schemaname,
  relname,
  n_live_tup,
  n_dead_tup,
  pg_size_pretty(pg_total_relation_size(relid)) AS total_size,
  pg_size_pretty(pg_relation_size(relid)) AS table_size,
  pg_size_pretty(pg_indexes_size(relid)) AS index_size,
  last_vacuum,
  last_autovacuum
FROM pg_stat_user_tables
ORDER BY pg_total_relation_size(relid) DESC
LIMIT :table_limit;

\echo '5. highest dead tuple user tables'
SELECT
  schemaname,
  relname,
  n_live_tup,
  n_dead_tup,
  CASE
    WHEN n_live_tup + n_dead_tup = 0 THEN 0
    ELSE round((n_dead_tup::numeric * 100) / (n_live_tup + n_dead_tup), 2)
  END AS dead_tuple_pct,
  last_vacuum,
  last_autovacuum
FROM pg_stat_user_tables
ORDER BY n_dead_tup DESC, dead_tuple_pct DESC
LIMIT :table_limit;

\echo '6. largest user indexes'
SELECT
  schemaname,
  relname AS table_name,
  indexrelname,
  idx_scan,
  idx_tup_read,
  idx_tup_fetch,
  pg_size_pretty(pg_relation_size(indexrelid)) AS index_size
FROM pg_stat_user_indexes
ORDER BY pg_relation_size(indexrelid) DESC
LIMIT :index_limit;
