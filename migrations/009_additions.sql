-- +goose Up
-- +goose StatementBegin

ALTER TABLE messages ADD COLUMN IF NOT EXISTS geo JSONB;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS link_preview JSONB;

ALTER TABLE sticker_packs ADD COLUMN IF NOT EXISTS thumb_url TEXT;
ALTER TABLE stickers ADD COLUMN IF NOT EXISTS media_id UUID REFERENCES media(id) ON DELETE CASCADE;
ALTER TABLE stickers ADD COLUMN IF NOT EXISTS position INT DEFAULT 0;
ALTER TABLE user_sticker_packs ADD COLUMN IF NOT EXISTS position INT DEFAULT 0;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE messages DROP COLUMN IF EXISTS geo;
ALTER TABLE messages DROP COLUMN IF EXISTS link_preview;
ALTER TABLE sticker_packs DROP COLUMN IF EXISTS thumb_url;
ALTER TABLE stickers DROP COLUMN IF EXISTS media_id;
ALTER TABLE stickers DROP COLUMN IF EXISTS position;
ALTER TABLE user_sticker_packs DROP COLUMN IF EXISTS position;

-- +goose StatementEnd