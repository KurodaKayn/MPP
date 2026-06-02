# 项目启动指南

## 1. 快速启动 (推荐)

使用 Docker Compose 启动所有服务：

```bash
# 进入 docker 目录
cd docker

# 按网关/部署模式创建环境变量文件。已有 dev .env 时会中止，避免误用开发配置。
if [ -f .env ] && grep -q '^APP_ENV=development$' .env; then
  echo "docker/.env 当前是 dev 模式；请先备份或删除它，再复制 .env.deploy.example。" >&2
  exit 1
fi
cp -n .env.deploy.example .env

# 启动所有服务
docker compose up -d

# 查看日志
docker compose logs -f
```

默认 Compose project name 为 `mpp`。该模式会启动 Traefik 作为统一入口，宿主机默认只暴露网关 HTTP/HTTPS 端口：

- **Web 工作台**: [http://localhost](http://localhost)
- **HTTPS 入口**: [https://localhost](https://localhost)

`frontend`、`backend`、`ai-service`、`browser-worker`、PostgreSQL 和 Redis 都作为 Compose 内网服务运行，不再默认暴露到宿主机。如果宿主机的 `80` 或 `443` 端口已被占用，可以在 `docker/.env` 中设置 `TRAEFIK_HTTP_PORT` 或 `TRAEFIK_HTTPS_PORT`，例如 `TRAEFIK_HTTP_PORT=8088`。当前 HTTPS 入口使用 Traefik 默认 TLS 行为，生产环境还需要继续配置真实证书或证书解析器。

Traefik 入口默认启用 IP 级限流，参数来自 `TRAEFIK_RATE_LIMIT_AVERAGE`、`TRAEFIK_RATE_LIMIT_PERIOD` 和 `TRAEFIK_RATE_LIMIT_BURST`，并用 `TRAEFIK_RATE_LIMIT_REDIS_ENDPOINTS` 连接 Redis 保存分布式限流状态。backend 在 `/api/user/dashboard` 用户路由下默认启用 Redis 配额，覆盖通用用户/租户限额、单接口限额，以及 AI、发布任务、browser session 的分钟级和日级配额。应用侧配额只来自 backend 内置的 `rate_limits.yml` 矩阵；环境变量只保留 `APP_RATE_LIMIT_ENABLED` 和 `APP_RATE_LIMIT_KEY_PREFIX`。

生产或网关部署时，请把 `FRONTEND_BASE_URL` 和 `X_OAUTH2_REDIRECT_URL` 调整为真实公开入口，例如：

```env
FRONTEND_BASE_URL=https://your-domain.example
X_OAUTH2_REDIRECT_URL=https://your-domain.example/api/user/dashboard/settings/x/oauth2/callback
```

## 2. 容器 Dev 模式 (热重载)

如果你希望一键拉起开发模式容器，并让代码修改自动生效，在项目根目录执行：

```bash
if [ -f docker/.env ] && grep -q '^APP_ENV=production$' docker/.env; then
  echo "docker/.env 当前是 deploy 模式；请先备份或删除它，再复制 docker/.env.dev.example。" >&2
  exit 1
fi
cp -n docker/.env.dev.example docker/.env
docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml watch
```

Dev 模式的 Compose project name 为 `mpp-dev`。

这个模式会启动前端、后端、AI 服务、browser-worker、数据库和 Redis，并保留原有直连端口，避免日常 Docker 开发体验变化：

- 前端使用 `pnpm dev`，源码变化会触发 Next.js 热更新。
- 后端使用 `air`，Go 源码变化会自动重新编译并重启 API。
- AI 服务使用 `uvicorn --reload`，Python 源码变化会自动重载。
- 依赖文件变化会触发对应服务重新构建，包括 `package.json`、`pnpm-lock.yaml`、`go.mod`、`go.sum`、`pyproject.toml`、`uv.lock`。

如果只想后台启动 dev 容器（源码热重载仍会生效，但依赖文件变化不会自动触发 Compose rebuild），可以执行：

```bash
if [ -f docker/.env ] && grep -q '^APP_ENV=production$' docker/.env; then
  echo "docker/.env 当前是 deploy 模式；请先备份或删除它，再复制 docker/.env.dev.example。" >&2
  exit 1
fi
cp -n docker/.env.dev.example docker/.env
docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml up -d
```

### 访问地址：

- **前端**: [http://localhost:3000](http://localhost:3000)
- **后端 API**: [http://localhost:8080/ping](http://localhost:8080/ping)
- **AI 服务**: [http://localhost:8000](http://localhost:8000)
- **browser-worker**: [http://localhost:8081](http://localhost:8081)
- **PostgreSQL**: `localhost:5432`
- **Redis**: `localhost:6379`

如果希望在保持上述 dev 直连体验的同时测试 Traefik，可以单独启动 dev 网关探针：

```bash
script/docker/dev-traefik.sh up
```

这只会启动 `traefik` 服务，不会启动或重建 frontend/backend/AI/DB/Redis，也不会移除上面的直连端口。Traefik dev 入口默认是：

- **HTTP 网关**: [http://localhost:8088](http://localhost:8088)
- **HTTPS 网关**: [https://localhost:8443](https://localhost:8443)

常用命令：

```bash
script/docker/dev-traefik.sh logs
script/docker/dev-traefik.sh restart
script/docker/dev-traefik.sh stop
```

如果需要换端口，可以在命令前设置 `TRAEFIK_HTTP_PORT` 或 `TRAEFIK_HTTPS_PORT`。

---

## 3. 本地开发启动

如果你需要进行调试或热更新开发，可以分别启动各服务。

### 前置条件

- 已安装 `pnpm`, `go`, `uv` (Python 管理工具)。
- 本地已有运行中的 PostgreSQL (17) 和 Redis。

### 前端 (Next.js)

```bash
cd frontend
pnpm install
pnpm dev
```

### 后端 (Go)

```bash
cd backend
go run cmd/api/main.go
```

### AI 服务 (Python)

```bash
cd ai-service
uv run uvicorn main:app --reload
```

---

## 4. 环境变量配置

- **统一管理**: 所有的环境变量现在都在 `docker/.env` 中进行统一管理。
- **Dev 模板**: 使用 `docker/.env.dev.example`，默认公开地址是 `http://127.0.0.1:3000`，适合 Docker dev 直连端口。
- **Deploy 模板**: 使用 `docker/.env.deploy.example`，默认公开地址是 `https://your-domain.example`，适合 Traefik 网关部署。
- **AI 服务**: 在 `docker/.env` 中设置 `LLM_PROVIDER_KEY`。
- **后端**: 数据库连接配置 `docker/.env`。
