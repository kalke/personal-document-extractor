-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE extractions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    doc_type TEXT NOT NULL,
    content_sha256 CHAR(64) NOT NULL,
    filename TEXT,
    mime TEXT,
    mode TEXT,
    model TEXT,
    request_id TEXT,
    status TEXT NOT NULL,
    result_json JSONB NOT NULL,
    duration_ms INT
);

CREATE INDEX extractions_doc_type_sha_idx ON extractions (doc_type, content_sha256);
CREATE INDEX extractions_created_at_idx ON extractions (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS extractions;
