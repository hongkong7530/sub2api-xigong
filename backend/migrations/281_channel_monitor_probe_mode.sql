-- Additive for rolling deployment; existing monitors retain active + passive behavior.
ALTER TABLE channel_monitors ADD COLUMN probe_mode varchar(16) NOT NULL DEFAULT 'hybrid';
ALTER TABLE channel_monitors ADD CONSTRAINT channel_monitors_probe_mode_check CHECK (probe_mode IN ('hybrid','passive'));
-- A missing baseline is a current observation, never a fabricated group-creation price.
INSERT INTO channel_monitor_group_rate_history(group_id,rate_multiplier,peak_rate_enabled,peak_rate_multiplier,peak_start,peak_end,recorded_at)
SELECT g.id,g.rate_multiplier,g.peak_rate_enabled,g.peak_rate_multiplier,g.peak_start,g.peak_end,statement_timestamp()
FROM groups g WHERE g.deleted_at IS NULL AND NOT EXISTS(SELECT 1 FROM channel_monitor_group_rate_history h WHERE h.group_id=g.id);
CREATE OR REPLACE FUNCTION record_channel_monitor_group_rate_initial() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO channel_monitor_group_rate_history(group_id,rate_multiplier,peak_rate_enabled,peak_rate_multiplier,peak_start,peak_end,recorded_at)
 VALUES(NEW.id,NEW.rate_multiplier,NEW.peak_rate_enabled,NEW.peak_rate_multiplier,NEW.peak_start,NEW.peak_end,clock_timestamp());
 RETURN NEW;
END $$;
CREATE TRIGGER groups_channel_monitor_rate_initial AFTER INSERT ON groups FOR EACH ROW EXECUTE FUNCTION record_channel_monitor_group_rate_initial();
