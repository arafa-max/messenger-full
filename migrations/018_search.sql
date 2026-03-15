-- +goose Up
-- +goose StatementBegin

-- ============================================
-- BLOCK 10 — SEARCH (без pgvector)
-- pgvector → 019_semantic_search.sql
-- (нужен образ pgvector/pgvector:pg17)
-- ============================================

-- GIN индекс на chats.name для глобального поиска каналов/групп
CREATE INDEX IF NOT EXISTS idx_chats_name_search
    ON chats USING gin(name gin_trgm_ops);

-- GIN индекс на chats.description
CREATE INDEX IF NOT EXISTS idx_chats_description_search
    ON chats USING gin(description gin_trgm_ops);

-- GIN индекс на sticker_packs.name для поиска стикер-паков
CREATE INDEX IF NOT EXISTS idx_sticker_packs_name_search
    ON sticker_packs USING gin(name gin_trgm_ops);

-- Таблица для поискового индекса медиа (OCR + Whisper)
CREATE TABLE IF NOT EXISTS media_search_index (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    media_id    UUID NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    message_id  UUID REFERENCES messages(id) ON DELETE CASCADE,
    content     TEXT NOT NULL,           -- распознанный текст (OCR или Whisper)
    source      VARCHAR(20) NOT NULL,    -- ocr, whisper
    lang        VARCHAR(10) DEFAULT 'ru',
    indexed_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_media_search_content
    ON media_search_index USING gin(content gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_media_search_media
    ON media_search_index(media_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP INDEX IF EXISTS idx_media_search_media;
DROP INDEX IF EXISTS idx_media_search_content;
DROP TABLE IF EXISTS media_search_index CASCADE;
DROP INDEX IF EXISTS idx_sticker_packs_name_search;
DROP INDEX IF EXISTS idx_chats_description_search;
DROP INDEX IF EXISTS idx_chats_name_search;

-- +goose StatementEnd