\set ON_ERROR_STOP on
\pset pager off

-- Override these with psql -v user_id=... -v project_id=... -v platform=...
\if :{?user_id}
\else
\set user_id '00000000-0000-0000-0000-000000000001'
\endif

\if :{?project_id}
\else
\set project_id '00000000-0000-0000-0000-000000000101'
\endif

\if :{?platform}
\else
\set platform 'douyin'
\endif

\if :{?page_limit}
\else
\set page_limit 12
\endif

\if :{?page_offset}
\else
\set page_offset 0
\endif

\echo '1. dashboard scoped project count'
EXPLAIN (ANALYZE, BUFFERS)
SELECT count(*)
FROM projects
WHERE user_id = :'user_id'::uuid;

\echo '2. dashboard scoped publication status count'
EXPLAIN (ANALYZE, BUFFERS)
SELECT count(*)
FROM project_platform_publications
JOIN projects ON projects.id = project_platform_publications.project_id
WHERE project_platform_publications.status = 'published'
  AND projects.user_id = :'user_id'::uuid;

\echo '3. dashboard scoped failed publication count'
EXPLAIN (ANALYZE, BUFFERS)
SELECT count(*)
FROM project_platform_publications
JOIN projects ON projects.id = project_platform_publications.project_id
WHERE project_platform_publications.status = 'failed'
  AND projects.user_id = :'user_id'::uuid;

\echo '4. project list page'
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, user_id, title, status, created_at, updated_at
FROM projects
WHERE user_id = :'user_id'::uuid
ORDER BY created_at DESC
LIMIT :page_limit OFFSET :page_offset;

\echo '5. project list with platform filter'
EXPLAIN (ANALYZE, BUFFERS)
SELECT projects.id, projects.user_id, projects.title, projects.status, projects.created_at, projects.updated_at
FROM projects
JOIN project_platform_publications ppp ON ppp.project_id = projects.id
WHERE projects.user_id = :'user_id'::uuid
  AND ppp.platform = :'platform'
GROUP BY projects.id
ORDER BY projects.created_at DESC
LIMIT :page_limit OFFSET :page_offset;

\echo '6. publication preload for project list'
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, project_id, platform, enabled, status, publish_url
FROM project_platform_publications
WHERE project_id = ANY(ARRAY[:'project_id'::uuid]);

\echo '7. platform account lookup'
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, user_id, platform, username, status, avatar_url, last_tested_at, last_test_error, created_at, updated_at
FROM platform_accounts
WHERE user_id = :'user_id'::uuid
  AND platform = :'platform'
LIMIT 1;

\echo '8. active browser sessions lookup'
EXPLAIN (ANALYZE, BUFFERS)
SELECT id, user_id, platform, status, worker_session_ref, created_at, expires_at
FROM remote_browser_sessions
WHERE user_id = :'user_id'::uuid
  AND platform = :'platform'
  AND status IN ('pending', 'ready', 'login_detected', 'capturing');
