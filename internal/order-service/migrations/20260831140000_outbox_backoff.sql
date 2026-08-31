-- +goose Up
ALTER TABLE outbox ADD COLUMN IF NOT EXISTS attempts INT NOT NULL DEFAULT 0;
ALTER TABLE outbox ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ;
ALTER TABLE outbox ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '';

DROP INDEX IF EXISTS outbox_unpublished_idx;
CREATE INDEX IF NOT EXISTS outbox_due_idx ON outbox (next_attempt_at, id) WHERE published_at IS NULL;

-- +goose Down
DROP INDEX IF EXISTS outbox_due_idx;
CREATE INDEX IF NOT EXISTS outbox_unpublished_idx ON outbox (id) WHERE published_at IS NULL;
ALTER TABLE outbox DROP COLUMN IF EXISTS last_error;
ALTER TABLE outbox DROP COLUMN IF EXISTS next_attempt_at;
ALTER TABLE outbox DROP COLUMN IF EXISTS attempts;
