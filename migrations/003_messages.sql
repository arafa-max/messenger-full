-- +goose Up
-- +goose StatementBegin

-- ============================================
-- PINNED MESSAGES (несколько закреплённых)
-- ============================================
CREATE TABLE IF NOT EXISTS pinned_messages (
    chat_id         UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    pinned_by       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pinned_at       TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (chat_id, message_id)
);
CREATE INDEX IF NOT EXISTS idx_pinned_chat ON pinned_messages(chat_id, pinned_at DESC);

-- ============================================
-- SAVED MESSAGES (избранное, как "Избранное" в Telegram)
-- ============================================
CREATE TABLE IF NOT EXISTS saved_messages (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    saved_at        TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (user_id, message_id)
);
CREATE INDEX IF NOT EXISTS idx_saved_user ON saved_messages(user_id, saved_at DESC);

-- ============================================
-- MESSAGE REMINDERS (напоминание на сообщение)
-- ============================================
CREATE TABLE IF NOT EXISTS message_reminders (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    remind_at       TIMESTAMPTZ NOT NULL,
    is_sent         BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (user_id, message_id)
);
CREATE INDEX IF NOT EXISTS idx_reminders_pending ON message_reminders(remind_at) WHERE is_sent = FALSE;

-- ============================================
-- QUICK REPLIES (быстрые ответы — шаблоны)
-- ============================================
CREATE TABLE IF NOT EXISTS quick_replies (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    shortcut        VARCHAR(32) NOT NULL,        -- /привет
    text            TEXT NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (user_id, shortcut)
);
CREATE INDEX IF NOT EXISTS idx_quick_replies_user ON quick_replies(user_id);

-- ============================================
-- РАСШИРЯЕМ messages — новые поля для Блока 3
-- ============================================

-- Форматирование текста (spoiler, markdown, quote)
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS format         VARCHAR(20) DEFAULT 'plain', -- plain, markdown, html
    ADD COLUMN IF NOT EXISTS is_spoiler     BOOLEAN DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS quoted_text    TEXT,        -- цитируемый фрагмент текста
    ADD COLUMN IF NOT EXISTS quoted_offset  INT,         -- позиция цитаты в оригинале
    ADD COLUMN IF NOT EXISTS quoted_length  INT;         -- длина цитаты

-- Forward — расширяем метаданные пересылки
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS forward_sender_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS forward_chat_id     UUID REFERENCES chats(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS forward_date        TIMESTAMPTZ;

-- Delete for me / delete for everyone
-- is_deleted уже есть (delete for everyone)
-- Нужна таблица для "удалить только у себя"
CREATE TABLE IF NOT EXISTS deleted_messages (
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deleted_at      TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (message_id, user_id)
);

-- ============================================
-- TYPING INDICATORS — храним в Redis, но
-- нужна таблица конфигурации last_seen privacy
-- ============================================
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS last_seen_privacy  VARCHAR(10) DEFAULT 'everyone', -- everyone, contacts, nobody
    ADD COLUMN IF NOT EXISTS online_privacy     VARCHAR(10) DEFAULT 'everyone';

-- ============================================
-- ИНДЕКСЫ
-- ============================================
CREATE INDEX IF NOT EXISTS idx_messages_type ON messages(chat_id, type);
CREATE INDEX IF NOT EXISTS idx_messages_spoiler ON messages(chat_id) WHERE is_spoiler = TRUE;
CREATE INDEX IF NOT EXISTS idx_deleted_msgs_user ON deleted_messages(user_id);

-- ============================================
-- ТРИГГЕР: автоудаление истёкших сообщений
-- (запускается через pg_cron или cron job в Go)
-- ============================================
CREATE OR REPLACE FUNCTION delete_expired_messages()
RETURNS void AS $$
BEGIN
    -- Помечаем как удалённые (soft delete)
    UPDATE messages
    SET is_deleted = TRUE, updated_at = NOW()
    WHERE expires_at IS NOT NULL
      AND expires_at < NOW()
      AND is_deleted = FALSE;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP FUNCTION IF EXISTS delete_expired_messages();
DROP INDEX IF EXISTS idx_deleted_msgs_user;
DROP INDEX IF EXISTS idx_messages_spoiler;
DROP INDEX IF EXISTS idx_messages_type;
ALTER TABLE users DROP COLUMN IF EXISTS last_seen_privacy, DROP COLUMN IF EXISTS online_privacy;
DROP TABLE IF EXISTS deleted_messages CASCADE;
ALTER TABLE messages
    DROP COLUMN IF EXISTS forward_sender_id,
    DROP COLUMN IF EXISTS forward_chat_id,
    DROP COLUMN IF EXISTS forward_date,
    DROP COLUMN IF EXISTS quoted_text,
    DROP COLUMN IF EXISTS quoted_offset,
    DROP COLUMN IF EXISTS quoted_length,
    DROP COLUMN IF EXISTS is_spoiler,
    DROP COLUMN IF EXISTS format;
DROP TABLE IF EXISTS quick_replies CASCADE;
DROP TABLE IF EXISTS message_reminders CASCADE;
DROP TABLE IF EXISTS saved_messages CASCADE;
DROP TABLE IF EXISTS pinned_messages CASCADE;
-- +goose StatementEnd