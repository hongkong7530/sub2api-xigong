-- Migration: 266_channel_monitor_group_binding
-- 渠道监控 V1 主动探测与分组打通：
--   1. group_id 建立监控与分组的真实外键关联（此前只有自由文本 group_name，
--      导致倍率、缓存率、分组描述和分组 Key 都无法接入监控）
--   2. key_provision_mode + provisioned_api_key_id 记录「自动配给的监控 Key」，
--      使监控可以只选分组、由后端自动建 Key，并在换绑/删除时回收
--   3. description + description_color 提供监控面板上的自定义彩色描述
--   4. channel_monitor_group_rate_history 记录分组倍率变更，用于历史倍率与倍率曲线
--
-- 缓存率不建表：usage_logs 已有 idx_usage_logs_group_created_at_not_null
-- (group_id, created_at) 索引，按分组时间窗实时聚合即可。

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL;

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS key_provision_mode VARCHAR(16) NOT NULL DEFAULT 'manual';

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS provisioned_api_key_id BIGINT REFERENCES api_keys(id) ON DELETE SET NULL;

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS description VARCHAR(500) NOT NULL DEFAULT '';

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS description_color VARCHAR(32) NOT NULL DEFAULT '';

-- 约束按存在性判断补建，保证迁移在部分应用后的库上可重跑。
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint c
          JOIN pg_class t ON t.oid = c.conrelid
         WHERE t.relname = 'channel_monitors'
           AND c.conname = 'channel_monitors_key_provision_mode_check'
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_key_provision_mode_check
            CHECK (key_provision_mode IN ('manual', 'auto'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_group_id
    ON channel_monitors(group_id);

CREATE INDEX IF NOT EXISTS idx_channel_monitors_provisioned_api_key_id
    ON channel_monitors(provisioned_api_key_id);

COMMENT ON COLUMN channel_monitors.group_id IS
    '监控绑定的分组 ID；分组删除时置空，监控保留并报「分组未关联」';
COMMENT ON COLUMN channel_monitors.key_provision_mode IS
    'manual = 管理员手填 API Key；auto = 后端按 group_id 自动配给专用监控 Key';
COMMENT ON COLUMN channel_monitors.provisioned_api_key_id IS
    '自动配给模式下后端创建的 api_keys.id；换绑分组或删除监控时回收';
COMMENT ON COLUMN channel_monitors.description IS
    '监控面板上展示的自定义描述文本';
COMMENT ON COLUMN channel_monitors.description_color IS
    '描述文本颜色（#RRGGBB 或预置色板 token）；空表示使用默认前景色';

-- 存量监控按 group_name 回填 group_id。
-- groups_name_unique_active 保证活跃分组名唯一，匹配无歧义。
UPDATE channel_monitors cm
   SET group_id = g.id
  FROM groups g
 WHERE cm.group_id IS NULL
   AND cm.group_name <> ''
   AND g.deleted_at IS NULL
   AND g.name = cm.group_name;

-- 分组倍率变更历史：仅在倍率相关字段发生变化时追加一行，
-- 因此表规模与「变更次数」成正比，而不是与采样频率成正比。
CREATE TABLE IF NOT EXISTS channel_monitor_group_rate_history (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    rate_multiplier DECIMAL(10, 4) NOT NULL,
    peak_rate_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    peak_rate_multiplier DECIMAL(10, 4) NOT NULL DEFAULT 1.0,
    peak_start VARCHAR(5) NOT NULL DEFAULT '',
    peak_end VARCHAR(5) NOT NULL DEFAULT '',
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE channel_monitor_group_rate_history IS
    '分组倍率变更历史，供渠道监控展示历史倍率与倍率变化曲线。';
COMMENT ON COLUMN channel_monitor_group_rate_history.recorded_at IS
    '观察到该组倍率取值的时间；曲线按阶梯函数在相邻两点间保持不变。';

CREATE INDEX IF NOT EXISTS idx_cm_group_rate_history_group_recorded
    ON channel_monitor_group_rate_history(group_id, recorded_at DESC);

-- 为每个活跃分组写入曲线起点，避免新面板在首次倍率变更前无数据可画。
INSERT INTO channel_monitor_group_rate_history (
    group_id, rate_multiplier, peak_rate_enabled,
    peak_rate_multiplier, peak_start, peak_end, recorded_at
)
SELECT g.id, g.rate_multiplier, g.peak_rate_enabled,
       g.peak_rate_multiplier, g.peak_start, g.peak_end, NOW()
  FROM groups g
 WHERE g.deleted_at IS NULL
   AND NOT EXISTS (
       SELECT 1
         FROM channel_monitor_group_rate_history h
        WHERE h.group_id = g.id
   );

CREATE OR REPLACE FUNCTION record_channel_monitor_group_rate_change()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.rate_multiplier IS DISTINCT FROM NEW.rate_multiplier
       OR OLD.peak_rate_enabled IS DISTINCT FROM NEW.peak_rate_enabled
       OR OLD.peak_rate_multiplier IS DISTINCT FROM NEW.peak_rate_multiplier
       OR OLD.peak_start IS DISTINCT FROM NEW.peak_start
       OR OLD.peak_end IS DISTINCT FROM NEW.peak_end THEN
        INSERT INTO channel_monitor_group_rate_history (
            group_id, rate_multiplier, peak_rate_enabled,
            peak_rate_multiplier, peak_start, peak_end, recorded_at
        )
        VALUES (
            NEW.id, NEW.rate_multiplier, NEW.peak_rate_enabled,
            NEW.peak_rate_multiplier, NEW.peak_start, NEW.peak_end, NOW()
        );
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS groups_channel_monitor_rate_history ON groups;
CREATE TRIGGER groups_channel_monitor_rate_history
AFTER UPDATE OF rate_multiplier, peak_rate_enabled, peak_rate_multiplier, peak_start, peak_end
ON groups
FOR EACH ROW
EXECUTE FUNCTION record_channel_monitor_group_rate_change();
