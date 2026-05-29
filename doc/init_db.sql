-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  username text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE projects (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id uuid NOT NULL REFERENCES users(id),
  title text NOT NULL,
  source_content text NOT NULL,
  status text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE project_platform_publications (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES projects(id),
  platform text NOT NULL,
  enabled boolean NOT NULL DEFAULT true,
  status text NOT NULL,
  config jsonb NOT NULL DEFAULT '{}'::jsonb,
  adapted_content jsonb NOT NULL DEFAULT '{}'::jsonb,
  remote_id text,
  publish_url text,
  error_message text,
  retry_count integer NOT NULL DEFAULT 0,
  last_attempt_at timestamptz,
  published_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, platform)
);

-- Recommended Indexes
CREATE INDEX idx_publications_project_platform ON project_platform_publications (project_id, platform);
CREATE INDEX idx_publications_platform_status ON project_platform_publications (platform, status);
CREATE INDEX idx_publications_project_status ON project_platform_publications (project_id, status);
