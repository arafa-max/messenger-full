-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS animated_emoji (
    id          SERIAL PRIMARY KEY,
    emoji       VARCHAR(10)  NOT NULL UNIQUE,  -- сам эмодзи символ
    object_key  TEXT         NOT NULL,          -- путь в MinIO (animated-emoji/😂.json)
    bucket      TEXT         NOT NULL DEFAULT 'animated-emoji',
    width       INT          DEFAULT 512,
    height      INT          DEFAULT 512,
    created_at  TIMESTAMPTZ  DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_animated_emoji ON animated_emoji(emoji);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS animated_emoji CASCADE;
-- +goose StatementEnd