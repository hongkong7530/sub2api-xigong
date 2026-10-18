-- Durable facts must survive retention of optional request detail.
ALTER TABLE billing_usage_entries ALTER COLUMN usage_log_id DROP NOT NULL;
ALTER TABLE billing_usage_entries DROP CONSTRAINT IF EXISTS billing_usage_entries_usage_log_id_fkey;
ALTER TABLE billing_usage_entries ADD CONSTRAINT billing_usage_entries_usage_log_id_fkey
    FOREIGN KEY (usage_log_id) REFERENCES usage_logs(id) ON DELETE SET NULL NOT VALID;
ALTER TABLE billing_usage_entries ADD COLUMN IF NOT EXISTS settlement_event_id TEXT;
ALTER TABLE billing_usage_entries ADD COLUMN IF NOT EXISTS settlement_fingerprint VARCHAR(64);

-- Activation is an explicit cutover operation after all old writers are fenced.
-- This trigger also rejects an already-running legacy absolute snapshot flusher.
CREATE OR REPLACE FUNCTION enforce_cluster_platform_quota_authority() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.daily_usage_usd,NEW.weekly_usage_usd,NEW.monthly_usage_usd,NEW.daily_window_start,NEW.weekly_window_start,NEW.monthly_window_start)
       IS DISTINCT FROM
       (OLD.daily_usage_usd,OLD.weekly_usage_usd,OLD.monthly_usage_usd,OLD.daily_window_start,OLD.weekly_window_start,OLD.monthly_window_start)
       AND EXISTS(SELECT 1 FROM cluster_control WHERE settlement_mode='atomic')
       AND COALESCE(current_setting('sub2api.quota_writer',true),'') NOT IN ('settlement-v1','administrator-reset-v1') THEN
        RAISE EXCEPTION 'legacy platform quota writes are fenced by cluster atomic authority' USING ERRCODE='55000';
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS cluster_platform_quota_authority ON user_platform_quotas;
CREATE TRIGGER cluster_platform_quota_authority BEFORE UPDATE ON user_platform_quotas
    FOR EACH ROW EXECUTE FUNCTION enforce_cluster_platform_quota_authority();
