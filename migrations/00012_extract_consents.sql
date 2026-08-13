-- +goose Up
CREATE TABLE IF NOT EXISTS extract_consents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    accepted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    user_sub TEXT NOT NULL,
    user_email TEXT,
    ip TEXT,
    user_agent TEXT,
    policy_version TEXT NOT NULL,
    content_sha256 CHAR(64),
    doc_type TEXT,
    extraction_id UUID
);

CREATE INDEX IF NOT EXISTS extract_consents_user_sub_idx ON extract_consents (user_sub);
CREATE INDEX IF NOT EXISTS extract_consents_accepted_at_idx ON extract_consents (accepted_at DESC);
CREATE INDEX IF NOT EXISTS extract_consents_policy_version_idx ON extract_consents (policy_version);

-- +goose Down
DROP INDEX IF EXISTS extract_consents_policy_version_idx;
DROP INDEX IF EXISTS extract_consents_accepted_at_idx;
DROP INDEX IF EXISTS extract_consents_user_sub_idx;
DROP TABLE IF EXISTS extract_consents;
