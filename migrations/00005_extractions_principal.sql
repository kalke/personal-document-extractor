-- +goose Up
ALTER TABLE extractions
    ADD COLUMN api_key_id UUID REFERENCES api_keys (id),
    ADD COLUMN auth_subject TEXT;

CREATE INDEX extractions_api_key_id_idx ON extractions (api_key_id) WHERE api_key_id IS NOT NULL;
CREATE INDEX extractions_auth_subject_idx ON extractions (auth_subject) WHERE auth_subject IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS extractions_auth_subject_idx;
DROP INDEX IF EXISTS extractions_api_key_id_idx;
ALTER TABLE extractions
    DROP COLUMN IF EXISTS auth_subject,
    DROP COLUMN IF EXISTS api_key_id;
