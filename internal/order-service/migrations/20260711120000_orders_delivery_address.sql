-- +goose Up
ALTER TABLE orders ADD COLUMN IF NOT EXISTS delivery_address TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE orders DROP COLUMN IF EXISTS delivery_address;
