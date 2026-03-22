-- +goose Up
CREATE TABLE IF NOT EXISTS business_profiles (
    user_id         UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    business_name   TEXT NOT NULL DEFAULT '',
    category        TEXT NOT NULL DEFAULT '',
    description     TEXT NOT NULL DEFAULT '',
    address         TEXT NOT NULL DEFAULT '',
    email           TEXT NOT NULL DEFAULT '',
    website         TEXT NOT NULL DEFAULT '',
    phone_public    TEXT NOT NULL DEFAULT '',
    working_hours   JSONB NOT NULL DEFAULT '{}',
    is_verified     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose Down
DROP TABLE IF EXISTS business_profiles;