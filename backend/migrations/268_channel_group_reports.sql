-- Migration: 268_channel_group_reports
-- 渠道状态分组用户反馈：已绑定分组的登录用户可提交类型+Markdown 正文；
-- 用户软删后分组侧记录仍保留，展示时标记为已删除用户。

CREATE TABLE IF NOT EXISTS channel_group_reports (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    category VARCHAR(32) NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channel_group_reports_group_created
    ON channel_group_reports (group_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_channel_group_reports_user_group_created
    ON channel_group_reports (user_id, group_id, created_at DESC);

COMMENT ON TABLE channel_group_reports IS
    '用户对已绑定监控分组的反馈：category=unreachable|slow_first_token|usage_doubt|other；body 为原始 Markdown。';
