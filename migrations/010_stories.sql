-- +goose Up
-- +goose StatementBegin

-- Реакции на stories
CREATE TABLE story_reactions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    story_id    UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji       VARCHAR(10) NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (story_id, user_id)
);
CREATE INDEX idx_story_reactions ON story_reactions(story_id);

-- Близкие друзья
CREATE TABLE close_friends (
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    friend_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, friend_id)
);

-- Архив stories
CREATE TABLE story_archive (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_id    UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_story_archive_user ON story_archive(user_id, archived_at DESC);

-- Расширяем stories
ALTER TABLE stories
    ADD COLUMN IF NOT EXISTS is_archived        BOOLEAN DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS audience           VARCHAR(20) DEFAULT 'everyone',
    ADD COLUMN IF NOT EXISTS media_id           UUID REFERENCES media(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS sticker_data       JSONB,
    ADD COLUMN IF NOT EXISTS music_data         JSONB;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

ALTER TABLE stories
    DROP COLUMN IF EXISTS is_archived,
    DROP COLUMN IF EXISTS audience,
    DROP COLUMN IF EXISTS media_id,
    DROP COLUMN IF EXISTS sticker_data,
    DROP COLUMN IF EXISTS music_data;

DROP TABLE IF EXISTS story_archive CASCADE;
DROP TABLE IF EXISTS close_friends CASCADE;
DROP TABLE IF EXISTS story_reactions CASCADE;

-- +goose StatementEnd