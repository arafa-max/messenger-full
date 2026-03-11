-- +goose Up
-- +goose StatementBegin

ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS media_id UUID REFERENCES media(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_messages_media ON messages(media_id) WHERE media_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_messages_media;
ALTER TABLE messages DROP COLUMN IF EXISTS media_id;

-- +goose StatementEnd