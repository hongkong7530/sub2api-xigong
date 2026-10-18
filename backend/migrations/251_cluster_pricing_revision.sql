CREATE TABLE IF NOT EXISTS cluster_pricing_revision (
    id SMALLINT PRIMARY KEY CHECK(id=1),
    revision BIGINT NOT NULL DEFAULT 1
);
INSERT INTO cluster_pricing_revision(id) VALUES(1) ON CONFLICT DO NOTHING;
CREATE OR REPLACE FUNCTION bump_cluster_pricing_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    UPDATE cluster_pricing_revision SET revision=revision+1 WHERE id=1;
    RETURN NULL;
END;
$$;
DO $$ DECLARE target text; BEGIN
    FOREACH target IN ARRAY ARRAY['channels','channel_groups','channel_model_pricing','channel_pricing_intervals','channel_account_stats_pricing_rules','channel_account_stats_model_pricing','channel_account_stats_pricing_intervals','user_group_rate_multipliers'] LOOP
        EXECUTE format('CREATE TRIGGER cluster_pricing_revision_change AFTER INSERT OR UPDATE OR DELETE OR TRUNCATE ON %I FOR EACH STATEMENT EXECUTE FUNCTION bump_cluster_pricing_revision()',target);
    END LOOP;
END $$;
