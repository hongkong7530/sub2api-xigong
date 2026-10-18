-- Restore channel-monitor group analytics and reports under an unused migration number.
-- Existing migration numbers are preserved; do not seed or alter production group state here.
ALTER TABLE channel_monitors
  ADD COLUMN IF NOT EXISTS group_id BIGINT REFERENCES groups(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS key_provision_mode VARCHAR(16) NOT NULL DEFAULT 'manual',
  ADD COLUMN IF NOT EXISTS provisioned_api_key_id BIGINT REFERENCES api_keys(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS description VARCHAR(500) NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS description_color VARCHAR(32) NOT NULL DEFAULT '';
ALTER TABLE channel_monitor_histories ADD COLUMN IF NOT EXISTS first_token_ms INTEGER;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_group_id ON channel_monitors(group_id);
CREATE INDEX IF NOT EXISTS idx_channel_monitors_provisioned_api_key_id ON channel_monitors(provisioned_api_key_id);
DO $$ BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='channel_monitors_key_provision_mode_check') THEN
    ALTER TABLE channel_monitors ADD CONSTRAINT channel_monitors_key_provision_mode_check CHECK (key_provision_mode IN ('manual','auto'));
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS channel_monitor_group_rate_history (
  id BIGSERIAL PRIMARY KEY,
  group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  rate_multiplier DECIMAL(10,4) NOT NULL,
  peak_rate_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  peak_rate_multiplier DECIMAL(10,4) NOT NULL DEFAULT 1.0,
  peak_start VARCHAR(5) NOT NULL DEFAULT '',
  peak_end VARCHAR(5) NOT NULL DEFAULT '',
  recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_cm_group_rate_history_group_recorded ON channel_monitor_group_rate_history(group_id, recorded_at DESC);

CREATE OR REPLACE FUNCTION record_channel_monitor_group_rate_change() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.rate_multiplier IS DISTINCT FROM NEW.rate_multiplier
     OR OLD.peak_rate_enabled IS DISTINCT FROM NEW.peak_rate_enabled
     OR OLD.peak_rate_multiplier IS DISTINCT FROM NEW.peak_rate_multiplier
     OR OLD.peak_start IS DISTINCT FROM NEW.peak_start
     OR OLD.peak_end IS DISTINCT FROM NEW.peak_end THEN
    INSERT INTO channel_monitor_group_rate_history(group_id,rate_multiplier,peak_rate_enabled,peak_rate_multiplier,peak_start,peak_end,recorded_at)
    VALUES(NEW.id,NEW.rate_multiplier,NEW.peak_rate_enabled,NEW.peak_rate_multiplier,NEW.peak_start,NEW.peak_end,NOW());
  END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS groups_channel_monitor_rate_history ON groups;
CREATE TRIGGER groups_channel_monitor_rate_history AFTER UPDATE OF rate_multiplier,peak_rate_enabled,peak_rate_multiplier,peak_start,peak_end ON groups FOR EACH ROW EXECUTE FUNCTION record_channel_monitor_group_rate_change();

CREATE TABLE IF NOT EXISTS channel_monitor_subscriptions (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  notify_first_token BOOLEAN NOT NULL DEFAULT TRUE,
  notify_rate_change BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  CONSTRAINT channel_monitor_subscriptions_user_group_unique UNIQUE(user_id,group_id)
);
CREATE INDEX IF NOT EXISTS idx_cm_subscriptions_group_id ON channel_monitor_subscriptions(group_id);
CREATE TABLE IF NOT EXISTS channel_monitor_group_notify_state (
  group_id BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
  first_token_state VARCHAR(20) NOT NULL DEFAULT 'ok',
  consecutive_green INTEGER NOT NULL DEFAULT 0,
  last_first_token_ms INTEGER,
  last_effective_rate DECIMAL(12,4),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS channel_group_reports (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  category VARCHAR(32) NOT NULL,
  body TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_channel_group_reports_group_created ON channel_group_reports(group_id,created_at DESC);
CREATE INDEX IF NOT EXISTS idx_channel_group_reports_user_group_created ON channel_group_reports(user_id,group_id,created_at DESC);
