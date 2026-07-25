-- +goose Up
CREATE TABLE IF NOT EXISTS daily_summary (
    date DATE PRIMARY KEY,
    total_orders INT NOT NULL DEFAULT 0,
    total_revenue BIGINT NOT NULL DEFAULT 0,
    avg_order_value DOUBLE PRECISION NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS processed_orders (
    order_id UUID PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS processed_orders;
DROP TABLE IF EXISTS daily_summary;
