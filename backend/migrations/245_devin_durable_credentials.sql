-- Durable cog_* keys may mint session credentials; identity and verification
-- remain versioned host-account properties. Existing session-only rows keep
-- exactly the same fingerprint and credential version.
DROP INDEX IF EXISTS accounts_devin_session_identity_unique;
CREATE UNIQUE INDEX accounts_devin_session_identity_unique
    ON accounts ((credentials->>'credential_fingerprint'))
    WHERE platform='devin' AND deleted_at IS NULL;

CREATE OR REPLACE FUNCTION normalize_devin_account_credentials() RETURNS trigger
LANGUAGE plpgsql AS $$
DECLARE
    incoming_token text;
    durable_key text;
    identity_value text;
    old_version bigint;
BEGIN
    IF NEW.platform <> 'devin' THEN RETURN NEW; END IF;
    incoming_token := COALESCE(NEW.credentials->>'session_token', '');
    durable_key := COALESCE(NEW.credentials->>'devin_api_key', '');
    IF NEW.type <> 'apikey'
       OR NEW.credentials->>'auth_mode' IS DISTINCT FROM 'session_token'
       OR jsonb_typeof(NEW.credentials->'session_token') IS DISTINCT FROM 'string'
       OR (incoming_token = '' AND durable_key = '')
       OR length(incoming_token) > 16384 OR incoming_token ~ '[[:space:]]'
       OR (NEW.credentials ? 'devin_api_key' AND (
           jsonb_typeof(NEW.credentials->'devin_api_key') IS DISTINCT FROM 'string'
           OR durable_key NOT LIKE 'cog\_%' ESCAPE '\'
           OR length(durable_key) > 16384 OR durable_key ~ '[[:space:]]'))
       OR NEW.credentials ?| ARRAY['credentials_content', 'token', 'api_key', 'windsurf_api_key'] THEN
        RAISE EXCEPTION 'invalid Devin credential record' USING ERRCODE='23514';
    END IF;
    IF (COALESCE(NEW.credentials->>'credentials_file', '') = '')
       <> (COALESCE(NEW.credentials->>'credentials_node', '') = '') THEN
        RAISE EXCEPTION 'incomplete Devin credential source' USING ERRCODE='23514';
    END IF;
    IF COALESCE(NEW.credentials->>'credentials_file', '') <> '' AND (
        NEW.credentials->>'credentials_file' ~ '(^/|(^|/)\.\.?(/|$)|//|:)'
        OR position(chr(92) in NEW.credentials->>'credentials_file') > 0
        OR NEW.credentials->>'credentials_node' !~ '^[a-z0-9._-]+$') THEN
        RAISE EXCEPTION 'noncanonical Devin credential source' USING ERRCODE='23514';
    END IF;
    identity_value := CASE WHEN durable_key <> '' THEN 'devin-api-key:' || durable_key ELSE incoming_token END;
    NEW.credentials := jsonb_set(NEW.credentials, '{credential_fingerprint}',
        to_jsonb(encode(sha256(convert_to(identity_value, 'UTF8')), 'hex')), true);
    IF TG_OP = 'INSERT' THEN
        NEW.credentials := jsonb_set(NEW.credentials, '{credential_version}', '1'::jsonb, true);
    ELSE
        old_version := COALESCE((OLD.credentials->>'credential_version')::bigint, 0);
        IF OLD.platform IS DISTINCT FROM NEW.platform
           OR OLD.credentials->'session_token' IS DISTINCT FROM NEW.credentials->'session_token'
           OR OLD.credentials->'devin_api_key' IS DISTINCT FROM NEW.credentials->'devin_api_key'
           OR OLD.credentials->'base_url' IS DISTINCT FROM NEW.credentials->'base_url'
           OR OLD.credentials->'credentials_file' IS DISTINCT FROM NEW.credentials->'credentials_file'
           OR OLD.credentials->'credentials_node' IS DISTINCT FROM NEW.credentials->'credentials_node' THEN
            old_version := old_version + 1;
        END IF;
        NEW.credentials := jsonb_set(NEW.credentials, '{credential_version}', to_jsonb(GREATEST(old_version, 1)), true);
    END IF;
    RETURN NEW;
END;
$$;
