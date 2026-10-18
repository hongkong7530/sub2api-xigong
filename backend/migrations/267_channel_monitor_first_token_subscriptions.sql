-- Migration: 267_channel_monitor_first_token_subscriptions
-- 渠道监控首字（TTFT）探测与分组订阅通知：
--   1. channel_monitor_histories.first_token_ms 记录流式探测首个非空内容增量的毫秒延迟
--   2. channel_monitor_subscriptions 用户按分组订阅「首字恢复」与「倍率变化」邮件
--   3. channel_monitor_group_notify_state 分组首字恢复状态机与倍率变化去重状态
--      （持久化到共享主库，保证服务重启 / 双节点滚动期间不丢状态、不重复发信）

ALTER TABLE channel_monitor_histories
    ADD COLUMN IF NOT EXISTS first_token_ms INT;

COMMENT ON COLUMN channel_monitor_histories.first_token_ms IS
    '流式探测首个非空内容增量的毫秒延迟（TTFT）；非流式兼容路径或失败为 NULL';

CREATE TABLE IF NOT EXISTS channel_monitor_subscriptions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    notify_first_token BOOLEAN NOT NULL DEFAULT TRUE,
    notify_rate_change BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT channel_monitor_subscriptions_user_group_unique UNIQUE (user_id, group_id)
);

CREATE INDEX IF NOT EXISTS idx_cm_subscriptions_group_id
    ON channel_monitor_subscriptions(group_id);

COMMENT ON TABLE channel_monitor_subscriptions IS
    '用户对分组监控的邮件订阅：notify_first_token = 首字恢复通知；notify_rate_change = 倍率变化通知；两开关全关等价于退订。';

CREATE TABLE IF NOT EXISTS channel_monitor_group_notify_state (
    group_id BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    first_token_state VARCHAR(20) NOT NULL DEFAULT 'ok',
    consecutive_green INT NOT NULL DEFAULT 0,
    last_first_token_ms INT,
    last_effective_rate DECIMAL(12, 4),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE channel_monitor_group_notify_state IS
    '分组通知状态机：first_token_state = ok | pending_recovery。首字变红进入 pending_recovery 并清零 consecutive_green；此后每次绿 +1（黄/红清零），连续 3 次绿发送恢复邮件并复位为 ok。last_effective_rate 记录上次通知的有效倍率，用于倍率变化去重（含双节点防重复发信）。';
