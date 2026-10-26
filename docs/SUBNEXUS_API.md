# SubNexus 专用只读 API

本接口由启用 ingress 的西宫 Sub2API 提供。当前仅支持 `bot:read`，不提供机器人写操作；不附带聊天插件、真实身份白名单或演示站凭据。

## 1. 创建与撤销安装

使用自己的管理员登录身份调用：

- `POST /api/v1/admin/bot/installations`：创建安装，凭据只返回一次。
- `GET /api/v1/admin/bot/installations`：列出最多最近 1000 个安装，不返回秘密。
- `DELETE /api/v1/admin/bot/installations/{installation_id}`：撤销安装；下次请求重新读取主库授权。

创建字段：`name`（最多 100 字符）、`platform`、`identities`、`expires_at`、`requests_per_minute`，以及 `all_data` 或 `user_ids`/`group_ids`。每个 identity 包含 `platform_user`、`session_id`、`conversation_id`，完整匹配而非任意交叉组合。身份数量 1–100；业务 ID 为正整数，单列表最多 1000 项。有效期必须在未来且不超过 366 天；每分钟限额默认 60、范围 1–600。

`all_data=true` 不能同时指定业务 ID 列表；默认 false 时至少一个列表非空，两个列表都有则取交集。请求参数只能缩小授权范围。用户受限安装不能查询组级账号/渠道库存或组级价格。

创建响应包含 `bearer_token`、`installation_id`、`installation_secret` 和安装策略，使用 `Cache-Control: no-store`。将秘密直接放入受限凭据存储，不写代码、聊天记录或日志。轮换采用新建相同或收紧范围的安装、更新客户端、撤销旧安装，不提供读回原秘密接口。

## 2. 请求签名

专用 Bearer 令牌格式为 `astrbot_<32位小写hex安装ID>_<64位随机hex>`。使用独立安装密钥，对以下八段按顺序以 LF 连接后执行 HMAC-SHA256，发送小写十六进制签名：

1. HTTP 方法。
2. 原始转义路径及原始查询串。
3. UTC 秒精度 RFC3339 时间。
4. nonce。
5. 真实聊天用户。
6. session。
7. conversation。
8. 原始 body。

GET body 必须为空，因此七个分隔符使签名原文以 LF 结束；不要增加 body 摘要或额外安装头。请求头为 `Authorization: Bearer <专用令牌>`、`X-AstrBot-Timestamp`、`X-AstrBot-Nonce`、`X-AstrBot-Signature`、`X-AstrBot-Platform-User`、`X-AstrBot-Session-ID`、`X-AstrBot-Conversation-ID`，每项只能出现一次。nonce 使用 16–128 位字母、数字、下划线或连字符；时间格式为 `YYYY-MM-DDTHH:MM:SSZ`。身份字段签名前去除首尾空白，方法名使用大写。代理必须保留原始路径、查询顺序与编码。

允许正负 5 分钟时间偏差。共享 Redis 原子执行 nonce 防重放和安装限流，nonce 保留至签名时间加 5 分钟再加 1 秒。重放 409，限流 429 且 `Retry-After: 60`；Redis/数据库/解密失败不降级为本机信任或伪造空结果。

当前协议由令牌定位安装的平台，不通过查询参数信任聊天身份；它不能单独证明客户端实际属于哪个平台。多个聊天平台应使用独立安装。

## 3. 八类读取接口

均为 `GET /api/v1/bot` 下路径，成功封装为 `code=0`、`message=success`、`data`。

| 路径 | 数据与口径 |
| --- | --- |
| `/costs` | 已结算收益事实中的成本；USD 十进制字符串，未知为 null，不视为零。 |
| `/profit` | 已知成本的计费毛利与授权范围汇总，不是现金净利润。 |
| `/consumption` | 当前保留用量明细的 Token/金额，`history_complete=false`，不冒充全历史。 |
| `/logs` | 用量白名单字段，不提供 IP、UA、Key、请求正文、错误正文或系统日志。 |
| `/model-audit` | 请求/转发/上游声明模型，可筛 mismatch；缺失声明保持 null，不证明内部实际模型。 |
| `/accounts/status` | 账号配置状态、开关与恢复时间；不是实时健康或最终可调度结论。 |
| `/channels/status` | 授权渠道的配置状态，不触发主动探测。 |
| `/models/prices` | 分组倍率与显式价卡字段；未解析继承/峰价/用户覆盖，不代表最终有效扣费。 |

`model-mismatches` 使用 `/model-audit?mismatch=true`；客户端本地报告不是新的服务端路由。

## 4. 查询与错误边界

- 默认最近 24 小时；时间范围为 `[start,end)`，最长 366 天，RFC3339 必须带时区。仅传 end 时从其前 24 小时开始。收益响应提供冻结账本时区与统计起点。
- 分页默认 page=1/page_size=50，上限 100000/1000。键只能出现一次、值不可空且最多 256 字符，总查询串最多 8192 字节。
- 成本/利润：`start,end,page,page_size,platform,group_id,user_id,type`；type 为 balance/subscription。
- 消耗：时间/分页/平台及 `group_id,user_id,account_id,channel_id,requested_model,upstream_model,upstream_response_model`；分页不改变聚合总计。
- 日志/审计：消耗参数加 `status,mismatch,log_source`；当前 status 只支持 all，log_source 只支持 usage；其他状态/系统日志返回 501。
- 账号：`page,page_size,platform,group_id,account_id,type`；渠道：`page,page_size,platform,group_id,channel_id`；价格：`page,page_size,platform,group_id`。
- `platform` 查询为业务供应商过滤，不是聊天平台身份。未知查询键 400，未知 GET 501，任何 `/api/v1/bot/*` 非 GET 403。
- 每次执行只读事务，SQL 上限 8 秒、查询上下文 10 秒、响应上限 2 MiB；超限不返回部分成功。

## 5. 部署与验收

包含迁移 `282_subnexus_bot.sql` 和 Wire/路由接线。节点间必须共享受保护的签名加密材料、权威数据库及 Redis。先按自己的部署流程备份、迁移并授予最小权限，再创建安装；不得把普通管理员密钥或模型 API Key 当作机器人令牌。

建议依次验证：八路读取、越权数据范围、未授权聊天身份、交叉拼接身份、非 GET 拒绝、跨入口 nonce 重放、撤销后拒绝；最后用真实获准聊天账号完成消息往返。HTTP/模拟身份测试不等于真人聊天验收。本版不实现群推送、定时报表、AI 外发或写操作。
