-- +goose Up
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    auth_subject TEXT NOT NULL,
    email TEXT,
    email_verified BOOLEAN NOT NULL DEFAULT false,
    display_name TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ,
    CONSTRAINT users_auth_subject_unique UNIQUE (auth_subject),
    CONSTRAINT users_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE INDEX users_email_idx ON users (email) WHERE email IS NOT NULL;

INSERT INTO users (auth_subject, display_name, status)
VALUES ('system:ops', 'System Ops', 'active');

-- +goose Down
DROP TABLE IF EXISTS users;
