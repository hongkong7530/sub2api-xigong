# 安全政策

## 私密报告

**不要在公开 Issue、PR、截图或日志中披露漏洞利用细节、真实凭据、客户内容或生产地址。**

优先使用本仓库的 GitHub 私密漏洞报告：

<https://github.com/hongkong7530/sub2api-xigong/security/advisories/new>

如果页面提示该功能尚未启用，请先通过维护者 GitHub 主页 <https://github.com/hongkong7530> 所列的联系方式联系，或开一个**不含任何技术细节**的 Issue 请求私密联系渠道。不要把本定制版报告默认发给上游安全邮箱。

请提供受影响的 tag/commit、权限前提、最小复现、影响范围和建议修复。仅使用自己拥有或明确获授权的测试环境，不对演示站或他人实例进行扫描、压测、支付重放或账号测试。

## 支持边界

安全修复优先面向当前公开主分支，不承诺历史版本长期回移。依赖、模型提供商和第三方素材有各自的更新与披露流程。发布前扫描并不等于完整安全审计。

## 部署基线

- 使用自己的独立强密码、持久 JWT/TOTP 密钥和最小权限数据库账号。
- 生产入口启用 HTTPS；不直接暴露 PostgreSQL、Redis、管理 Socket 或集群工作端口。
- 集群启用 mTLS，备份密钥与恢复目录；先验证角色、连接池总预算和结算恢复，再增加节点。
- 不执行指向不明来源的安装脚本；本定制版优先源码构建，不通过上游在线更新替换二进制。
- 发布截图前检查邮箱、Key、订单、用户、上游地址、日志与浏览器会话信息。
- 日志及数据目录可能含凭据或用户数据，不能提交到 Git 或作为公开 Issue 附件。

## 上游公共 OAuth 客户端常量

源码继承了 Gemini CLI / Antigravity 的公共客户端标识与默认客户端常量，它们不是演示站用户 Token、管理员口令或私人 OAuth 应用密钥。本次不测试其有效性，不认为公开常量授予任何账号权限。部署者必须遵守提供商条款，并按需通过 `GEMINI_CLI_OAUTH_CLIENT_SECRET`、`ANTIGRAVITY_OAUTH_CLIENT_SECRET` 覆盖；不要把自己申请的私密客户端值写入源码。

由于 GitHub 默认按 Google OAuth 模式拦截这些常量，`.github/secret_scanning.yml` 对三个继承文件做了**最小范围**排除，否则公开仓库无法接收上游代码。排除范围应保持精确；新增文件不得借此处绕过扫描，真正新增的私密凭据必须移除并轮换，而不是加入忽略列表。

## English

Report vulnerabilities privately through this repository's Security Advisory page. If unavailable, request a private contact channel without disclosing technical details. Do not test the public demo or unrelated services. Remove credentials and user data from every report. This source release is not a comprehensive security audit.
