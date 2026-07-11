-- +goose Up
CREATE TABLE users (
    id         UUID PRIMARY KEY,
    email      TEXT NOT NULL UNIQUE,
    password   TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO users (id, email, password) VALUES
    ('11111111-1111-4111-8111-111111111111', 'demo@example.com', '$2a$12$lh6HwPwCsHIGoBzGlyFGzedUVKLzSPdF2qwFwyNWgaFROUOEwOMxW'),
    ('22222222-2222-4222-8222-222222222222', 'admin@example.com', '$2a$12$3VdmnBXHGlSDmh5LO8x1t.LcQu6rqOR/k6iqt4csqjJxmMsbXnBOO');

-- +goose Down
DROP TABLE IF EXISTS users;
