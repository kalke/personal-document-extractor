-- +goose Up
ALTER TABLE extractions DROP COLUMN IF EXISTS api_key_id;
DROP TABLE IF EXISTS api_keys;
DELETE FROM users WHERE auth_subject = 'system:ops';

-- +goose Down
CREATE TABLE IF NOT EXISTS api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users (id),
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL UNIQUE,
    key_hash TEXT NOT NULL,
    scopes TEXT[] NOT NULL DEFAULT '{}',
    expires_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE extractions
    ADD COLUMN IF NOT EXISTS api_key_id UUID NULL REFERENCES api_keys (id);

INSERT INTO users (auth_subject, display_name, status)
VALUES ('system:ops', 'System Ops', 'active')
ON CONFLICT (auth_subject) DO NOTHING;
