# Dashboard API Documentation

This document defines the core read-only APIs used by the platform dashboard. All endpoints follow RESTful conventions and return a standardized error data structure upon failure.

## Global Conventions

### Base Path
`/api`

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
    "code": "invalid_request",  // Error code (e.g., invalid_request, not_found, internal_error)
    "message": "Detailed error description" // Specific error message intended for developers
  }
}
```

---

## 1. Get Dashboard Overview Statistics (Dashboard Stats)

Retrieves macro-level system metrics used for the primary data display at the top of the dashboard.

- **URL**: `/dashboard/stats`
- **Method**: `GET`
- **Auth Required**: Admin (Not strictly enforced during the current development phase)

### Request Parameters
None.

### Response `200 OK`
```json
{
  "total_users": 2,                           // Total number of registered users on the platform
  "total_projects": 19,                       // Total number of projects (articles) created in the system
  "total_published_publications": 2,          // Total number of successfully published records across all platforms
  "total_failed_publications": 1              // Total number of failed publication records across all platforms
}
```

---

## 2. Get Project List (List Projects)

Retrieves a paginated list of projects (articles). This endpoint is optimized for list rendering; **it explicitly excludes extremely large text fields like `source_content`**, and returns a nested, summarized distribution status for each platform.

- **URL**: `/projects`
- **Method**: `GET`
- **Auth Required**: Admin (or the user specified in the current context)

### Query Parameters
| Parameter | Type | Required | Default | Description |
| :--- | :--- | :--- | :--- | :--- |
| `page` | integer | No | 1 | Current page number, minimum is 1 |
| `limit` | integer | No | 10 | Number of items to display per page, maximum limit is 100 |
| `status` | string | No | - | Filter by project status (`draft`, `ready`, `publishing`, `published`, `failed`) |
| `user_id`| string(uuid)| No | - | (Admin only) Filter projects belonging to a specific user |
| `platform`| string | No | - | Filter projects that contain distribution records for a specific platform (e.g., `wechat`, `zhihu`) |

### Response `200 OK`
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
        },
        {
          "id": "231d85d2-9dec-49a9-9e85-af05e71a3391",
          "platform": "zhihu",
          "enabled": true,
          "status": "published",
          "publish_url": "https://www.zhihu.com/question/12345/answer/67890"
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

---

## 3. Get Project Platform Publication Details (Project Publication Details)

Used on the project detail page to view the specific configurations, adapted content summaries, and distribution statuses for this content across various social media platforms.

> ⚠️ **Security Notice**: To prevent the leakage of sensitive information, this endpoint implements strict **whitelist filtering** on the `config` field (hiding credentials such as Tokens and Cookies). To ensure network transmission efficiency, `adapted_content` only returns the format type and a text summary, filtering out the massive `full_text` string used for rendering.

- **URL**: `/projects/:id/publications`
- **Method**: `GET`
- **Auth Required**: Admin (or the owning user)

### Path Parameters
| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `id` | string(uuid) | Yes | The UUID of the Project |

### Response `200 OK`
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
        "summary": "A comprehensive 10,000-word analysis of 2026 AI large model development trends, helping you understand the future of multi-modality and Agent ecosystems. Click 'Read More' for the full report.",
        "format": "html"
        // Note: The massive body text for rendering is filtered out to protect frontend performance
      },
      "publish_url": "https://mp.weixin.qq.com/s/abcdefg123456",
      "remote_id": "wx_article_998877",
      "retry_count": 0,
      "last_attempt_at": null,
      "published_at": null,
      "created_at": "2026-05-26T11:23:50.482Z",
      "updated_at": "2026-05-26T11:23:50.482Z"
    }
  ]
}
```

### Error Response Examples

- **400 Bad Request** (`invalid_request`): The provided `id` is not a valid UUID format.
- **404 Not Found** (`not_found`): The specified project ID does not exist in the database.
