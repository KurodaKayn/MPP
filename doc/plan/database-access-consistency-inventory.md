# 数据库访问一致性与读写路由盘点

最近更新：2026-06-06

本文是 `doc/plan/database-optimization.md` 阶段 0 的执行产物，用来回答三个问题：

- 现在已经盘点了哪些数据库访问路径。
- 哪些路径未来可以走 reader、读模型或归档查询。
- 哪些路径即使引入 read replica 或 Citus，也必须继续走 writer。

## 1. 当前状态

已完成：

- 已按 dashboard、publish、collab-service 三条主路径标注一致性等级。
- 已标出当前实际路由：目前所有 PostgreSQL 访问仍走单一 writer 连接。
- 已给出阶段 2/3/5 的后续落点：读模型、reader/writer router、Citus workspace 边界。

未完成：

- 还没有引入 read replica。
- 还没有 `DB_WRITER_*`、`DB_READER_*` 双连接配置。
- 还没有代码级 `StrongRead`、`ReadYourWrite`、`EventualRead` 标注。
- 还没有自动验证某个查询实际走 writer 或 reader 的测试/指标。

## 2. 一致性等级定义

| 等级 | 含义 | 默认路由 | 典型场景 |
| ---- | ---- | -------- | -------- |
| `WriterOnly` | 事务、写入、锁、状态机推进、凭据读取、授权后立即写 | writer | 创建项目、发布状态 CAS 更新、协作 flush |
| `ReadYourWrite` | 用户刚写入后马上读取，必须避免副本延迟造成状态倒退 | writer 或 sticky writer | 保存后刷新详情、发布后轮询、协作文档加载 |
| `EventualRead` | 可以接受秒级延迟，优先从读模型、缓存或 reader 获取 | read model / reader | dashboard count、项目列表、历史活动 |
| `AnalyticsRead` | 可离线或分钟级延迟，默认不占热库主路径 | archive / offline | 超保留期审计、跨租户管理统计 |

当前阶段的保守原则：

- 只要一次请求内有写入，事务内读写都留在 writer。
- 鉴权读如果会决定后续写入，先按 `WriterOnly` 处理。
- publish 和 collab 的状态推进不进入 read replica。
- Citus 是最终水平扩展形态；应用侧 router 只表达一致性和 workspace 上下文，不做长期自研分片路由。

## 3. 执行 Checklist

- [x] 盘点 dashboard stats/project list/project detail 的数据库访问一致性。
- [x] 盘点 publish enqueue/worker/idempotency/status event 的数据库访问一致性。
- [x] 盘点 collab-service document load/initialize/flush/store/source sync 的数据库访问一致性。
- [x] 标注每类路径的当前路由和未来目标路由。
- [ ] 在 Go backend 增加 `StrongRead`、`ReadYourWrite`、`EventualRead` 代码级路由枚举。
- [ ] 在 collab-service 增加 writer/reader pool wrapper，并只在 lag 门禁通过时放行 reader load。
- [ ] 增加 writer/reader 路由指标，证明查询实际落到预期连接池。
- [ ] 引入 read replica 后做一次全链路演练。

## 4. Dashboard 与项目读取路径

| 路径 | 代表代码 | 主要表 | 一致性等级 | 当前路由 | 后续目标 | 备注 |
| ---- | -------- | ------ | ---------- | -------- | -------- | ---- |
| 全局 dashboard 用户数 | `backend/internal/services/stats/overview.go` `GetStats` | `users` | `EventualRead` | writer | read model 或 reader | 管理视图可接受延迟；不要在高频首页直接实时 Count |
| 用户/工作区项目总数 | `GetStats`、`GetWorkspaceStats` | `projects`、`workspace_members`、`workspaces` | `EventualRead`，鉴权部分为 `WriterOnly` | writer | `workspace_dashboard_stats` 读模型 | 先完成读模型，再考虑 reader |
| 发布成功/失败统计 | `GetStats`、`GetWorkspaceStats` | `project_platform_publications`、`projects` | `EventualRead` | writer | `workspace_dashboard_stats` 读模型 | 发布完成后 UI 短轮询需要 sticky writer |
| 项目列表 Count + page | `backend/internal/services/project/lifecycle.go` `ListProjects` | `projects`、`project_platform_publications` | `EventualRead` | writer | `project_list_summaries` 或 reader | 写后立即返回列表时使用 sticky writer |
| 项目详情读取 | `GetProject` | `projects`、`project_platform_publications` | `ReadYourWrite` | writer | sticky writer 后 reader | 保存、同步、发布后刷新详情必须避免旧副本 |
| 项目访问范围过滤 | `ScopeAccessibleProjects`、`projectAccessForUser` | `project_collaborators`、`workspace_members`、`workspaces` | `WriterOnly` 用于写前鉴权；普通展示可 `ReadYourWrite` | writer | 权限版本缓存 + sticky writer | 成员变更后不能让旧权限在 reader 上继续授权写入 |
| 项目发布配置列表 | `backend/internal/services/project/publications.go` | `project_platform_publications` | `ReadYourWrite` | writer | sticky writer 后 reader | 平台开关保存后立即展示必须新鲜 |

Dashboard 后续落地顺序：

1. 阶段 2 先建 `workspace_dashboard_stats` 和 `project_list_summaries`。
2. 阶段 2 给 stats/list 增加 Redis TTL 缓存，缓存键包含 `workspace_id` 和权限版本。
3. 阶段 3 再把 `EventualRead` 路径切到 reader。
4. 阶段 5 进入 Citus 前，所有项目域读模型都必须能以 `workspace_id` 定位。

## 5. Publish 路径

| 路径 | 代表代码 | 主要表/组件 | 一致性等级 | 当前路由 | 后续目标 | 备注 |
| ---- | -------- | ----------- | ---------- | -------- | -------- | ---- |
| 发布请求幂等查重 | `findIdempotentPublishResponse` | `publish_events` | `WriterOnly` | writer | writer | 不能因副本延迟重复入队 |
| 发布 Redis 锁 | `RedisPublishQueue.AcquireLock` | Redis | 不走 PostgreSQL | Redis | Redis | 锁本身不进入 DB router，但持久状态必须 writer |
| 准备发布任务 | `preparePublishJob`、`markPublicationQueued` | `projects`、`project_platform_publications`、`publish_events` | `WriterOnly` | writer | writer | 包含状态机推进和事件写入 |
| worker 读取发布记录 | `PublishProject` | `projects`、`project_platform_publications` | `WriterOnly` | writer | writer | 读取后会立即推进状态，必须与后续写同源 |
| 平台账号/浏览器 cookie 读取 | `ApplySavedCredentialsToPublication`、`CookieStore.Load` | `platform_accounts`、cookie store 表 | `WriterOnly` | writer | writer | 凭据读取直接影响外部发布副作用 |
| 发布中/成功/失败状态更新 | `markPublicationPublishing`、`PublishProject` updates | `project_platform_publications` | `WriterOnly` | writer | writer | CAS 条件更新不能走 reader |
| 发布事件和项目活动写入 | `recordPublishEvent`、`recordProjectPublishActivity` | `publish_events`、`project_activities` | `WriterOnly` 写；历史查询可 `EventualRead` | writer | writer 写 + reader/归档读 | 事件表后续按时间分区和归档 |
| 发布后 UI 轮询 | publish API response + project detail/list | `project_platform_publications` | `ReadYourWrite` | writer | sticky writer，然后 reader | sticky 窗口过后可转读模型或 reader |

Publish 后续落地顺序：

1. 保持发布状态机、幂等查重、凭据读取全部 writer。
2. 只把发布历史、活动流、dashboard 成功失败统计纳入 `EventualRead`。
3. 阶段 3 引入 reader 后，发布完成后的短窗口 sticky writer 默认 5-10 秒，并受 replica lag 门禁约束。
4. 阶段 5 为 `PublishJob` payload 补 `workspace_id`，Citus 下 worker 不再只靠 `project_id` 定位。

## 6. Collab-Service 路径

| 路径 | 代表代码 | 主要表 | 一致性等级 | 当前路由 | 后续目标 | 备注 |
| ---- | -------- | ------ | ---------- | -------- | -------- | ---- |
| 文档加载 | `collab-service/src/persistence/document-persistence.ts` `load` | `collab_document_states`、`collab_document_update_batches` | `ReadYourWrite` | writer | lag 门禁通过后可 reader | 不能让用户加载缺少最近 batch 的旧状态 |
| 初始化项目文档 | `initializeProjectDocument` | `projects`、`collab_documents`、`collab_document_states` | `WriterOnly` | writer | writer | 首次 materialize 会写 state |
| 同步协作文档回项目正文 | `syncProjectSourceContent` | `projects`、collab 表 | `WriterOnly` | writer | writer | 包含 flush、load 和项目正文更新 |
| 追加更新入内存队列 | `appendUpdate` | 内存 | 不走 PostgreSQL | service memory | service memory | 只有 flush 时进入 PostgreSQL |
| flush update batch | `flushDocumentOnce`、`lockDocumentAndReadSeq` | `collab_documents`、`collab_document_update_batches` | `WriterOnly` | writer | writer | `SELECT ... FOR UPDATE` 锁序列并插入 batch |
| store/compaction | `store` | `collab_document_states`、`collab_document_update_batches` | `WriterOnly` | writer | writer | compaction 后删除旧 batch |
| 删除已压缩 batch | `pruneCompactedBatches` | `collab_document_update_batches` | `WriterOnly` maintenance | writer | writer / archive worker | 阶段 4 再进入分区与保留期治理 |
| 健康检查 | `ping` | PostgreSQL | `EventualRead` | writer | writer 或 reader | 需分别暴露 writer/readiness 和 reader/readiness |

Collab 后续落地顺序：

1. 阶段 1/4 先处理 update batch 保留期、hash 分区和 compaction 观测。
2. 阶段 3 只有在 reader lag 低于阈值并能证明最新 flush 已 replay 时，`load` 才允许 reader。
3. 阶段 5 补齐 collab 表与 project/workspace 的稳定映射，Citus 下优先让同一 workspace 的 project 和 collab 元数据 colocate。

## 7. 后续 Router 技术栈落点

| 问题 | 推荐技术栈/实现 | 采用时机 |
| ---- | --------------- | -------- |
| Go backend 显式路由 | 自建薄 `DBRouter` + 可选 GORM DBResolver 作为底层连接组 | 阶段 3 |
| Node collab-service 显式路由 | `pg.Pool` writer/reader wrapper，默认 writer，按方法白名单放行 reader | 阶段 3 |
| sticky writer | Redis 短 TTL 标记或请求上下文标记，key 包含 `user_id`/`workspace_id` | 阶段 3 |
| replica lag 门禁 | PostgreSQL exporter + `pg_stat_replication`/replay lag 指标 | 阶段 3 |
| 读模型更新 | PostgreSQL outbox + Asynq dispatcher，后期 Debezium + Redpanda/Kafka | 阶段 2 起步，事件消费者增多后升级 |
| 水平扩展 | Citus distributed PostgreSQL，`workspace_id` 作为 distribution column | 阶段 5/6 |

Router 接口应表达业务语义，而不是隐藏分片实现：

```text
Writer(ctx)
Reader(ctx, StrongRead | ReadYourWrite | EventualRead | AnalyticsRead)
ForWorkspace(ctx, workspaceID)
```

`ForWorkspace` 的目标是让业务代码显式携带租户边界。进入 Citus 后，真正的数据分布由 Citus coordinator/worker 和 colocated table group 承担，不在应用里维护长期 shard map。

## 8. 明确不做

- 不用透明 SQL 代理自动猜读写路由。
- 不把 publish 状态机、幂等查重、平台凭据读取放到 read replica。
- 不在 collab flush/store/source sync 上尝试 reader。
- 不在 `workspace_id`、读模型、分区和迁移演练完成前切 Citus 生产流量。
- 不用应用层自研水平分片替代 Citus。
