-- +goose Up
CREATE TABLE IF NOT EXISTS receipt_documents (
    order_id UUID PRIMARY KEY,
    user_id UUID NOT NULL,
    total_amount BIGINT NOT NULL,
    status TEXT NOT NULL,
    finalized_at TIMESTAMPTZ NOT NULL,
    delivery_address TEXT NOT NULL DEFAULT '',
    search_text TEXT NOT NULL,
    tsv tsvector GENERATED ALWAYS AS (to_tsvector('simple', search_text)) STORED
);

CREATE INDEX IF NOT EXISTS receipt_documents_tsv_idx ON receipt_documents USING GIN (tsv);
CREATE INDEX IF NOT EXISTS receipt_documents_user_id_idx ON receipt_documents (user_id);

-- +goose Down
DROP INDEX IF EXISTS receipt_documents_user_id_idx;
DROP INDEX IF EXISTS receipt_documents_tsv_idx;
DROP TABLE IF EXISTS receipt_documents;
