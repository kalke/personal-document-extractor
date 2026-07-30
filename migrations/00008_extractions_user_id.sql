-- +goose Up
ALTER TABLE extractions
    ADD COLUMN user_id UUID REFERENCES users (id);

CREATE INDEX extractions_user_id_idx ON extractions (user_id) WHERE user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS extractions_user_id_idx;
ALTER TABLE extractions DROP COLUMN IF EXISTS user_id;
