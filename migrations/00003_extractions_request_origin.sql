-- +goose Up
ALTER TABLE extractions
    ADD COLUMN client_ip TEXT,
    ADD COLUMN user_agent TEXT;

-- +goose Down
ALTER TABLE extractions
    DROP COLUMN IF EXISTS user_agent,
    DROP COLUMN IF EXISTS client_ip;
