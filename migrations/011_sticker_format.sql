-- +goose Up
-- +goose StatementBegin

ALTER TABLE stickers
    ADD COLUMN IF NOT EXISTS format     VARCHAR(10) DEFAULT 'static',  -- static | animated | video
    ADD COLUMN IF NOT EXISTS lottie_url TEXT;                          -- URL на Lottie JSON (для animated)

-- Индекс для быстрого поиска анимированных стикеров
CREATE INDEX IF NOT EXISTS idx_stickers_format ON stickers(format);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_stickers_format;
ALTER TABLE stickers DROP COLUMN IF EXISTS lottie_url;
ALTER TABLE stickers DROP COLUMN IF EXISTS format;

-- +goose StatementEnd