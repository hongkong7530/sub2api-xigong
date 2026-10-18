# 部署、升级与备份

> 这里部署的是**西宫定制版源码**。`deploy/` 中部分保留的上游脚本和 Compose 文件默认下载官方二进制/镜像，不含本版定制；首次部署请使用下方 `docker-compose.xigong.yml`，不要混用。

## 1. 前置条件

- 支持 Docker Compose v2 的 Linux 主机、可用磁盘和内存；容量按真实负载评估，不承诺固定机器配置的吞吐。
- 构建阶段需访问 npm、Go modules 和容器镜像源；Dockerfile 使用 Node 24、Go 1.27.2、PostgreSQL 18 客户端，数据库与 Redis 镜像版本以 Compose 为准。
- 生产域名与 HTTPS 反向代理。默认集群关闭，建议先验证单实例。

## 2. 从源码构建

```bash
git clone https://github.com/hongkong7530/sub2api-xigong.git
cd sub2api-xigong
cp deploy/.env.example deploy/.env
chmod 600 deploy/.env
```

编辑 `deploy/.env`：至少设置 `POSTGRES_PASSWORD`，建议明确设置 `ADMIN_EMAIL`、`ADMIN_PASSWORD`、固定 `JWT_SECRET` 和固定 `TOTP_ENCRYPTION_KEY`。每个密钥独立生成，例如 `openssl rand -hex 32`；不要把生成结果提交到 Git。

新部署建议：

```dotenv
BIND_HOST=127.0.0.1
SERVER_PORT=8080
# 下列值不要复制占位文本，必须替换成自己的独立随机值。
POSTGRES_PASSWORD=REPLACE_WITH_A_UNIQUE_RANDOM_PASSWORD
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=REPLACE_WITH_A_DIFFERENT_RANDOM_PASSWORD
JWT_SECRET=REPLACE_WITH_RANDOM_64_HEX_CHARACTERS
TOTP_ENCRYPTION_KEY=REPLACE_WITH_DIFFERENT_RANDOM_64_HEX_CHARACTERS
# 从小规模连接预算开始，根据监控调整。
DATABASE_MAX_OPEN_CONNS=50
DATABASE_MAX_IDLE_CONNS=10
REDIS_POOL_SIZE=128
REDIS_MIN_IDLE_CONNS=10
SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=false
SECURITY_URL_ALLOWLIST_ALLOW_PRIVATE_HOSTS=false
```

首次创建空 PostgreSQL 卷时，Compose 挂载的 `deploy/initdb/001-schema-owner.sql` 会在应用迁移之前创建非登录属主 `sub2api_owner`，并赋予初始化账号所需成员资格及默认对象权限。这是历史迁移 265/271 的必要前置，不修改任何既有 SQL 校验和。

**已有卷或外部数据库不会自动重跑 initdb 脚本。** 请先备份，以数据库管理员/属主身份对目标数据库执行并审查该脚本后再运行应用；不要删除现有卷来绕过角色错误。集群部署还需独立节点授权。

数据库同容器网络内的连接由 Compose 管理；不要对外开放 PostgreSQL/Redis。更多配置请参考 `deploy/.env.example` 和 `deploy/config.example.yaml`，不要照搬演示站拓扑或容量。

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.xigong.yml up -d --build
docker compose --env-file deploy/.env -f deploy/docker-compose.xigong.yml ps
curl --fail http://127.0.0.1:8080/health
```

浏览器访问 `http://127.0.0.1:8080`，远程可使用 SSH 隧道。首次自动初始化使用自己的环境配置；若未明确管理员密码，初始化日志可能输出临时凭据，务必私密查看并立即管理，不公开日志。

容器卷包括应用数据、PostgreSQL 与 Redis。`docker compose down` 不等同数据备份；**不要执行 `down -v`，除非明确打算删除卷。**

## 3. 反向代理

示例只表示 Nginx 基本转发，证书、限流、管理访问策略和超时需按实际业务补齐。

```nginx
# 放在 http {} 内，保留部分客户端使用的 session_id 等请求头。
underscores_in_headers on;

server {
    listen 443 ssl;
    server_name api.example.com;
    # 在此配置自己的 ssl_certificate 与 ssl_certificate_key。
    client_max_body_size 256m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_buffering off;
        proxy_read_timeout 3600s;
    }
}
```

SSE 不应被代理缓冲；WebSocket 需正确升级。真实 IP 只信任受控反向代理，不接受来自任意公网客户端的伪造转发头。示例未配置证书，不可原样上线。

## 4. 初始化功能

1. 登录后台，配置自己的站点名称、注册策略、支付和上游账号；公开版不附带生产配置或账号池。
2. 在站点内容/外观入口配置首页模式、文档、公告、壁纸和荣誉站点。源码存在功能不代表数据库中已有对应内容。
3. 配置分组、可用模型与价格后再分发用户 Key；主动探测和自动化可能触发正常上游费用。
4. 检查公开模型目录和监控可见范围，避免泄露不应公开的运营信息。

## 5. 备份与升级

- 升级前保留旧 tag/commit、镜像摘要、应用数据目录、密钥与一致性数据库备份；定期演练恢复。
- 数据库迁移应使用正确属主/权限执行。不要重编号、覆盖或删除历史 SQL；有集群时只通过独立迁移器执行。
- 更新源码后重新构建定制镜像，再逐实例检查进程、版本、健康、准入和账务。不要把健康 200 当作全部协议与账务验收。
- 回滚二进制不等于回滚数据库。确认迁移兼容性，禁止未经验证逆向执行 SQL 或删除结算/恢复证据。
- **不要使用后台上游在线更新来替换本版**：当前更新器保留上游地址，可能覆盖定制功能。采用本仓库已审查源码和自己的构建流程。

## 6. 多节点

[架构文档](ARCHITECTURE.md) 解释角色和安全边界。多个普通 Compose 实例共享数据库并不自动构成可靠集群；还需登记、mTLS、准入、连接池预算、恢复槽和独立迁移管理。公开仓库不提供带演示站真实主机、凭据的“一键复制生产集群”脚本。

## 常见问题

- **服务健康但没有页面**：构建后端必须使用 `-tags embed`，并先构建前端。
- **启动报数据表权限**：核对数据库属主、迁移角色和节点授权，不要给所有账号超级用户权限。
- **更新后仍是官方界面**：检查是否拉取了上游镜像，而不是本仓库源码构建。
- **截图/外站图标获取失败**：外部服务可能限流、拒绝或返回 HTML；支持手动上传，不保证任意站点都可自动抓取。
- **吞吐或速度未知**：缺少完整观测或旧记录应显示未知，不代表零速或故障。
