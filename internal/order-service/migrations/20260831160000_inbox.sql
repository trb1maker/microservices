-- +goose Up
CREATE TABLE IF NOT EXISTS inbox (
    consumer TEXT NOT NULL,
    event_id TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (consumer, event_id)
);

-- +goose Down
DROP TABLE IF EXISTS inbox;
