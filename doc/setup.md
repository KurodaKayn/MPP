# 项目启动指南

## 1. 快速启动 (推荐)

使用 Docker Compose 启动所有服务：

```bash
# 进入 docker 目录
cd docker

# 启动所有服务
docker compose up -d

# 查看日志
docker compose logs -f
```

默认 Compose project name 为 `mpp`。

## 2. 容器 Dev 模式 (热重载)

如果你希望一键拉起开发模式容器，并让代码修改自动生效，在项目根目录执行：

```bash
docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml watch
```

Dev 模式的 Compose project name 为 `mpp-dev`。

这个模式会启动前端、后端、AI 服务和数据库：

- 前端使用 `pnpm dev`，源码变化会触发 Next.js 热更新。
- 后端使用 `air`，Go 源码变化会自动重新编译并重启 API。
- AI 服务使用 `uvicorn --reload`，Python 源码变化会自动重载。
- 依赖文件变化会触发对应服务重新构建，包括 `package.json`、`pnpm-lock.yaml`、`go.mod`、`go.sum`、`pyproject.toml`、`uv.lock`。

如果只想后台启动 dev 容器（源码热重载仍会生效，但依赖文件变化不会自动触发 Compose rebuild），可以执行：

```bash
docker compose -f docker/docker-compose.yml -f docker/docker-compose.dev.yml up -d
```

### 访问地址：

- **前端**: [http://localhost:3000](http://localhost:3000)
- **后端 API**: [http://localhost:8080/ping](http://localhost:8080/ping)
- **AI 服务**: [http://localhost:8000](http://localhost:8000)

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
- **AI 服务**: 在 `docker/.env` 中设置 `OPENAI_API_KEY`。
- **后端**: 数据库连接配置 `docker/.env`。
