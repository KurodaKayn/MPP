# MPP SaaS 功能设计计划

## 1. 目标与边界

目标：把 MPP 从单用户发布工具演进为团队型多平台内容发布工作台。

范围只包含 SaaS 产品与业务设计：

- 工作区、成员、角色、邀请、权限和审计。
- 平台账号连接、共享授权、健康状态和重登流程。
- 内容项目、协同编辑、模板、品牌预设、素材库、评论和版本。
- 平台草稿、目标账号、排期发布、发布日历和结果追踪。
- 发布审批、待办、通知。
- 套餐、配额、用量、账单状态和后台管理。

不包含高并发、分布式部署、队列、容器调度、网关、数据库扩容方案。

## 2. 当前业务底座

已具备：

- `Workspace`、`WorkspaceMember`、`WorkspaceActivity`，支持工作区、成员和活动记录。
- `Project` 作为内容主对象，已关联 `workspace_id`、协同文档和平台发布记录。
- 项目协作者、分享链接、评论、版本记录和项目活动。
- `MediaAsset`，已带 `workspace_id`、`project_id` 和对象存储边界。
- `PlatformAccount`，当前按用户和平台保存账号、凭据、Cookie、状态和测试结果。
- `RemoteBrowserSession`，支持平台登录态捕获，当前按用户和平台管理会话。
- `ProjectPlatformPublication`，保存平台草稿、发布状态、远端 ID、发布 URL、错误和重试次数。
- `PublishEvent`，记录发布执行事件。

主要缺口：

- 工作区还不是明确的 SaaS 租户、套餐、用量和账单归属。
- 平台账号仍偏个人资产，缺少工作区级共享、账号别名、多账号选择和授权可见性。
- 发布权限仍偏 owner-only，缺少发布者权限、审批边界和目标账号授权。
- 内容生产已有项目协作，但缺少模板、品牌预设、素材库和发布前检查。
- 发布已有状态与事件，但缺少排期模型、发布日历、人工处理状态和版本冻结。
- 缺少套餐、配额、用量事件、账单状态和配额拦截点。
- 审计与通知分散，缺少统一事件流和待办入口。

## 3. 核心设计

### 3.1 工作区

工作区是 SaaS 租户、协作、资源归属和计费边界。

字段方向：

- `workspaces`: 增加 `plan_code`、`billing_status`、`billing_customer_ref`、`trial_ends_at`、`default_timezone`、`settings`。
- `workspace_members`: 保留 `owner`、`admin`、`member`、`viewer`。
- `workspace_activities`: 扩展事件类型，覆盖邀请、账号、排期、发布、审批、配额和账单状态变化。

新增对象：

- `WorkspaceInvite`: 邮箱、角色、邀请人、过期时间、接受状态。
- `WorkspaceSeat`: 成员席位状态、加入时间、释放时间。

规则：

- 个人工作区自动创建，团队工作区支持邀请成员、管理账号和管理套餐。
- 所有 dashboard 查询带当前 `workspace_id`。
- 新业务资源默认必须带 `workspace_id`。
- 工作区切换后，内容、账号、素材、模板、排期、统计、账单都按当前工作区过滤。

### 3.2 成员与权限

角色先少，权限点明确。服务层判断权限点，不散落判断角色名。

| 权限 | owner | admin | member | viewer |
| --- | --- | --- | --- | --- |
| 管理账单 | 是 | 可配置 | 否 | 否 |
| 管理成员 | 是 | 是 | 否 | 否 |
| 管理平台账号 | 是 | 是 | 否 | 否 |
| 使用授权账号 | 是 | 是 | 是 | 否 |
| 创建项目 | 是 | 是 | 是 | 否 |
| 编辑项目 | 是 | 是 | 是 | 否 |
| 评论审阅 | 是 | 是 | 是 | 是 |
| 发起发布 | 是 | 是 | 是 | 否 |
| 审批发布 | 是 | 是 | 可配置 | 否 |
| 查看发布日历 | 是 | 是 | 是 | 是 |

关键权限点：

- `workspace.manage_billing`
- `workspace.manage_members`
- `account.connect`
- `account.manage`
- `account.use`
- `project.create`
- `project.edit`
- `project.review`
- `publication.approve`
- `publication.publish`
- `publication.schedule`

规则：

- 项目级协作者继续保留，用于覆盖工作区默认权限。
- 工作区权限先算基础角色，再叠加项目直接授权。
- 发布权限必须同时满足项目编辑权、账号使用权、审批状态和配额余量。
- 前端隐藏无权限入口，后端仍做强校验。

### 3.3 平台账号资产

平台账号从个人连接记录升级为工作区可授权资产。

字段方向：

- `platform_accounts`: 增加 `workspace_id`、`owner_user_id`、`connected_by_user_id`、`display_name`、`platform_user_id`、`share_scope`、`last_connected_at`、`last_verified_at`、`expires_at`、`health_status`、`credential_secret_ref`。
- 唯一约束从 `user_id + platform` 调整为 `workspace_id + platform + platform_user_id`，缺远端 ID 时用 `workspace_id + platform + display_name`。
- `remote_browser_sessions`: 增加 `workspace_id`、`platform_account_id`。

新增对象：

- `PlatformAccountGrant`: 授权账号给成员或项目，角色为 `manager`、`publisher`、`viewer`。
- `PlatformAccountHealthCheck`: 最近检测结果、错误分类、下次检测时间。

规则：

- 账号连接入口位于当前工作区。
- 用户可选择“仅自己可用”或“工作区可用”。
- Cookie 捕获后写入密钥存储，业务表只保留 `credential_secret_ref`。
- 发布任务只引用 `platform_account_id`，不复制 Cookie。
- 账号断连、Cookie 过期、凭据测试失败、平台风控时标记 `needs_reauth`。
- 账号移除前提示受影响排期和草稿。

### 3.4 内容项目

`Project` 继续作为内容生产主对象，不新增重复内容主表。

字段方向：

- `Project.workspace_id` 必填。
- `Project.template_id` 可选。
- `Project.brand_profile_id` 可选。
- `Project.status`: `draft`、`reviewing`、`approved`、`scheduled`、`publishing`、`published`、`failed`。

规则：

- 项目列表支持按状态、平台、作者、更新时间、排期状态筛选。
- 项目详情整合源内容、平台草稿、评论、版本、活动和发布结果。
- 项目权限展示来源：owner、工作区角色、项目协作者、分享链接。
- `ProjectVersion` 作为审批、回滚和排期发布依据。
- 版本恢复后提示重新同步平台草稿。

### 3.5 模板、品牌与素材库

新增对象：

- `ContentTemplate`: 标题规则、正文结构、标签规则、默认平台、平台参数、适用范围。
- `BrandProfile`: 语气、称谓、禁用词、CTA、链接策略、默认标签。
- `ProjectChecklist`: 封面、标题、平台账号、审批、素材状态等发布前检查项。
- `MediaAssetVariant`: 裁剪、压缩、封面规格、平台适配结果。
- `MediaAssetUsage`: 素材被哪些 project、publication、template 使用。

字段方向：

- `media_assets`: 增加 `library_scope`、`tags`、`alt_text`、`source`、`derivative_of`。

规则：

- 模板分系统、工作区、个人三层。
- AI 编辑带入模板和品牌上下文，但只生成提案；用户接受后才落库。
- 上传素材先进入 `pending`，处理完成变 `ready`。
- 发布前只允许引用 `ready` 素材。
- 删除素材默认软删；被已发布记录引用时禁止物理删除。

### 3.6 平台草稿与发布目标

当前 `ProjectPlatformPublication` 是 `project + platform`，后续要表达 `project + platform + account`。

字段方向：

- 增加 `platform_account_id`，支持同一平台多个账号。
- `config` 存平台发布参数，如合集、标签、可见性、原创声明。
- `adapted_content` 存平台草稿内容。
- `status` 继续表达草稿、同步中、排队、发布中、成功、失败、取消。

规则：

- 平台草稿面板支持每个平台选择目标账号。
- 同一项目允许同平台多目标账号，UI 默认一平台一账号。
- 平台草稿变更后标记需同步或需重新审批。
- 发布结果按账号维度展示，不只按平台展示。
- 失败结果保留可读错误、失败时间、重试入口和账号健康入口。

### 3.7 排期发布

排期是独立业务对象，不塞进项目状态。

新增对象：

- `ScheduledPublication`
  - `workspace_id`
  - `project_id`
  - `publication_id`
  - `platform_account_id`
  - `project_version_id`
  - `scheduled_at`
  - `timezone`
  - `status`: `draft`、`pending_review`、`approved`、`scheduled`、`running`、`published`、`failed`、`needs_manual_action`、`cancelled`
  - `idempotency_key`
  - `created_by`
  - `approved_by`
  - `cancelled_by`
  - `last_error`
- `PublishAttempt`
  - `scheduled_publication_id`
  - `attempt_no`
  - `started_at`
  - `finished_at`
  - `status`
  - `remote_id`
  - `publish_url`
  - `error_code`
  - `error_message`

规则：

- 立即发布也是 `ScheduledPublication`，`scheduled_at = now`。
- 排期绑定 `ProjectVersion`；内容变更后必须重新确认或重新审批。
- 排期到期前校验账号状态、账号授权、平台草稿同步、权限、配额。
- 浏览器发布遇到验证码、二次确认、登录过期时，状态转 `needs_manual_action`，保留远程会话入口直到 TTL。
- 失败可重试，但重试必须复用同一 `idempotency_key`。
- 取消、失败、重新排期都有独立记录。

### 3.8 审批

审批只服务发布前质量控制，不做复杂流程引擎。

新增对象：

- `ApprovalRequest`
  - `workspace_id`
  - `project_id`
  - `project_version_id`
  - `publication_id`
  - `platform_account_id`
  - `scheduled_publication_id`
  - `requested_by`
  - `status`: `pending`、`approved`、`rejected`、`cancelled`
  - `due_at`
- `ApprovalDecision`
  - `approval_request_id`
  - `reviewer_user_id`
  - `decision`
  - `comment`
  - `created_at`

规则：

- 工作区设置支持“发布前需要审批”。
- 需要审批时，member 可提交发布申请，owner/admin 或有审批权限成员审批。
- 审批记录绑定项目版本、平台草稿、目标账号和排期。
- 草稿内容、目标账号或排期内容变更后，已通过审批自动失效。
- 审批评论走现有 `ProjectComment`，不写入正文。

### 3.9 套餐、配额与用量

套餐控制产品能力和资源上限，不参与部署架构。

新增对象：

- `Plan`: 成员数、平台账号数、月发布数、月排期数、AI 用量、浏览器分钟数、媒体存储、历史版本保留时间。
- `WorkspaceSubscription`: 当前套餐、账单状态、账期、外部客户引用。
- `UsageEvent`: 追加式用量事件。
- `UsageAggregate`: 当前周期聚合计数。
- `QuotaOverride`: 临时加量或人工调整。

用量事件字段：

- `workspace_id`
- `actor_user_id`
- `event_type`
- `quantity`
- `unit`
- `resource_type`
- `resource_id`
- `idempotency_key`
- `created_at`

配额拦截点：

- 邀请成员。
- 连接平台账号。
- 上传媒体。
- AI 编辑。
- 启动浏览器会话。
- 立即发布。
- 排期发布。

规则：

- 先实现手动套餐和配额，不阻塞产品验证。
- 超限阻止新资源创建或新任务启动，不中断已运行任务。
- 用量先写 `UsageEvent`，再异步聚合到 `UsageAggregate`。
- 所有计量事件带 `idempotency_key`，防止重试重复扣量。
- 超额返回统一错误码，前端展示升级或清理动作。

### 3.10 审计、通知与后台

统一新增 `AuditEvent`：

- `workspace_id`
- `actor_user_id`
- `event_type`
- `resource_type`
- `resource_id`
- `metadata`
- `created_at`

统一新增 `Notification`：

- `workspace_id`
- `recipient_user_id`
- `event_type`
- `resource_type`
- `resource_id`
- `status`: `unread`、`read`、`archived`
- `created_at`

重点事件：

- 成员加入、移除、角色变更。
- 平台账号连接、授权、重登、删除。
- 项目创建、版本保存、审批通过或拒绝。
- 排期创建、取消、发布开始、发布成功、发布失败、需人工处理。
- 配额接近上限、超限、账单状态变更。
- 后台改套餐、加额度、冻结发布能力。

规则：

- 站内通知先做，邮件和 IM 后置。
- 审批待办、账号需重登、发布失败、需人工处理、配额告警必须生成通知。
- 审计事件不可由普通用户删除。
- 后台可查看审计活动，但不可查看敏感凭据。

## 4. 阶段计划

### 阶段一：工作区 SaaS 基座

目标：所有业务入口都有明确工作区上下文。

状态：已完成。

任务：

- [x] 后端统一 `workspace_id` 上下文解析和权限校验。
- [x] 前端 dashboard 路由和 API 请求统一携带当前工作区。
- [x] 项目、账号、素材、活动、设置页全部按当前工作区过滤。
- [x] 个人工作区自动创建和历史项目回填校验。
- [x] 新增 `WorkspaceInvite`，支持邀请、接受、过期、撤销。
- [x] 建立角色到权限点映射，补齐服务层 guard。
- [x] 增加越权测试：跨工作区读取、编辑、发布、账号使用都必须拒绝。

验收：

- [x] 切换工作区后，看不到其他工作区项目、账号、素材和排期。
- [x] 无权限成员无法通过 API 修改成员、账号、发布和设置。

### 阶段二：平台账号工作区化

目标：平台账号成为团队可管理、可授权、可追踪的资源。

任务：

- [ ] 调整账号模型，支持 `workspace_id`、`owner_user_id`、`connected_by_user_id`、账号别名和远端账号 ID。
- [ ] 迁移旧 `user_id + platform` 数据到个人工作区。
- [ ] 建 `PlatformAccountGrant`，支持账号对成员或项目授权。
- [ ] 远程浏览器登录会话绑定工作区和平台账号。
- [ ] Cookie/credential 存储改为 secret ref，业务响应不返回敏感字段。
- [ ] 账号管理页支持连接、测试、共享、重命名、重登、移除。
- [ ] 发布前必须选择可用且已授权账号。
- [ ] 账号异常统一展示并阻止对应发布。

验收：

- 同一工作区可管理多个同平台账号。
- 成员只能使用自己有权限的账号。
- 账号失效后新发布被阻止，并产生通知。

### 阶段三：协作内容生产闭环

目标：团队能围绕项目完成编辑、模板复用、审阅、版本恢复和平台草稿准备。

任务：

- [ ] 项目详情整合源内容、平台草稿、评论、版本、活动和发布结果。
- [ ] 新增 `ContentTemplate`，支持系统、工作区、个人范围。
- [ ] 新增 `BrandProfile`，绑定工作区，可被项目引用。
- [ ] `Project` 增加 `template_id`、`brand_profile_id`。
- [ ] `MediaAsset` 补齐 library scope、tags、alt text、source。
- [ ] 新增 `MediaAssetUsage`，记录素材引用关系。
- [ ] 平台草稿同步、编辑、审核状态清晰。
- [ ] 版本恢复后提示重新同步平台草稿。
- [ ] 分享链接和项目协作者权限与工作区权限合并展示。

验收：

- 从模板创建项目可生成初始标题、正文结构和默认平台。
- 项目可引用工作区素材，发布前能校验素材 ready 状态。
- editor 能编辑内容但不能绕过发布权限。
- viewer 只能查看内容、草稿和结果。

### 阶段四：排期发布与日历

目标：立即发布、定时发布、失败重试、人工处理统一到一个发布计划模型。

任务：

- [ ] 新增 `ScheduledPublication` 和 `PublishAttempt`。
- [ ] 立即发布改为创建 `ScheduledPublication(scheduled_at=now)`。
- [ ] `ScheduledPublication` 绑定 `project_version_id`、`publication_id`、`platform_account_id`。
- [ ] 内容页支持立即发布、排期发布、取消排期。
- [ ] 发布日历支持日/周/月视图。
- [ ] 排期到期前校验账号状态、权限、配额、草稿同步状态。
- [ ] 增加 `needs_manual_action` 状态，记录远程会话入口和过期时间。
- [ ] 发布详情页展示 attempts、失败原因、重试入口和人工处理入口。

验收：

- 立即发布和定时发布走同一数据模型。
- 内容变更后，旧排期必须提示重新确认或重新审批。
- 失败重试有完整 attempt 记录，不重复扣发布额度。

### 阶段五：审批协作

目标：团队发布前有清晰可审计的批准流程。

任务：

- [ ] 新增 `ApprovalRequest`、`ApprovalDecision`。
- [ ] 工作区设置增加默认审批策略。
- [ ] 项目可覆盖审批策略。
- [ ] 发起审批时冻结当前 `ProjectVersion`。
- [ ] 审批记录绑定项目、平台草稿、目标账号和排期。
- [ ] 审批通过后，若项目或平台草稿变更，审批失效。
- [ ] 发布前接入审批校验。
- [ ] 前端新增审批待办、审批详情、通过/拒绝入口。
- [ ] 审批事件写入 project activity、audit event、notification。

验收：

- 需要审批的工作区不能绕过审批发布。
- 审批记录能追溯到具体版本、审批人、时间和意见。
- 内容变更后旧审批不可继续用于发布。

### 阶段六：套餐、配额、用量

目标：SaaS 商业限制可执行、可展示、可审计。

任务：

- [ ] 新增 `Plan`、`WorkspaceSubscription`、`UsageEvent`、`UsageAggregate`、`QuotaOverride`。
- [ ] 建立计量指标：席位、平台账号、月发布数、月排期数、AI token/调用数、浏览器分钟、存储量。
- [ ] 接入成员、账号、媒体、AI、浏览器会话、发布、排期配额拦截点。
- [ ] 聚合当前账期 usage，提供工作区 billing API。
- [ ] 设置页展示套餐、周期用量、剩余额度和升级入口。
- [ ] 后台支持手动改套餐、加额度、停用工作区。
- [ ] 统一超额错误码和前端提示。
- [ ] 补齐计量幂等测试，确保重试不重复扣量。

验收：

- 超额不能绕过 API。
- 用量页面能解释每项额度被谁、何时、因什么动作消耗。
- usage event 与 aggregate 可对账。

### 阶段七：审计、通知与后台

目标：成员、账号、审批、发布、计费事件形成可检索、可提醒的 SaaS 运营视图。

任务：

- [ ] 新增 `AuditEvent`，封装统一记录函数。
- [ ] 将 workspace activity、project activity、publish event 关键事件同步写 audit event。
- [ ] 新增 `Notification`，支持站内未读、已读、归档。
- [ ] 审批待办、账号重登、发布失败、人工处理、配额告警生成通知。
- [ ] 前端新增通知中心和审计日志页面。
- [ ] 后台可搜索用户、工作区、平台账号和项目。
- [ ] 后台可查看审计活动，但不可查看敏感凭据。
- [ ] 后台可临时冻结发布能力。
- [ ] 后台操作全部写入管理审计日志。

验收：

- 工作区 owner/admin 可查关键操作审计。
- 用户能看到自己待处理的审批、重登、发布失败和人工处理任务。
- 客服/运营能定位工作区、账号、排期、配额问题。
- 通知状态变更不影响审计记录。

## 5. 优先级清单

| 优先级 | 任务 | 原因 |
| --- | --- | --- |
| P0 | 工作区上下文统一 | 所有 SaaS 能力都依赖工作区边界 |
| P0 | 权限点模型 | 避免角色判断散落，发布/审批/账号共享都要用 |
| P0 | 平台账号工作区化 | 多平台发布的团队协作核心 |
| P0 | 发布者权限 | owner-only 不适合团队发布 |
| P1 | 排期对象和发布日历 | 内容团队日常工作流核心 |
| P1 | 审批开关 | 团队发布质量控制 |
| P1 | 审计与通知 | 账号、审批、发布失败和人工处理都需要待办 |
| P1 | 套餐和配额模型 | 商业化和资源边界 |
| P1 | 用量事件 | 配额、账单、审计都依赖它 |
| P2 | 模板、品牌、素材库 | 提升创作效率，但依赖基础内容模型稳定 |
| P2 | 后台管理 | 客户管理和问题排查 |
| P2 | 账单状态联动 | 收费后再强化 |

## 6. 单次提交边界建议

推荐拆成以下独立实现提交：

1. 工作区上下文和权限 guard。
2. 工作区邀请。
3. 平台账号工作区迁移。
4. 平台账号授权。
5. 模板、品牌和素材库基础模型。
6. 排期发布基础模型。
7. 发布日历 API。
8. 审批 request/decision。
9. 用量 event/aggregate。
10. Billing settings API。
11. 审计事件。
12. 通知中心。
13. 后台工作区管理。

## 7. 不做内容

- 不设计服务拆分、队列、网关、Kubernetes、数据库扩容。
- 不按平台拆独立微服务。
- 不做复杂审批流引擎。
- 不做公开内容社区。
- 不做通用项目管理工具。
