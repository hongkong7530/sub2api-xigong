# 本地开发与构建

## 工具链

- Go **1.27.2**，以 `backend/go.mod` 为准。
- Node.js **20+**、pnpm **9**；Docker 构建使用 Node 24。
- PostgreSQL、Redis；集成测试通常需要 Docker/Testcontainers。
- Linux 发行目标建议 `CGO_ENABLED=0`，前端嵌入必须使用 `-tags embed`。

```bash
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend run typecheck
pnpm --dir frontend run build
cd backend
go build -tags embed -trimpath -o bin/sub2api ./cmd/server
```

Vite 的输出目录是 `backend/internal/web/dist`，不是通常的 `frontend/dist`。`pnpm run build` 依次运行 i18n 检查、vue-tsc 和 Vite；先构建前端，再构建后端。

## 开发服务

在自己的隔离数据库和 Redis 上运行后端；首次启动可使用安装向导或自己的配置，不连接演示站数据库。

```bash
# 终端 1：从 backend 目录运行后端。
go run ./cmd/server

# 终端 2：从仓库根目录运行 Vite。
pnpm --dir frontend dev
```

Vite 默认端口 3000、后端代理目标 `http://localhost:8080`；可用 `VITE_DEV_PORT` 和 `VITE_DEV_PROXY_TARGET` 覆盖。首次后端向导需要可用的前端资源时，请先完成 embed 构建，或按已有开发方式使用前后端分别启动。

## 代码生成

```bash
cd backend
go generate ./ent
go generate ./cmd/server
```

只在 schema/provider 等相关改动时执行；检查并提交生成结果。公开仓库保留上游 Go module 路径，禁止为“改仓库名”机械替换全部 import。

## 测试

```bash
pnpm --dir frontend run check:i18n
pnpm --dir frontend run typecheck
pnpm --dir frontend test:run

cd backend
go test -tags unit ./...
go test -tags integration ./...
go test ./internal/cluster
```

也可通过现有 Makefile 运行定向关键用例。全量测试存在记录在案的旧失败和环境依赖，详见 [已知限制](KNOWN_LIMITATIONS.md)。不要把命令可执行写成命令已通过，不把跳过用例算作验收。

## Linux 二进制

```bash
pnpm --dir frontend run build
cd backend
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags embed -trimpath \
  -ldflags="-s -w" -o bin/sub2api-linux-amd64 ./cmd/server
sha256sum bin/sub2api-linux-amd64
```

版本默认读取 `backend/cmd/server/VERSION`，可用现有 ldflags 注入版本、提交和日期。不含 `embed` 标签时只构建 API 后端，不应称为完整可视化发行包。

## 仓库结构

```text
backend/                 Go 服务、Ent schema/生成代码、迁移与资源
frontend/                Vue 3 + TypeScript + Vite
frontend/public/experience/ 自托管字体与壁纸
backend/internal/cluster/ 集群角色、准入、传输与恢复
backend/internal/web/    前端嵌入边界
deploy/                  容器与系统服务模板
scripts/                本公开版截图复现工具
tools/cluster/           集群管理工具；操作前必须阅读参数与权限
docs/                    用户与开发文档
```

## 公开快照原则

生产源码目录仍是私人开发基线。此仓库从当前文件导出，不携带原私人 `.git`；提交仅用于公开发行。不要把运行配置、内部部署记录、数据库、恢复槽、日志、Cookie 或真实 Key 加入仓库。更新公开版时以源码差异审核，不能直接整目录覆盖生产环境。
