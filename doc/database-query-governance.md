# 数据库索引、分页与慢查询治理

MPP 的高频 dashboard 查询必须同时满足三个约束：

- 列表接口只返回当前页，禁止无界扫描或拉取大字段。
- 高频过滤条件必须被组合索引覆盖，尤其是用户作用域、状态、平台和创建时间。
- 慢查询必须可观测、可复盘，并在上线前后做查询计划审计。

## 运行时观测

backend 会通过 GORM callback 记录数据库查询指标：

- `mpp_db_queries_total{service,operation,table,status}`
- `mpp_db_query_duration_seconds_bucket{service,operation,table,status}`
- `mpp_db_slow_queries_total{service,operation,table,status}`

慢查询阈值由 `DB_SLOW_QUERY_THRESHOLD` 控制，默认示例值为 `250ms`。设置为 `0` 可以关闭慢查询计数和日志，但仍保留总查询量和耗时直方图。

超过阈值的查询会写入结构化 JSON 日志，字段包括：

- `trace_id`
- `operation`
- `table`
- `query_hash`
- `duration_ms`
- `rows_affected`
- `sql`
- `error`

日志只记录参数化 SQL 和 hash，不记录查询变量值。

## 查询计划审计

dashboard 相关查询计划使用脚本审计：

```bash
script/db/audit_dashboard_query_plans.sh \
  -v user_id=00000000-0000-0000-0000-000000000001 \
  -v project_id=00000000-0000-0000-0000-000000000101 \
  -v platform=douyin
```

脚本会对项目计数、publication 状态计数、项目列表、平台过滤、publication preload、账号查询和活跃 browser session 查询执行 `EXPLAIN (ANALYZE, BUFFERS)`。

审计环境应尽量使用生产同量级数据或脱敏快照。重点检查：

- 是否出现高成本 `Seq Scan`。
- `Rows Removed by Filter` 是否远高于返回行数。
- `Sort Method` 是否落盘。
- `shared read blocks` 是否异常升高。
- p95 查询耗时是否接近或超过慢查询阈值。

## 持续治理流程

新增或改动 dashboard 查询时：

1. 确认接口分页，且列表 DTO 不返回 `source_content`、`adapted_content`、cookies、credentials 等大字段或敏感字段。
2. 确认 where/order/join 条件能匹配已有组合索引；不确定时先跑查询计划脚本。
3. 对新增高频查询补充审计 SQL，方便后续回归。
4. 上线后观察 Grafana 的 DB p95 和慢查询率；如果出现慢查询日志，用 `query_hash` 聚合定位。
5. 优先通过查询收敛、字段裁剪、索引调整或读模型治理；不要在请求路径里临时放宽连接池来掩盖慢查询。

当前重点索引：

- `projects(user_id, status, created_at)`
- `projects(status, created_at)`
- `project_platform_publications(project_id, platform)`
- `project_platform_publications(platform, status)`
- `platform_accounts(user_id, platform)`
- `platform_accounts(platform, status)`
- `remote_browser_sessions(user_id, platform, status)`
- `ux_remote_browser_sessions_active_user_platform` active-session partial unique fallback
