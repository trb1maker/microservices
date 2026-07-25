-- +goose Up
INSERT INTO accounts (user_id, balance, version) VALUES
    ('user-1', 100000, 1),
    ('user-2', 50000, 1),
    ('user-3', 25000, 1)
ON CONFLICT (user_id) DO NOTHING;

-- +goose Down
DELETE FROM accounts WHERE user_id IN ('user-1', 'user-2', 'user-3');