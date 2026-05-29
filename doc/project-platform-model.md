# Project and Platform Publication Data Model Guide

## 1. Background

This system is designed for multi-platform content publishing. A user can manage multiple projects, and each project represents a piece of content that is either being prepared for publication or has already been published. A single project can usually be distributed to multiple platforms, such as WeChat Official Accounts, Zhihu, Bilibili, and Xiaohongshu.

Different platforms have different publication parameters, content formats, media requirements, and extension fields. These platform capabilities can also change over time. For that reason, platform-specific data should not be modeled entirely as fixed relational columns.

A more maintainable approach is:

- Store only stable, shared information in the project table.
- Model each platform publication target and its state separately.
- Store frequently changing platform-specific parameters in PostgreSQL `jsonb` fields.

## 2. Core Concepts

### 2.1 User

A user is a content creator or operator in the system. One user can create and manage multiple projects.

The user entity should only store stable and general account information, such as:

- User ID
- Username
- Authentication-related information
- Created time
- Updated time

### 2.2 Project

A project is the business container for content creation and publication. In practical terms, a project is a source content draft.

A project should belong to one user and store stable project-level information, such as:

- Project ID
- User ID
- Project title
- Source content
- Project status
- Created time
- Updated time

### 2.3 Platform Publication

A platform publication represents one publication target or publication instance for a project on a specific platform.

One project can have multiple platform publication records, for example:

- The same content is published to WeChat, Zhihu, and Xiaohongshu.
- The same platform publication may require retries or later edits.

A platform publication should store:

- Platform key
- Whether the platform target is enabled
- Platform-specific configuration
- Platform-adapted content
- Publication status
- Remote article ID or publication URL returned by the third-party platform
- Error and retry information

## 3. Data Modeling Principles

### 3.1 Use Relational Columns for Stable Fields

Fields that are stable and frequently used for querying, filtering, sorting, or indexing should use regular database columns.

Examples:

- `id`
- `user_id`
- `title`
- `status`
- `platform`
- `created_at`
- `updated_at`

### 3.2 Use JSONB for Frequently Changing Fields

Platform-specific fields, publication configuration, adapted content, and draft metadata that change frequently should be stored with PostgreSQL `jsonb`.

JSONB should only carry the platform-specific differences. Avoid storing all platform publication records inside one large JSON object.

Example fields:

```sql
config jsonb
adapted_content jsonb
```

## 4. Recommended Table Structure

The following structure is a development reference. The exact fields can be adjusted based on the backend framework and migration tooling used by the project.

```sql
CREATE TABLE users (
  id uuid PRIMARY KEY,
  username text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE projects (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id),
  title text NOT NULL,
  source_content text NOT NULL,
  status text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE project_platform_publications (
  id uuid PRIMARY KEY,
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
```

## 5. JSON Field Guidelines

### 5.1 Store Only Platform-Specific Fields in JSON

`config` and `adapted_content` should only store dynamic platform-specific fields, such as:

- Summary
- Tags
- Cover image
- Topics
- Category
- Original content declaration
- Platform-adapted formatting result

### 5.2 Use Stable Platform Keys

The `platform` field should use a stable platform key instead of a display name.

Recommended:

- `wechat`
- `zhihu`
- `bilibili`
- `xiaohongshu`

Not recommended:

- `WeChat Official Account`
- `Zhihu Platform`
- `Bilibili Site`
- `Xiaohongshu Note`

### 5.3 Validate Platform Configuration in the Backend

Although platform configuration is stored as JSONB, the business layer should still validate it to prevent unconstrained data from polluting the database.

Recommended validation rules include:

- Whether the platform key is valid
- Whether required fields are present
- Whether field types are correct
- Whether character limits, tag counts, and media counts comply with platform rules
- Whether the platform schema version matches the current backend rules

## 6. Query and Indexing Recommendations

Prefer indexing platform publication records instead of repeatedly querying a large JSON object.

Examples:

```sql
CREATE INDEX idx_publications_project_platform
ON project_platform_publications (project_id, platform);

CREATE INDEX idx_publications_platform_status
ON project_platform_publications (platform, status);

CREATE INDEX idx_publications_project_status
ON project_platform_publications (project_id, status);
```

If a specific field inside `config` must be queried frequently, add a JSONB expression index or a GIN index for that field.

If platform capabilities continue to grow, the model can be further extended with:

- `platforms`: platform metadata and version information
- `platform_accounts`: platform accounts and authorization information
- `project_platform_publications`: publication records and execution state

