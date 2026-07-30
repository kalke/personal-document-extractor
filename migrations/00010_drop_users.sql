-- +goose Up
ALTER TABLE extractions DROP COLUMN IF EXISTS user_id;
DROP TABLE IF EXISTS users;

-- +goose Down
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    auth_subject TEXT NOT NULL,
    email TEXT NULL,
    email_verified BOOLEAN NOT NULL DEFAULT false,
    display_name TEXT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ NULL,
    CONSTRAINT users_auth_subject_unique UNIQUE (auth_subject),
    CONSTRAINT users_status_check CHECK (status IN ('active', 'disabled'))
);

ALTER TABLE extractions
    ADD COLUMN IF NOT EXISTS user_id UUID NULL REFERENCES users (id);
