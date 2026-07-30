-- +goose Up
ALTER TABLE extractions
    ADD COLUMN IF NOT EXISTS auth_client TEXT,
    ADD COLUMN IF NOT EXISTS auth_email TEXT;

CREATE INDEX IF NOT EXISTS extractions_auth_client_idx ON extractions (auth_client) WHERE auth_client IS NOT NULL;
CREATE INDEX IF NOT EXISTS extractions_auth_email_idx ON extractions (auth_email) WHERE auth_email IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS extractions_auth_email_idx;
DROP INDEX IF EXISTS extractions_auth_client_idx;
ALTER TABLE extractions
    DROP COLUMN IF EXISTS auth_email,
    DROP COLUMN IF EXISTS auth_client;
