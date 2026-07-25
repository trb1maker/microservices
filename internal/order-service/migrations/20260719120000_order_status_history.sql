-- +goose Up
CREATE TABLE IF NOT EXISTS order_status_history (
    id         BIGSERIAL PRIMARY KEY,
    order_id   UUID        NOT NULL REFERENCES orders (order_id) ON DELETE CASCADE,
    status     TEXT        NOT NULL,
    reason     TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_order_status_history_order_id_created_at
    ON order_status_history (order_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS order_status_history;
