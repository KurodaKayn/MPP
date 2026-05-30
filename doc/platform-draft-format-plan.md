# 平台草稿多格式适配方案

## 1. 背景

当前项目已经具备“一个项目，多平台发布记录”的基础模型：

- `projects.source_content` 保存项目源稿。
- `project_platform_publications.adapted_content` 保存平台适配后的内容。
- `ProjectPlatformPublication` 以 `(project_id, platform)` 唯一区分同一项目在不同平台的发布目标。

这套模型适合承载多格式草稿，但当前实现还没有把“源稿格式”和“平台目标格式”严格分层。前端编辑器保存的是 HTML；微信公众号发布路径按 HTML 处理；知乎适配字段名是 `markdown`，但现在只是把源 HTML 原样放进去，发布逻辑也没有读取真实 `adapted_content`。

目标需求是：

- 微信公众号草稿使用 HTML。
- 知乎草稿使用 Markdown。
- 后续平台仍能扩展为纯文本、图片笔记、视频动态等格式。
- `dashboard/content` 不再采用左编辑器、右预览的双栏布局，而是把编辑和预览合并为同一个工作区，通过 tab 切换。
- 页面增加“预发布”区块，用户点击同步按钮后，系统才把当前编辑器内容转换为各平台数据格式并持久化。
- 预发布区块既能查看平台原始格式，例如 HTML、Markdown、text，也能查看该格式的预览结果。
- 预发布链路需要给未来 AI agent 介入留出稳定扩展点。

![platform-draft-format-architecture.svg](./assests/platform-draft-format-architecture.svg)。

## 2. 现状判断

### 2.1 已具备的能力

- `Project.SourceContent` 能作为源稿保存字段。
- `ProjectPlatformPublication.AdaptedContent` 能按平台保存派生稿。
- `CreateProject` 和 `UpdateProject` 已经通过 `buildPublicationPayload` 调用平台 `AdaptContent`。
- 微信公众号 `AdaptContent` 当前输出 `{ "format": "html", "html": ... }`，发布时会读取 HTML 并处理正文图片。
- X 发布已经有 HTML 转纯文本的逻辑，可作为“平台派生稿只保存最终发布形态”的参考。

### 2.2 主要缺口

- 源稿格式没有显式标注。系统默认把 `source_content` 当 HTML 用，但字段名没有体现。
- 知乎 `AdaptContent` 没有做 HTML 到 Markdown 的转换。
- 知乎 `AdaptContent` 使用字符串拼 JSON，正文包含引号、反斜杠或换行时存在 JSON 破坏风险。
- 知乎 `Publish` 目前硬编码标题、正文和本地图片，没有使用项目标题和 `pub.AdaptedContent`。
- 前端“知乎预览”仍然渲染 HTML，不是 Markdown 视图。
- `GetProjectPublications` 对 `adapted_content` 做摘要过滤，只返回 `summary` 和 `format`，不适合平台草稿编辑或调试。

## 3. 设计目标

### 3.1 功能目标

- 保存一份规范源稿，并为每个启用平台生成一份派生草稿。
- 微信公众号派生稿必须是 HTML。
- 知乎派生稿必须是 Markdown。
- 发布器只消费本平台派生稿，不再自行猜测或重做主要格式转换。
- 平台派生稿可被重新生成、预览、发布和排错。

### 3.2 工程目标

- 不把多个格式混在 `Project.SourceContent` 里。
- 不在前端重复实现后端发布所需的格式转换规则。
- 每个平台适配规则集中在平台 adapter 或 publisher 包内。
- JSONB 字段保持结构化、可版本化、可回填。
- 增加测试覆盖，避免平台格式回退为“看起来能存，实际不能发”。

### 3.3 非目标

- 不在第一阶段做多用户协同编辑。
- 不在第一阶段做富文本双向无损转换。
- 不要求知乎 Markdown 能 100% 还原微信公众号 HTML 样式。
- 不把所有平台配置拆成固定数据库列，仍保留 JSONB 扩展空间。

## 4. 总体架构

采用“HTML 源稿 + 显式同步 + 平台派生稿”的架构：

1. `dashboard/content` 顶部主工作区合并编辑和预览，用户在 `编辑` / `预览` tab 间切换，不再长期占用右侧预览栏。
2. 前端 TipTap 编辑器输出 canonical HTML，同时保留用于 UI 的纯文本摘要。
3. `保存` 只保存项目源稿和平台选择；`同步到预发布` 是单独动作。
4. 用户点击预发布区块里的同步按钮后，后端按平台调用 adapter：
   - `wechat`: HTML 清洗、图片预处理信息、输出 HTML。
   - `zhihu`: HTML 转 Markdown、图片引用映射、输出 Markdown。
   - `x`: HTML 转纯文本、长度裁剪、输出 text。
5. 后端把派生稿存入 `project_platform_publications.adapted_content`。
6. 预发布区块读取并展示这些派生稿，提供 `原始格式` 与 `预览` 两种查看方式。
7. 发布时 `PublishProject` 读取对应平台的 `adapted_content`，交给对应 publisher。
8. publisher 只处理平台 API、鉴权、远端媒体上传和最终提交。
9. AI agent 未来作为 adapter 前后的可插拔 intervention，不直接绕过源稿、派生稿和发布器边界。

## 5. 数据模型方案

### 5.1 保持现有关系模型

不需要新增一张“草稿格式表”。现有模型已经表达了：

- 一个项目：`projects`
- 多个平台目标：`project_platform_publications`
- 每个平台目标一份派生稿：`adapted_content`

第一阶段建议只增强 JSON 契约，不改表结构。

### 5.2 源稿字段约定

`projects.source_content` 在第一阶段明确约定为 HTML：

```json
{
  "source_content": "<h1>标题</h1><p>正文</p>"
}
```

可选增强字段：

```sql
ALTER TABLE projects
ADD COLUMN source_format text NOT NULL DEFAULT 'html';
```

是否增加 `source_format` 取决于是否预计未来允许 Markdown 原稿、导入 Word、导入公众号历史文章等场景。如果短期只有 TipTap 编辑器，先用代码常量约束即可。

### 5.3 平台派生稿 JSON 契约

微信公众号：

```json
{
  "schema_version": 1,
  "format": "html",
  "summary": "用于列表和调试的摘要",
  "source_revision": "2026-05-30T14:30:00Z",
  "generated_by": {
    "type": "system",
    "id": "wechat-html-adapter",
    "version": "1"
  },
  "html": "<p>适合微信草稿接口的 HTML</p>",
  "assets": [
    {
      "type": "image",
      "source_url": "https://example.com/a.png",
      "alt": "配图"
    }
  ]
}
```

知乎：

```json
{
  "schema_version": 1,
  "format": "markdown",
  "summary": "用于列表和调试的摘要",
  "source_revision": "2026-05-30T14:30:00Z",
  "generated_by": {
    "type": "system",
    "id": "zhihu-markdown-adapter",
    "version": "1"
  },
  "markdown": "## 小标题\n\n正文内容\n\n![配图](https://example.com/a.png)",
  "assets": [
    {
      "type": "image",
      "source_url": "https://example.com/a.png",
      "alt": "配图"
    }
  ]
}
```

X：

```json
{
  "schema_version": 1,
  "format": "text",
  "summary": "最终短文本",
  "source_revision": "2026-05-30T14:30:00Z",
  "generated_by": {
    "type": "system",
    "id": "x-text-adapter",
    "version": "1"
  },
  "text": "最终短文本"
}
```

`generated_by` 用于给未来 AI agent 介入留出审计空间。第一阶段全部由系统 adapter 生成；后续可以记录：

- `type=agent`: 由 AI agent 生成或改写。
- `type=user`: 用户手工改写平台稿。
- `source_revision`: 生成时对应的源稿版本。
- `agent_run_id`: 对应一次 agent 运行记录。
- `instructions`: agent 接收的用户意图摘要。

### 5.4 状态字段约定

保留现有 publication 状态：

- `pending`: 没有注册 adapter 或尚未适配。
- `adapted`: 已生成平台派生稿。
- `publishing`: 正在提交远端平台。
- `published`: 远端平台已接受。
- `failed`: 适配或发布失败。
- `disabled`: 用户未选择该平台。

建议补充约定：

- 适配失败时，不更新旧的 `adapted_content`，只写 `error_message`。
- 源稿更新后，已启用平台的 `adapted_content` 标记为过期，但不自动覆盖。用户点击预发布同步后再重新生成，并清空旧 `remote_id`、`publish_url`、`published_at`。
- 已发布记录被重新编辑时，状态回到 `adapted`，由 UI 提示“本地内容已变更，需重新发布”。
- 发布前必须确认平台派生稿没有过期；如果过期，先要求同步。

## 6. 后端模块设计

### 6.1 新增内容适配边界

建议把格式转换从 publisher 中抽出一层 adapter：

```go
type PlatformContentAdapter interface {
    Platform() string
    TargetFormat() string
    Adapt(project *models.Project, opts AdaptOptions) (AdaptedContent, error)
}
```

`AdaptedContent` 用强类型结构承载，再统一 `json.Marshal`：

```go
type AdaptedContent struct {
    SchemaVersion int             `json:"schema_version"`
    Format        string          `json:"format"`
    Summary       string          `json:"summary"`
    SourceRevision string         `json:"source_revision"`
    GeneratedBy   GeneratedBy     `json:"generated_by"`
    HTML          string          `json:"html,omitempty"`
    Markdown      string          `json:"markdown,omitempty"`
    Text          string          `json:"text,omitempty"`
    Assets        []AdaptedAsset  `json:"assets,omitempty"`
}

type GeneratedBy struct {
    Type         string `json:"type"`
    ID           string `json:"id"`
    Version      string `json:"version,omitempty"`
    AgentRunID   string `json:"agent_run_id,omitempty"`
    Instructions string `json:"instructions,omitempty"`
}
```

短期可以不新增接口文件，先把现有 `PlatformPublisher.AdaptContent` 按这个契约改造；中期再拆出 adapter，避免 publisher 同时负责转换和发布。

同步动作建议落在 service 层：

```go
func (s *DashboardService) SyncProjectPrepublish(
    projectID uuid.UUID,
    userID uuid.UUID,
    platforms []string,
    actor SyncActor,
) (*dto.ProjectPublicationsResponse, error)
```

`actor` 第一阶段固定为系统；未来 AI agent 介入时复用同一入口，只改变 `actor` 和 adapter 前后的处理步骤。

### 6.2 微信公众号适配

职责：

- 接收 HTML 源稿。
- 保留段落、标题、列表、引用、图片。
- 清理微信不支持或风险较高的标签和属性。
- 输出 `format=html`。

发布器职责：

- 读取 `adapted_content.html`。
- 下载、压缩、上传正文图片。
- 替换图片 URL。
- 创建微信草稿。
- 如果账号无直接发布权限，保留草稿 ID 并提示人工发布。

### 6.3 知乎适配

职责：

- 接收 HTML 源稿。
- 转换为 Markdown。
- 保留标题层级、段落、粗体、斜体、链接、列表、引用、代码块、图片。
- 输出 `format=markdown`。

推荐实现：

- 使用 Go 侧 HTML-to-Markdown 库，例如 [`github.com/JohannesKaufmann/html-to-markdown/v2`](https://pkg.go.dev/github.com/JohannesKaufmann/html-to-markdown/v2)。该库在 `pkg.go.dev` 可查，支持通过 `go get -u github.com/JohannesKaufmann/html-to-markdown/v2` 引入。
- 通过 package manager 修改依赖，不手动编辑 `go.mod` / `go.sum`。
- 对 TipTap 常见 HTML 节点做单元测试。

发布器职责：

- 读取 `adapted_content.markdown`。
- 读取项目标题或 `config.title`。
- 使用浏览器自动化进入知乎写作页面。
- 将 Markdown 内容粘贴到编辑器，必要时按知乎编辑器能力降级为纯文本粘贴。
- 图片策略先支持远程 URL Markdown；本地 `data:` 图片需要先上传或转换为剪贴板图片。

### 6.4 X 适配

职责：

- 接收 HTML 源稿。
- 转换为纯文本。
- 拼接标题和正文。
- 按 X 权重规则裁剪到 280。
- 输出 `format=text`。

现有 X 逻辑已经接近目标，只需要补齐 `schema_version` 和统一结构。

## 7. API 设计

### 7.1 保存项目

保存项目沿用现有接口，但职责调整为只保存源稿、标题和平台选择，不在每次保存时自动覆盖平台派生稿：

```http
POST /api/user/dashboard/projects
PUT /api/user/dashboard/projects/:projectId
```

请求体仍传：

```json
{
  "title": "标题",
  "source_content": "<p>HTML 源稿</p>",
  "summary": "纯文本摘要",
  "cover_image_url": "https://example.com/cover.png",
  "platforms": ["wechat", "zhihu"]
}
```

后端在事务内完成：

- 保存 `projects.source_content`。
- 保存用户选择的平台目标。
- 禁用未选择平台。
- 将已启用平台派生稿标记为需要同步，或者通过 `adapted_content.source_revision` 判断是否过期。

### 7.2 同步到预发布

新增同步接口：

```http
POST /api/user/dashboard/projects/:projectId/prepublish/sync
```

请求体：

```json
{
  "platforms": ["wechat", "zhihu"],
  "actor": {
    "type": "system"
  }
}
```

响应体返回最新平台派生稿摘要和必要的完整内容：

```json
{
  "project_id": "...",
  "items": [
    {
      "platform": "wechat",
      "status": "adapted",
      "adapted_content": {
        "schema_version": 1,
        "format": "html",
        "summary": "...",
        "html": "<p>...</p>"
      }
    },
    {
      "platform": "zhihu",
      "status": "adapted",
      "adapted_content": {
        "schema_version": 1,
        "format": "markdown",
        "summary": "...",
        "markdown": "..."
      }
    }
  ]
}
```

同步语义：

- 只同步当前已选择平台，除非请求体显式指定平台列表。
- 同步成功会覆盖对应平台旧 `adapted_content`。
- 同步失败不覆盖旧平台稿，只更新错误信息。
- 同步完成后预发布区块直接展示数据库里的 `adapted_content`，而不是前端临时转换结果。

### 7.3 获取项目详情

现有 `GetProject` 只返回源稿和 publication summary，适合编辑器初始化。

建议新增或扩展平台草稿详情接口：

```http
GET /api/user/dashboard/projects/:projectId/publications
```

返回摘要时默认不暴露完整正文：

```json
{
  "project_id": "...",
  "items": [
    {
      "platform": "wechat",
      "status": "adapted",
      "adapted_content": {
        "schema_version": 1,
        "format": "html",
        "summary": "..."
      }
    }
  ]
}
```

如需预览或调试完整平台草稿，建议加显式参数：

```http
GET /api/user/dashboard/projects/:projectId/publications?include_content=true
```

这样可以避免列表接口传输大正文，也方便权限和脱敏控制。

### 7.4 单平台重新适配

同步接口也可以提供单平台入口，便于预发布区块里只刷新一个 tab：

```http
POST /api/user/dashboard/projects/:projectId/publications/:platform/adapt
```

用途：

- 调试单个平台转换结果。
- 平台规则升级后重新生成派生稿。
- 适配失败后局部重试。

## 8. 前端交互设计

### 8.1 页面结构

`dashboard/content` 调整为四个纵向区块：

1. 页面头部：标题、保存、发布设置入口。
2. 内容工作区：编辑和预览合并在同一个模块，通过 tab 切换。
3. 预发布区块：选择平台、同步生成平台稿、查看原始格式和预览结果。
4. 发布区块：只负责提交已经同步好的平台稿。

不再使用当前“左侧编辑器 + 右侧平台预览”的双栏结构。原因是平台预览以后不只是前端临时预览，而是和后端 `adapted_content` 绑定，需要在预发布区块里展示。

### 8.2 内容工作区

内容工作区是一个主模块，包含两个 tab：

- `编辑`: TipTap 编辑器、标题输入、正文编辑、图片插入。
- `预览`: 当前源稿的即时预览，展示编辑器 HTML 的渲染结果。

这一层只处理源稿，不展示各平台最终格式：

- 编辑器输出 HTML。
- 本地 `ContentValue.text` 只用于摘要、字数和低保真预览。
- 保存时继续传 `source_content=content.html`。
- 预览 tab 是源稿预览，不代表微信或知乎最终发布格式。

### 8.3 预发布区块

预发布区块是平台稿的工作台，包含：

- 平台选择：沿用现有平台 checkbox 或平台卡片。
- 同步按钮：`同步到预发布`。
- 同步状态：未同步、已同步、源稿已变更需重新同步、同步失败。
- 平台 tab：微信公众号、知乎、X、B站、小红书等。
- 视图切换：`原始格式` / `预览`。

同步按钮行为：

1. 如果项目还没有保存，先保存项目源稿。
2. 调用 `POST /api/user/dashboard/projects/:projectId/prepublish/sync`。
3. 后端生成并保存各平台 `adapted_content`。
4. 前端刷新预发布区块。

`原始格式` 视图展示真正存入数据库、发布器将消费的内容：

- 微信公众号：HTML 字符串，可用只读代码视图展示。
- 知乎：Markdown 字符串。
- X：纯文本。

`预览` 视图按平台格式渲染：

- 微信公众号：渲染 HTML。
- 知乎：渲染 Markdown；第一阶段可以先用纯文本 Markdown 预览，后续再通过 pnpm 引入 Markdown 渲染器。
- X：展示最终短文本和字符权重。

### 8.4 发布区块

发布区块只允许发布已同步且未过期的平台稿：

- 如果平台未同步，发布按钮置灰并提示先同步。
- 如果源稿保存时间晚于平台稿生成时间，提示重新同步。
- 如果平台稿格式和 publisher 要求不一致，禁止发布。
- X 的手动发布链接也应优先读取 `adapted_content.text`。

### 8.5 AI Agent 扩展点

预发布区块是未来 AI agent 介入的主要入口，而不是编辑器正文或 publisher 内部。

建议预留三个 agent action：

- `优化平台稿`: 在平台 adapter 生成后，对单个平台稿做语气、标题、结构优化。
- `批量适配`: 基于源稿和平台规则，一次生成多个平台稿。
- `修复发布失败`: 根据错误信息和平台稿内容，建议或执行最小修改。

agent 介入必须通过同一套同步/适配入口写回 `adapted_content`：

- 写入 `generated_by.type=agent`。
- 写入 `agent_run_id`。
- 保留 `source_revision`，避免 agent 基于旧源稿改写。
- 不允许 agent 绕过 `PublishProject` 直接调用平台 publisher。

### 8.6 状态提示

UI 需要区分：

- 源稿未保存。
- 源稿已保存但未同步预发布。
- 平台稿已同步。
- 源稿已变更，平台稿过期。
- 同步失败但旧平台稿仍可查看。
- 发布失败但平台稿仍可查看。

## 9. 发布链路设计

### 9.1 通用发布流程

```text
PublishProject(projectID, platform)
  -> 校验项目归属
  -> 读取 ProjectPlatformPublication
  -> 校验 enabled/status
  -> 校验平台稿已同步且未过期
  -> 注入平台账号凭证
  -> 校验 adapted_content.format
  -> 调用 publisher.Publish
  -> 写回 remote_id/publish_url/status/error_message
```

关键约束：

- publisher 必须拒绝不匹配的 `format`。
- 发布前不应该用 `Project.SourceContent` 临时替代平台派生稿，除非明确触发重新适配。
- 发布动作不负责生成平台稿；平台稿必须由预发布同步动作提前生成。
- DB 更新错误必须返回给调用方，不能忽略。

### 9.2 微信公众号发布

输入：

- `adapted_content.format=html`
- `adapted_content.html`
- `config.title`
- `config.digest`
- `config.cover_image_url`

输出：

- 成功创建草稿：写入 `remote_id=draft_media_id`。
- 成功发布：写入 `publish_url`。
- 无发布权限：保留草稿 ID，状态可设为 `failed` 或新增 `manual_required`。第一阶段若不改状态枚举，可继续用 `failed` 加明确错误信息。

### 9.3 知乎发布

输入：

- `adapted_content.format=markdown`
- `adapted_content.markdown`
- `config.title`
- `account.cookies`

输出：

- 发布成功：写入知乎文章 URL。
- 登录失效：写入可操作错误信息。
- 编辑器不接受 Markdown 时：降级为纯文本粘贴，但仍保留 Markdown 派生稿用于后续优化。

## 10. 迁移和回填策略

### 10.1 不改表结构版本

如果第一阶段不增加 `source_format`：

- 现有 `source_content` 全部按 HTML 解释。
- 对所有已启用 publication 重新执行当前平台 adapter。
- 无 publisher 的平台保持 `pending`。

### 10.2 增加 `source_format` 版本

迁移：

```sql
ALTER TABLE projects
ADD COLUMN source_format text NOT NULL DEFAULT 'html';
```

回填：

- 所有历史项目默认 `source_format='html'`。
- 对 `adapted_content` 缺少 `schema_version` 的记录重新生成。

### 10.3 回滚策略

- 保留原 `source_content` 不变。
- `adapted_content` 是派生数据，可重复生成。
- 发布状态、远端 ID、发布 URL 不应在批量回填时清空，除非用户重新保存源稿。

## 11. 测试方案

### 11.1 后端单元测试

微信公众号：

- HTML 源稿输出 `format=html`。
- 图片、链接、标题、列表保留。
- JSON 使用 `json.Marshal`，正文包含引号和换行仍合法。

知乎：

- HTML 标题转换为 Markdown 标题。
- `<strong>` 转为 `**bold**`。
- `<a href>` 转为 Markdown 链接。
- `<img alt src>` 转为 Markdown 图片。
- 空内容返回明确错误。
- `Publish` 使用 `adapted_content.markdown`，不再使用硬编码正文。

X：

- 保持当前长度裁剪测试。
- 补充统一结构测试。

服务层：

- `CreateProject` / `UpdateProject` 只保存源稿和平台选择，不自动覆盖已有平台稿。
- `SyncProjectPrepublish` 为微信和知乎分别生成 HTML/Markdown 派生稿。
- `UpdateProject` 后已启用平台稿会被识别为过期。
- 适配失败时事务回滚或返回明确错误。
- `PublishProject` 拒绝发布未同步或已过期的平台稿。

### 11.2 前端测试

- 保存项目仍发送 HTML 源稿。
- 内容工作区能在 `编辑` / `预览` tab 间切换。
- 预发布同步按钮会先保存源稿，再调用同步接口。
- 预发布区块能识别后端返回的 `format`。
- 原始格式视图展示 HTML/Markdown/text 原文。
- 预览视图不再把源稿 HTML 当知乎最终 Markdown 展示。
- 源稿变更后，预发布区块提示重新同步。

### 11.3 集成测试

- 创建一篇含标题、段落、图片、链接、列表的文章。
- 选择微信和知乎。
- 保存源稿后，预发布区块显示未同步。
- 点击同步。
- 验证数据库中：
  - `projects.source_content` 是 HTML。
  - 微信 `adapted_content.format=html`。
  - 知乎 `adapted_content.format=markdown`。
- 发布微信时使用 HTML。
- 发布知乎时使用 Markdown。

## 12. 分阶段实施计划

### 阶段 1：页面交互和同步边界

- 将 `dashboard/content` 改为主工作区 tab：`编辑` / `预览`。
- 移除右侧固定平台预览栏。
- 增加预发布区块和同步按钮。
- 保存源稿与同步平台稿分离。

交付标准：

- 用户能在一个模块中切换源稿编辑和源稿预览。
- 用户能通过预发布区块显式同步平台稿。
- 发布入口能感知未同步或已过期状态。

### 阶段 2：格式契约收敛

- 定义 `AdaptedContent` 强类型结构。
- 修改微信、知乎、X 的 `AdaptContent` 输出统一结构。
- 修复知乎 JSON 拼接问题。
- 增加后端单元测试。
- 所有派生稿包含 `generated_by`，为 AI agent 介入留出审计字段。

交付标准：

- 同步预发布时微信输出 HTML，知乎输出 Markdown，X 输出 text。
- 所有派生稿包含 `schema_version`、`format`、`summary`、`generated_by`。

### 阶段 3：知乎 Markdown 转换

- 所有依赖改动都通过对应包管理器完成；前端用 `pnpm`，后端 Go 依赖用 `go get` / `go mod tidy`，不手动编辑依赖文件。
- 引入 HTML-to-Markdown 转换库。
- 补齐 TipTap 常见节点转换测试。
- `ZhihuPublisher.Publish` 改为读取 `adapted_content.markdown` 和标题。

交付标准：

- 不再出现硬编码知乎标题和正文。
- HTML 源稿能稳定生成可读 Markdown。

### 阶段 4：平台草稿预览

- 扩展 publication detail API，支持按需返回完整 `adapted_content`。
- 预发布区块展示后端适配结果。
- 支持 `原始格式` / `预览` 视图切换。
- 知乎预览展示 Markdown。

交付标准：

- 用户能看到真正将被发布的平台草稿。
- 列表页不传输完整大正文。

### 阶段 5：发布可靠性增强

- 发布前校验 `adapted_content.format`。
- 发布前校验平台稿未过期。
- 检查所有 DB 更新错误。
- 统一远端发布失败错误分类。
- 为微信无发布权限场景设计 `manual_required` 或明确的失败状态文案。

交付标准：

- 发布结果、本地状态和远端状态不再明显不一致。

### 阶段 6：AI Agent 介入

- 增加 agent action 的后端入口。
- agent 只读源稿、平台规则和当前平台稿。
- agent 写回仍走 `adapted_content`，并记录 `generated_by.type=agent`。
- 预发布区块展示 agent 生成来源、运行时间和可回滚版本。

交付标准：

- agent 能对单个平台稿生成建议或直接写回。
- 用户能区分系统 adapter 生成和 agent 生成的平台稿。
- 发布链路不需要知道平台稿来自系统、用户还是 agent。

## 13. 风险与取舍

- HTML 到 Markdown 不是无损转换，知乎首版应以可读和可发布为目标。
- 微信 HTML 支持范围和浏览器 HTML 不完全一致，需要持续维护清洗规则。
- 知乎编辑器对 Markdown 粘贴能力可能变化，发布器需要保留降级路径。
- `adapted_content` 存完整正文会增大数据库体积，但换来可审计、可重试、可预览。
- 如果未来要支持平台级手工微调，需要进一步区分“自动派生稿”和“用户编辑后的平台稿”。

## 14. 验收标准

- 同一项目选择微信和知乎后，数据库里存在两条启用 publication。
- 保存源稿不会自动覆盖平台派生稿。
- 点击预发布同步后才生成或覆盖平台派生稿。
- 微信 publication 的 `adapted_content.format` 为 `html`，正文在 `html` 字段。
- 知乎 publication 的 `adapted_content.format` 为 `markdown`，正文在 `markdown` 字段。
- 知乎 Markdown 中不包含未经转换的大段 HTML 标签。
- 预发布区块能显示原始格式和预览结果。
- 源稿变更后，发布前会提示重新同步。
- `generated_by` 能标识系统 adapter 或未来 agent 写回。
- 发布器不再使用硬编码测试内容。
- 单元测试覆盖微信、知乎、X 三种目标格式。
- SVG 架构图能说明编辑/预览合一、预发布同步、平台格式存储、AI agent 扩展和发布边界。
