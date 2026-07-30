-- +goose Up
ALTER TABLE extractions
    ADD COLUMN deleted_at TIMESTAMPTZ NULL;

CREATE INDEX extractions_active_doc_type_sha_idx
    ON extractions (doc_type, content_sha256)
    WHERE deleted_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS extractions_active_doc_type_sha_idx;
ALTER TABLE extractions DROP COLUMN IF EXISTS deleted_at;
