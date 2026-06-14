# MPP 高并发与分布式架构演进计划书

## 1. 背景

MPP 当前已经具备多服务雏形：

- `frontend`: Next.js 工作台与 API 代理入口。
- `backend`: Go API、用户态业务、发布编排、账号管理、任务协调。
- `ai-service`: FastAPI AI 编辑服务，负责提示词、模型调用和流式响应。
- `browser-worker`: 远程浏览器会话、Chromium 容器创建、销毁和登录态捕获。
- `PostgreSQL`: 持久化业务数据。
- `Redis`: 发布队列、分布式锁、OAuth 状态和临时会话状态。

这说明项目不需要从零开始引入“微服务架构”。更合适的方向是：保留 Go 后端作为业务核心，把高风险、高资源消耗、异步执行、外部依赖强的部分逐步演进成独立服务或 worker。

## 2. 目标

本计划的目标是提升 MPP 的并发承载、稳定性、安全性、可观测性、故障恢复能力和资源治理能力。

每项设计都需要能落到 MPP 的实际业务场景中，避免引入当前阶段难以维护的复杂基础设施。

## 3. 设计原则

- 先用好现有 PostgreSQL、Redis、Docker Compose 和服务边界，再考虑引入更重的基础设施。
- 先解决入口安全、队列可靠性、幂等、限流、监控和故障隔离，再考虑 Kubernetes、Service Mesh 和更重的数据基础设施。
- 按运行特征拆分服务，而不是按业务名词硬拆微服务。
- 所有异步任务都要有幂等键、状态机、重试策略和可追踪日志。
- 所有外部平台调用都要可降级、可重试、可限流、可审计。

数据层专项扩展的进度、Checklist 和 Citus 目标态，统一在 [MPP 数据库读写分离与水平分表渐进式方案](./database-optimization.md) 中维护。

## 4. 推荐总体架构

![MPP 高并发与分布式架构演进图](../assets/plan/high-concurrency-distributed-architecture.svg)

## 5. 架构设计候选表

评分说明：

- 生产价值：1 低，5 高。
- 成本：1 低，5 高。
- 推荐优先级：P0 立即做，P1 优先做，P2 增长后做，P3 暂不建议。
- 完成状态基于当前代码盘点：`完成` 表示主要落点已有明确实现，`进行中` 表示已有部分实现但仍缺关键能力，`未开始` 表示未发现明确实现，`暂缓` 表示计划中明确不建议当前优先做。

| 序号 | 架构设计                     | 解决的问题                                | 项目落点                                                      | 生产价值 | 成本 | 优先级 | 完成状态 | 建议                                                                                                                                                                                                                          |
| ---- | ---------------------------- | ----------------------------------------- | ------------------------------------------------------------- | -------- | ---- | ------ | -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | Traefik API Gateway          | 统一入口、HTTPS、路由、内部服务隐藏       | 应用入口只暴露 `80/443`，隐藏 backend、AI、worker、DB、Redis  | 5        | 2    | P0     | 进行中   | 应用入口和服务隐藏已落地；Let’s Encrypt 自动证书与手动证书 Compose override 已落地，生产域名/证书需按环境配置；Prometheus/Grafana/Loki/Alloy 观测端口仍按环境变量显式暴露                                                     |
| 2    | 网关与应用双层限流           | 防止爬虫、恶意请求、AI 滥用、发布任务刷爆 | Traefik 做 IP 级限流，backend 做用户/工作区/接口级配额        | 5        | 2    | P0     | 完成     | 生产保护关键能力                                                                                                                                                                                                              |
| 3    | 可观测性基线                 | 出问题能定位，能展示系统真实运行状态      | Prometheus 指标、Grafana 面板、Loki 日志、Trace ID            | 5        | 3    | P0     | 完成     | backend、publish-worker、ai-service、browser-worker 已有 HTTP 指标和结构化请求日志；这是基线，不是完整分布式 tracing                                                                                                          |
| 4    | 健康检查与优雅关闭           | 支持滚动重启，避免请求中断                | frontend/backend/ai/browser-worker 增加 health/readiness      | 5        | 2    | P0     | 完成     | 成本低，生产必备                                                                                                                                                                                                              |
| 5    | API 服务无状态化与横向扩容   | 支撑更多并发请求                          | backend 不保存本地会话，扩多副本，共享 Redis/Postgres         | 5        | 2    | P1     | 完成     | API 基本无状态并支持 `api/worker/all` 角色，生产 Compose 默认多 backend 副本，数据层专项演进见 [数据库专项方案](./database-optimization.md)                                                                                   |
| 6    | Redis 队列升级为可靠任务模型 | 发布任务异步化、可重试、可恢复            | 用 Redis Streams 或 Asynq 管理 publish jobs                   | 5        | 3    | P1     | 完成     | 已用 Asynq 替换 Redis List；publish job 具备 ack/retry/worker crash recovery/archive 语义，任务 payload 只保存 durable IDs，不携带 browser session 地址或 token                                                               |
| 7    | 幂等键与发布状态机           | 防止重复点击、重复消费、重复发布          | publish 请求带 idempotency key，publication 状态机严格流转    | 5        | 3    | P1     | 完成     | 发布请求已支持 idempotency key 复用；publication 状态已对齐 `draft`、`syncing`、`queued`、`publishing`、`succeeded`、`failed`、`cancelled`，旧状态名保留为兼容别名                                                               |
| 8    | Outbox Pattern               | 数据库更新与事件投递一致性                | publication 状态更新后写 outbox，由 worker 投递任务           | 4        | 4    | P1     | 完成     | 发布排队链路已落地事务 Outbox：`EnqueuePublishProject` 同事务写 `outbox_events`，提交后即时 dispatch，worker 定时 flush failed/stale processing 并重试；当前仅覆盖 `publish.job_requested`，通用多消费者 CDC 仍按数据库专项演进 |
| 9    | 分布式锁强化                 | 避免同一 publication 被并发发布           | Redis lock 加 owner、TTL、续约、释放校验                      | 5        | 2    | P1     | 完成     | 项目已经有 Redis，成本可控                                                                                                                                                                                                    |
| 10   | 外部调用熔断、重试、退避     | 防止第三方平台或 LLM 故障拖垮系统         | AI、微信、知乎、X、抖音调用统一 retry/backoff/circuit breaker | 5        | 3    | P1     | 完成     | backend 已新增统一 resilience 层，HTTP retry 默认仅覆盖安全方法，非幂等 POST 只做 timeout/circuit breaker；发布操作层按平台维度熔断但不重试完整发布；ai-service LLM 客户端已配置 timeout、max retries 和 stream chunk timeout |
| 11   | Browser Worker 资源池与配额  | 控制 Chromium 容器数量，避免宿主机爆掉    | 每用户/工作区限制并发 browser session，全局 worker pool       | 5        | 3    | P1     | 完成     | 已有用户+平台活跃 session 锁、用户/工作区并发配额、全局 worker pool 和容器 CPU/内存限制                                                                                                                                       |
| 12   | WebSocket/SSE 长连接治理     | 处理 AI stream 和远程浏览器 stream        | 网关 timeout、连接数限制、stream token、断线恢复              | 4        | 3    | P1     | 完成     | AI stream 和远程浏览器长连接已接入连接数限制；远程浏览器 stream 统一校验 token，HTTP gateway timeout 返回 504，token 有效期内支持断线重连                                                                                     |
| 13   | 对象存储与签名 URL           | 图片和媒体不压在应用容器与数据库上        | S3/R2/OSS 存储媒体，backend 生成 signed URL                   | 5        | 3    | P2     | 完成     | R2 对象存储配置、上传/下载签名 URL、媒体对象引用和发布前临时签名 URL 已落地；后续重点转向 CDN、归档和生产环境配置                                                                                                             |
| 14   | CDN 与静态资源缓存           | 降低前端资源和图片访问压力                | Next 静态资源、媒体文件走 CDN                                 | 4        | 2    | P2     | 未开始   | 公开上线后逐步做                                                                                                                                                                                                              |
| 15   | Temporal 工作流编排          | 复杂长流程、可恢复任务、Saga              | 多平台发布、浏览器自动化、重试补偿                            | 4        | 5    | P2     | 未开始   | 等发布流程复杂后再引入                                                                                                                                                                                                        |
| 16   | Kubernetes                   | 服务调度、弹性伸缩、滚动发布              | 从 Compose 迁移到 K8s + Ingress                               | 3        | 5    | P3     | 进行中   | 已有 Kustomize 包、环境 overlay、镜像 pinning、NetworkPolicy、观测资源和运维 runbook；仍应作为生产复杂度增长后的可选路径推进                                                                                                  |
| 17   | Service Mesh                 | 服务间治理、mTLS、流量控制                | Istio/Linkerd 管理服务间调用                                  | 1        | 5    | P3     | 暂缓     | 当前规模不值得                                                                                                                                                                                                                |
| 18   | 多活与异地容灾               | 区域级故障恢复                            | 多区域部署、数据复制、故障切换                                | 1        | 5    | P3     | 暂缓     | 业务成熟后再考虑                                                                                                                                                                                                              |

## 6. 推荐落地路线

### 阶段一：生产入口与稳定性基线

目标：让项目具备生产入口、基本安全边界和排障能力。

交付项：

- [x] 引入 Traefik 作为统一入口，只暴露应用入口 `80/443`。（已提供 Let’s Encrypt 自动证书和手动证书两种 Compose override；生产域名/证书需按环境配置。）
- [x] backend、ai-service、browser-worker、PostgreSQL、Redis 全部改为内网服务。
- [x] 增加基础 health/readiness endpoint。（已完成：frontend 已有 `/api/health`、`/api/ready`，backend、ai-service、browser-worker 已有 `/health`、`/ready`；Dockerfile 和生产 Compose 已接入 readiness healthcheck，backend 与 browser-worker 已支持信号驱动的优雅关闭。）
- [x] 增加请求日志中的 request ID / trace ID。
- [x] 增加基础限流：IP 级、用户级、AI 接口级、browser session 级。
- [x] 整理生产环境变量和 secret 管理规范。

### 阶段二：异步发布与幂等治理

目标：让发布链路能够承受并发请求、重复点击、worker 重启和第三方平台失败。

交付项：

- [x] 将发布任务模型升级为可靠队列，优先考虑 Redis Streams 或 Asynq。（已完成：使用 Asynq + Redis 管理 publish jobs，支持 ack、失败 retry、worker crash recovery 和 archive。）
- [x] 发布请求引入 idempotency key。
- [x] publication 状态机明确化：`draft`、`syncing`、`queued`、`publishing`、`succeeded`、`failed`、`cancelled`。（已完成：旧 `pending`、`adapted`、`published`、`disabled` 状态名保留为兼容别名。）
- [x] 分布式锁增加 owner、TTL、续约和释放校验。
- [x] 外部平台调用增加 retry、backoff、timeout 和 circuit breaker。（已完成：backend 统一 resilience 层覆盖 AI service、微信、X、browser-worker、媒体下载 HTTP 调用；HTTP retry 默认仅用于安全方法，非幂等 POST 不自动重放；发布操作层按平台维度熔断但不重试完整发布；ai-service LLM 客户端支持 timeout、max retries 和 stream chunk timeout。）
- [x] 每次任务执行写入 publish event，方便审计和排查。

### 阶段三：资源隔离与横向扩容

目标：让高资源模块和普通 API 模块可以独立扩容。

交付项：

- [x] 将 backend 拆成 `backend` API 服务和 `publish-worker` 两个运行进程。
- [x] browser-worker 增加全局资源池和用户级并发配额。（已完成：backend 使用 Redis 维护用户/工作区并发配额，browser-worker 在启动 Chromium 容器前预留全局池槽位，容器仍保留 CPU/内存限制。）
- [x] AI 请求增加用户级并发限制和 token/成本统计。（已完成：普通和流式 AI 请求统一接入 `STREAM_GATE_*`/`AI_STREAM_*` 并发租约；非流式 AI 响应透传 provider token usage 和按 `LLM_INPUT_COST_PER_1K_TOKENS`、`LLM_OUTPUT_COST_PER_1K_TOKENS` 估算的成本，ai-service 暴露 `mpp_ai_tokens_total`、`mpp_ai_cost_total` 指标。流式 AI 仍以连接并发治理为主，不返回逐请求 token 明细。）
- [x] backend-api 支持多副本运行。（已完成：API 已基本无状态，生产 Compose 默认 `BACKEND_API_REPLICAS=2`，dev override 固定单副本避免端口冲突。）
- [x] 长连接接口统一设置 gateway timeout、连接数限制和 token 校验。（已完成：远程浏览器 stream 入口统一校验 session stream token；WebSocket/websockify 长连接接入 `BROWSER_STREAM_*` 连接数限制；HTTP 反代上游首包由 `BROWSER_STREAM_GATEWAY_TIMEOUT` 控制并在超时时返回 504；token 有效期内客户端可断线重连。）

数据层专项扩展不在本阶段展开，统一按数据库专项方案推进。

### 阶段四：媒体与工作流扩展

目标：支撑更多用户、更多媒体、更多平台和更复杂的发布工作流。

交付项：

- [x] 媒体文件迁移到对象存储，使用 signed URL。
- [ ] 静态资源和媒体接入 CDN。
- [ ] 根据任务复杂度评估 Temporal 工作流。
- [x] 根据部署复杂度评估 Kubernetes。（已形成可选 Kubernetes 生产部署路径、验证脚本和运维 runbook；实际启用仍按团队部署复杂度决定。）

## 7. P0/P1 优先清单

当前最值得做的不是大而全的微服务，而是下面这些高价值改造：

| 完成 | 优先级 | 改造项                | 原因                                                                                                                                                                                                      |
| ---- | ------ | --------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [x]  | P0     | Traefik 统一入口      | 生产部署安全边界和内部服务隐藏已落地；Let’s Encrypt 自动证书和手动证书可通过 Compose override 选择                                                                                                        |
| [x]  | P0     | 限流与配额            | 保护 AI、browser session 和发布接口                                                                                                                                                                       |
| [x]  | P0     | 可观测性基线          | 出问题能定位，支撑稳定迭代                                                                                                                                                                                |
| [x]  | P0     | health/readiness      | 支持生产重启、监控和负载均衡；当前已补齐 frontend/backend/ai-service/browser-worker 的基础 health/readiness                                                                                               |
| [x]  | P1     | 发布幂等              | 防止重复发布和并发写冲突；当前已支持 idempotency key、幂等响应复用、发布锁和状态机校验                                                                                                                     |
| [x]  | P1     | 可靠队列              | 已用 Asynq + Redis 替代 Redis List，提供 ack、retry、worker crash recovery 和 archive                                                                                                                      |
| [x]  | P1     | 分布式锁强化          | 保证同一 publication 不被多 worker 并发处理                                                                                                                                                               |
| [x]  | P1     | 外部调用熔断与重试    | 统一 resilience 层已覆盖 AI service、微信、X、browser-worker、媒体下载 HTTP 调用；HTTP retry 默认仅用于安全方法，发布完整操作不重试；ai-service LLM 客户端已配置 timeout/max retries/stream chunk timeout |
| [x]  | P1     | browser-worker 资源池 | 已有用户/工作区并发配额、同一用户/平台活跃 session 锁和全局 worker pool，控制 Chromium 容器成本和风险                                                                                                     |

## 8. 暂不建议优先引入的技术

| 技术               | 暂缓原因                                    | 什么时候再考虑                                         |
| ------------------ | ------------------------------------------- | ------------------------------------------------------ |
| Kubernetes         | 已有可选生产部署路径，但默认引入仍会增加运维复杂度 | 服务数量、环境数量、发布频率和副本规模明显增长后       |
| Service Mesh       | 当前服务间调用链不复杂                      | 多团队、多语言、多集群、mTLS 和灰度流量治理成为刚需后  |
| 每个平台一个微服务 | 会让发布状态、账号、权限和事务变复杂        | 平台适配团队独立、单个平台流量巨大或故障频繁影响全局后 |

## 9. 结论

综合成本和价值，MPP 最适合采用“渐进式分布式架构”：

- 当前保留 Go backend 作为业务核心，不急着拆成多个业务微服务。
- Traefik、限流、可观测性、健康检查、幂等、可靠队列、分布式锁、publish-worker、browser-worker 资源池和对象存储已经形成主要基线。
- 下一阶段优先推进 CDN、缓存精细失效、读写分离收尾、事件保留期和归档；Outbox 后续重点是从发布任务扩展到更多业务事件，并在多消费者需求明确后再评估 CDC。
- Temporal、Service Mesh 和多区域容灾继续按触发条件评估；Kubernetes 保留为已经具备基础包和 runbook 的可选生产路径；数据层专项扩展按 [数据库专项方案](./database-optimization.md) 推进。

这条路线能在控制复杂度的同时提升项目的并发承载、稳定性和业务增长承载能力。
