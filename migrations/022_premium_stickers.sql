-- +goose Up
ALTER TABLE sticker_packs ADD COLUMN IF NOT EXISTS is_premium BOOLEAN NOT NULL DEFAULT FALSE;

-- +goose Down
ALTER TABLE sticker_packs DROP COLUMN IF EXISTS is_premium;