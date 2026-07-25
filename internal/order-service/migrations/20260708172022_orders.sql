-- +goose Up
CREATE TABLE IF NOT EXISTS orders (
    order_id    UUID        PRIMARY KEY,
    user_id     UUID        NOT NULL,
    status      TEXT        NOT NULL,
    total_price BIGINT      NOT NULL,
    payment_id  UUID,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS orders;
