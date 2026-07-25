-- +goose Up
CREATE TABLE IF NOT EXISTS order_items (
    order_id    UUID   NOT NULL REFERENCES orders (order_id) ON DELETE CASCADE,
    product_id  UUID   NOT NULL,
    quantity    BIGINT NOT NULL,
    unit_price  BIGINT NOT NULL,
    total_price BIGINT NOT NULL,
    PRIMARY KEY (order_id, product_id)
);

-- +goose Down
DROP TABLE IF EXISTS order_items;
