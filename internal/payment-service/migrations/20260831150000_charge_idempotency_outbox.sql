-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS transactions_order_type_uidx ON transactions (order_id, type);

CREATE TABLE IF NOT EXISTS outbox (
    id BIGSERIAL PRIMARY KEY,
    aggregate_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    subject TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS payment_outbox_due_idx ON outbox (next_attempt_at, id) WHERE published_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS payment_outbox_due_idx;
DROP TABLE IF EXISTS outbox;
DROP INDEX IF EXISTS transactions_order_type_uidx;
