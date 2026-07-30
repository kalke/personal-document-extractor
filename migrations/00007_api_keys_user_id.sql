-- +goose Up
ALTER TABLE api_keys
    ADD COLUMN user_id UUID REFERENCES users (id);

UPDATE api_keys
SET user_id = (SELECT id FROM users WHERE auth_subject = 'system:ops')
WHERE user_id IS NULL;

ALTER TABLE api_keys
    ALTER COLUMN user_id SET NOT NULL;

CREATE INDEX api_keys_user_id_idx ON api_keys (user_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS api_keys_user_id_idx;
ALTER TABLE api_keys DROP COLUMN IF EXISTS user_id;
