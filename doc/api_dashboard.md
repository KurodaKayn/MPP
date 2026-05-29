# Dashboard API Documentation

This document defines the core read-only APIs used by the platform dashboard. The APIs are segregated into Admin and User (Personal Center) scopes. All endpoints follow RESTful conventions and return a standardized error data structure upon failure.

## Global Conventions

### Base Paths
- Admin Scope: `/api/admin/dashboard`
- User Scope: `/api/user/dashboard`

### Standard Pagination Response
Endpoints that return paginated lists will use the following nested structure:
```json
{
  "items": [],        // List of data objects
  "page": 1,          // Current page number
  "limit": 10,        // Number of items per page
  "total": 100,       // Total number of records
  "total_pages": 10   // Total number of pages
}
```

### Standard Error Response
Upon failure (HTTP status codes 4xx or 5xx), the following structure will be returned:
```json
{
  "error": {
    "code": "invalid_request",  // Error code (e.g., invalid_request, unauthorized, forbidden, not_found, internal_error)
    "message": "Detailed error description" // Specific error message intended for developers
  }
}
```

---

## Authentication & Authorization

### 1. Mock Login (Development Only)
Generates a JWT token for local testing without a full authentication flow.

- **URL**: `/api/auth/mock-login`
- **Method**: `POST`
- **Auth Required**: None

**Request Body** (`application/json`):
```json
{
  "username": "kuroda_kayn"
}
```

**Response `200 OK`**:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

### 2. User Scope Authentication
All endpoints under `/api/user/dashboard` require a valid JWT token passed in the Authorization header.
- **Header**: `Authorization: Bearer <token>`
- **Behavior**: The server extracts the `user_id` securely from the JWT. Attempting to access another user's project will result in a `403 Forbidden` error.

---

## Dashboard APIs (Admin & User)

The following APIs share the same response structures but apply different data scope boundaries depending on the base path.

### 1. Get Dashboard Overview Statistics
Retrieves macro-level system metrics used for the primary data display.

- **Admin URL**: `GET /api/admin/dashboard/stats` (Returns global totals across all users)
- **User URL**: `GET /api/user/dashboard/stats` (Returns totals belonging only to the authenticated user)

**Response `200 OK`**:
```json
{
  "total_users": 2,                           // Total users (Always 1 for User URL)
  "total_projects": 19,                       // Total number of projects (articles)
  "total_published_publications": 2,          // Total successfully published records
  "total_failed_publications": 1              // Total failed publication records
}
```

### 2. Get Project List
Retrieves a paginated list of projects (articles). Optimized for list rendering; explicitly excludes extremely large text fields like `source_content`, returning a summarized distribution status for each platform.

- **Admin URL**: `GET /api/admin/dashboard/projects`
- **User URL**: `GET /api/user/dashboard/projects` (Strictly scoped to the authenticated user)

**Query Parameters**:
| Parameter | Type | Required | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `page` | integer | No | 1 | Current page number, minimum is 1 |
| `limit` | integer | No | 10 | Number of items to display per page, maximum is 100 |
| `status` | string | No | - | Filter by project status (`draft`, `ready`, `publishing`, `published`, `failed`) |
| `user_id`| string(uuid)| No | - | **(Admin URL only)** Filter projects belonging to a specific user |
| `platform`| string | No | - | Filter projects containing distribution records for a specific platform (e.g., `wechat`, `zhihu`) |

**Response `200 OK`**:
```json
{
  "items": [
    {
      "id": "f31d7ae2-0cea-4c7a-a0e9-f760e568f4d5",
      "user_id": "c5f8cb20-b58e-4f43-a6b0-5c3189fbda0f",
      "title": "2026 AI Industry Large Model Trend Deep Dive",
      "status": "published",
      "created_at": "2026-05-26T11:23:50.482Z",
      "updated_at": "2026-05-26T11:23:50.482Z",
      "publications": [
        {
          "id": "74b801d8-587c-4658-aec1-88a19cf63797",
          "platform": "wechat",
          "enabled": true,
          "status": "published",
          "publish_url": "https://mp.weixin.qq.com/s/abcdefg123456"
        }
      ]
    }
  ],
  "page": 1,
  "limit": 10,
  "total": 1,
  "total_pages": 1
}
```

### 3. Get Project Platform Publication Details
Used on the project detail page to view specific configurations, adapted content summaries, and distribution statuses for this content across various social media platforms.

> ⚠️ **Security Notice**: To prevent the leakage of sensitive information, this endpoint implements strict **whitelist filtering** on the `config` field (hiding credentials such as Tokens and Cookies). `adapted_content` only returns the format type and a text summary, filtering out the massive `full_text` string used for rendering.

- **Admin URL**: `GET /api/admin/dashboard/projects/:id/publications`
- **User URL**: `GET /api/user/dashboard/projects/:id/publications` (Verifies ownership of `:id`)

**Path Parameters**:
| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `id` | string(uuid) | Yes | The UUID of the Project |

**Response `200 OK`**:
```json
{
  "project_id": "f31d7ae2-0cea-4c7a-a0e9-f760e568f4d5",
  "items": [
    {
      "id": "74b801d8-587c-4658-aec1-88a19cf63797",
      "platform": "wechat",
      "enabled": true,
      "status": "published",
      "error_message": "",
      "config": {
        "title": "2026 AI Industry Large Model Trends",
        "tags": ["AI", "Large Models", "Frontier Tech"],
        "original_declaration": true
        // Note: Sensitive configurations like author_token stored in the database are automatically sanitized
      },
      "adapted_content": {
        "summary": "A comprehensive analysis of 2026 AI large model trends.",
        "format": "html"
        // Note: The massive body text for rendering is filtered out
      },
      "publish_url": "https://mp.weixin.qq.com/s/abcdefg123456",
      "remote_id": "wx_article_998877",
      "retry_count": 0,
      "last_attempt_at": null,
      "published_at": "2026-05-26T11:23:50.482Z",
      "created_at": "2026-05-26T11:23:50.482Z",
      "updated_at": "2026-05-26T11:23:50.482Z"
    }
  ]
}
```

**Error Response Examples**:
- **401 Unauthorized**: Missing or invalid JWT token on `/api/user/*` routes.
- **403 Forbidden**: Authenticated user attempts to view a project they do not own.
- **404 Not Found**: The specified project ID does not exist.
