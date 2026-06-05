# R2 Object Storage and Signed URL Plan

## 状态

已确认方案 B：Backend 统一管理 R2 对象存储和媒体资产元数据，Frontend 使用 backend 生成的 signed URL 直传 R2。

第一阶段范围只支持：

- 项目编辑器图片上传。
- 发布链路从对象存储读取图片。

第一阶段暂不支持：

- Extension handoff 资产下载改造。
- 视频上传。
- multipart upload。
- CDN 和自定义域名。
- 历史 `data:image` 内容批量迁移。

## 背景

高并发与分布式架构计划第 15 条要求引入对象存储与签名 URL，避免图片和媒体继续压在应用容器与数据库上。

当前项目状态：

- `Project.SourceContent` 以 HTML 字符串保存正文内容。
- `ProjectPlatformPublication.Config` 和 `AdaptedContent` 以 JSON 保存平台发布配置和适配内容。
- 前端 TipTap 编辑器当前会把本地图片转为 `data:image` 并直接插入 HTML。
- backend 发布链路当前可以通过 `media.DownloadAndProcess` 下载 URL 或解析 data URL。
- content pipeline protobuf 已具备 `object_ref` 形态，但 backend 当前媒体处理入口仍主要按 URL/data URL 使用。
- extension handoff 已有 `assets[].source_url` 字段，但第一阶段不改 extension 资产交付。

## 目标

把新增图片从内联 data URL 改为 R2 对象。

保存到数据库的内容应保存稳定媒体引用，而不是保存短期 signed URL。

Backend 负责：

- 校验用户是否有项目编辑权限。
- 创建 media asset 元数据。
- 生成 R2 PUT signed URL。
- 校验上传完成状态。
- 为前端预览生成短期 GET signed URL。
- 为发布链路读取对象内容。

Frontend 负责：

- 图片选择后请求上传 URL。
- 使用 signed URL 直传 R2。
- 将图片以稳定 asset ref 插入编辑器内容。
- 渲染编辑器时把 asset ref hydrate 成短期预览 URL。

## R2 约束与参考

Cloudflare R2 提供 S3-compatible API，presigned URL 支持临时授权 `GET`、`PUT`、`HEAD`、`DELETE` 等对象操作。官方文档说明 presigned URL 有效期范围为 1 秒到 7 天。

R2 S3 API endpoint 形如：

```text
https://<ACCOUNT_ID>.r2.cloudflarestorage.com
```

region 使用：

```text
auto
```

参考：

- https://developers.cloudflare.com/r2/api/s3/presigned-urls/
- https://developers.cloudflare.com/r2/api/s3/api/

## 架构选择

### 选定方案

使用私有 R2 bucket。

用户上传图片时：

1. Frontend 请求 backend 创建上传任务。
2. Backend 创建 `media_assets` 记录，生成 object key。
3. Backend 生成短期 PUT signed URL。
4. Frontend 直传图片到 R2。
5. Frontend 通知 backend 上传完成。
6. Backend 通过 HEAD 校验对象存在、大小和类型。
7. Editor 插入稳定 asset ref。

用户打开项目时：

1. Backend 返回项目内容，内容中保留稳定 asset ref。
2. Frontend 批量解析 asset ref。
3. Backend 返回短期 GET signed URL。
4. Frontend 用 signed URL 渲染图片。

发布时：

1. 发布服务解析项目内容或平台 config 中的 asset ref。
2. Backend/publish-worker 使用 R2 client 读取对象 bytes，或生成内部短期 GET signed URL 后复用现有下载处理逻辑。
3. 现有平台发布器继续接收处理后的图片 bytes 或临时本地文件。

### 不选方案

不把 signed URL 永久保存进 `SourceContent`。

原因：

- signed URL 会过期。
- DB 中会保存临时凭证参数。
- 后续编辑、预览和发布都会变得不稳定。

不让 frontend 直接持有 R2 access key。

原因：

- R2 secret 只能存在 backend 环境变量中。
- 所有上传权限必须由 backend 根据用户和项目权限临时签发。

## 数据模型

新增模型：`MediaAsset`

建议字段：

```go
type MediaAsset struct {
    ID               uuid.UUID
    UserID           uuid.UUID
    WorkspaceID      *uuid.UUID
    ProjectID        *uuid.UUID
    Bucket           string
    ObjectKey        string
    OriginalFilename string
    MimeType         string
    SizeBytes        int64
    Usage            string
    Status           string
    ETag             string
    ErrorMessage     string
    CreatedAt        time.Time
    UpdatedAt        time.Time
    DeletedAt        gorm.DeletedAt
}
```

状态建议：

```text
pending
ready
failed
deleted
```

usage 第一阶段只需要：

```text
editor_image
cover_image
```

后续可以扩展：

```text
extension_asset
video
thumbnail
```

## Object Key 规范

建议 key：

```text
workspaces/{workspace_id}/projects/{project_id}/assets/{asset_id}/{safe_filename}
```

要求：

- `asset_id` 使用 UUID。
- `safe_filename` 只用于可读性，不作为权限边界。
- 不使用用户原始文件名直接作为 key。
- 不允许客户端传入完整 object key。

## Backend 模块设计

### Object Storage 抽象

新增包建议：

```text
backend/internal/pkg/objectstorage
```

接口建议：

```go
type Client interface {
    PresignPutObject(ctx context.Context, input PutObjectInput) (PresignedURL, error)
    PresignGetObject(ctx context.Context, input GetObjectInput) (PresignedURL, error)
    HeadObject(ctx context.Context, key string) (ObjectInfo, error)
    GetObject(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error)
    DeleteObject(ctx context.Context, key string) error
}
```

实现：

```text
backend/internal/pkg/objectstorage/r2
```

使用 AWS SDK for Go v2 的 S3-compatible client。

### Media Service

新增服务建议：

```text
backend/internal/services/media_asset
```

职责：

- 校验项目访问权限。
- 校验 MIME type。
- 校验 size。
- 创建 pending asset。
- 生成 PUT signed URL。
- 上传完成后 HEAD 校验对象。
- 生成 GET signed URL。
- 为发布链路读取对象。
- 软删除对象记录，必要时异步删除 R2 object。

### Dashboard API

建议路由：

```text
POST /api/user/dashboard/projects/:id/media/uploads
POST /api/user/dashboard/media/:id/complete
POST /api/user/dashboard/media/resolve
DELETE /api/user/dashboard/media/:id
```

创建上传 URL 请求：

```json
{
  "filename": "cover.png",
  "mime_type": "image/png",
  "size_bytes": 123456,
  "usage": "editor_image"
}
```

创建上传 URL 响应：

```json
{
  "asset_id": "asset-uuid",
  "object_ref": "mpp://media/asset-uuid",
  "upload_url": "https://<account>.r2.cloudflarestorage.com/...",
  "headers": {
    "Content-Type": "image/png"
  },
  "expires_at": "2026-06-05T10:10:00Z"
}
```

完成上传响应：

```json
{
  "asset_id": "asset-uuid",
  "status": "ready"
}
```

批量解析请求：

```json
{
  "asset_ids": ["asset-uuid"]
}
```

批量解析响应：

```json
{
  "items": [
    {
      "asset_id": "asset-uuid",
      "url": "https://<account>.r2.cloudflarestorage.com/...",
      "expires_at": "2026-06-05T10:05:00Z"
    }
  ]
}
```

## Frontend 编辑器设计

当前编辑器在 `use-content-tiptap-editor.ts` 中通过 `FileReader.readAsDataURL` 插入图片。

第一阶段改为：

1. 选择图片。
2. 检查文件类型和大小。
3. 调用 upload URL API。
4. 使用 `fetch(upload_url, { method: "PUT", body: file, headers })` 上传到 R2。
5. 调用 complete API。
6. 在 editor 中插入图片节点。

编辑器内部渲染时可以使用短期 signed GET URL。

保存到 backend 时必须持久化稳定引用：

```html
<img src="mpp://media/asset-uuid" data-mpp-media-id="asset-uuid" alt="cover.png">
```

打开项目时：

1. 从 HTML 中收集 `data-mpp-media-id` 或 `mpp://media/{id}`。
2. 调用 resolve API。
3. 将 editor runtime 中的 `src` 替换为 signed GET URL。
4. 保存前再序列化回稳定 `mpp://media/{id}`。

## 发布链路设计

第一阶段只要求 backend 发布链路能读取 R2 图片。

落点：

- `backend/internal/pkg/media`
- WeChat 发布器。
- Douyin 发布器。
- dashboard publish delegates。

处理策略：

1. 当图片来源是 `mpp://media/{asset_id}` 时，调用 media asset service 校验权限和对象状态。
2. 使用 R2 `GetObject` 读取 bytes。
3. 复用现有压缩和平台上传逻辑。

如果为了最小改动，也可以先将 `mpp://media/{asset_id}` 解析为短期 GET signed URL，再复用现有 `DownloadAndProcess`。但 publish-worker 已经运行在后端环境中，长期更推荐直接通过 object storage client 读取对象。

## 配置项

建议环境变量：

```text
OBJECT_STORAGE_PROVIDER=r2
R2_ACCOUNT_ID=
R2_ACCESS_KEY_ID=
R2_SECRET_ACCESS_KEY=
R2_BUCKET=
R2_ENDPOINT=
R2_REGION=auto
MEDIA_UPLOAD_MAX_BYTES=5242880
MEDIA_UPLOAD_URL_TTL=10m
MEDIA_DOWNLOAD_URL_TTL=5m
MEDIA_ALLOWED_MIME_TYPES=image/jpeg,image/png,image/webp,gif
```

说明：

- `R2_ENDPOINT` 可由 `R2_ACCOUNT_ID` 推导，也允许显式覆盖，方便测试。
- R2 secret 只进入 backend/publish-worker。
- Frontend 不读取 R2 secret。
- 本地开发可以关闭对象存储，继续兼容旧 data URL。

## R2 Bucket CORS

如果浏览器直接 PUT 到 R2，需要配置 bucket CORS。

第一阶段至少允许：

```text
Origins:
- 本地 frontend origin
- 生产 frontend origin

Methods:
- PUT
- GET
- HEAD

Headers:
- Content-Type
```

实际允许的 Origin 不应使用宽泛通配符。

## 安全策略

- Bucket 默认私有。
- 所有上传 URL 都必须绑定当前用户可编辑的 project。
- 上传 URL TTL 建议 10 分钟。
- 下载 URL TTL 建议 5 分钟。
- 上传前校验声明的 MIME type 和 size。
- 上传完成后使用 HEAD 校验实际对象。
- object key 由 backend 生成。
- 不允许客户端指定 bucket 或 object key。
- 删除项目或图片时软删除 asset 记录，R2 删除可以同步或异步执行。
- 日志中不要输出 signed URL query string。

## 开发阶段

### 阶段 1：Backend R2 基础设施

交付：

- Object storage interface。
- R2 S3-compatible client。
- Presign PUT/GET。
- Head/Get/Delete object。
- 配置读取和校验。
- Fake client 单元测试。

建议分支：

```text
feature/backend-r2-object-storage
```

### 阶段 2：Media Asset 数据模型与 API

交付：

- `MediaAsset` model。
- DB migration/AutoMigrate。
- Create upload URL API。
- Complete upload API。
- Resolve download URL API。
- Delete API。
- 权限校验和单元测试。

建议分支：

```text
feature/backend-media-asset-api
```

### 阶段 3：Frontend 编辑器上传接入

交付：

- 替换 `FileReader.readAsDataURL` 图片插入流程。
- 直传 R2。
- 编辑器 runtime signed URL hydrate。
- 保存时序列化为稳定 asset ref。
- 前端测试覆盖上传成功、失败、过期 URL、保存序列化。

建议分支：

```text
feature/frontend-r2-editor-upload
```

### 阶段 4：发布链路读取 R2 图片

交付：

- `mpp://media/{asset_id}` 解析器。
- backend/publish-worker 从 R2 读取图片 bytes。
- WeChat/Douyin 发布链路兼容 object ref。
- 保留旧 data URL 和外部 URL 兼容。

建议分支：

```text
feature/backend-r2-publish-media
```

### 阶段 5：部署文档与运行手册

交付：

- R2 bucket 创建说明。
- API Token 权限说明。
- 环境变量说明。
- Bucket CORS 示例。
- 本地开发 fallback 说明。
- 故障排查：403、签名过期、CORS、MIME 不匹配、对象不存在。

建议分支：

```text
docs/r2-media-storage-ops
```

## 测试计划

Backend：

- R2 config 缺失时返回明确错误。
- 生成 upload URL 时校验项目编辑权限。
- 禁止 viewer 创建上传。
- 禁止超大小图片。
- 禁止不支持 MIME type。
- complete 时 HEAD 不存在返回失败。
- complete 时 size/mime 不匹配返回失败。
- resolve URL 只允许项目可访问用户读取。
- publish 读取 object ref 成功。
- publish 对 missing/deleted asset 给出可读错误。

Frontend：

- 图片选择后调用 upload URL API。
- PUT 失败时展示错误并不插入图片。
- complete 失败时展示错误并不插入图片。
- 成功后插入图片并带 `data-mpp-media-id`。
- 打开项目时 resolve signed URL。
- 保存时不保存 signed URL。

集成/手动：

1. 配置 R2 bucket 和 CORS。
2. 登录 MPP。
3. 新建项目。
4. 在编辑器上传图片。
5. 刷新页面确认图片仍能显示。
6. 保存项目，确认 DB 中没有新的 base64 图片。
7. 执行发布，确认发布链路能读取 R2 图片。

## 非目标

- 不实现视频。
- 不实现 multipart upload。
- 不实现 CDN。
- 不实现 extension handoff asset signed URL。
- 不删除旧 data URL 兼容。
- 不批量迁移历史内容。

## 风险与处理

### Signed URL 过期

不要把 signed URL 存入数据库。只在 runtime hydrate 或下载前生成。

### CORS 失败

直传 R2 依赖 bucket CORS。部署文档必须包含精确 Origin 和 method/header 配置。

### 发布任务延迟

异步发布可能晚于 frontend 预览 URL TTL。发布链路不能依赖前端生成的 signed URL，应由 backend/publish-worker 自己读取对象或重新生成 signed URL。

### 旧内容兼容

`media.DownloadAndProcess` 需要继续支持旧 data URL 和外部 URL，避免破坏已有项目。
