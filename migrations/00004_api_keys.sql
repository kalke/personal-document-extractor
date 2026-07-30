-- +goose Up
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    key_prefix TEXT NOT NULL,
    key_hash TEXT NOT NULL,
    scopes TEXT[] NOT NULL DEFAULT '{}',
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT api_keys_key_prefix_unique UNIQUE (key_prefix)
);

CREATE INDEX api_keys_key_prefix_idx ON api_keys (key_prefix) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS api_keys;
