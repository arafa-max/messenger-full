-- +goose Up
-- +goose StatementBegin

-- ============================================
-- ПАРТИЦИОНИРОВАНИЕ messages ПО МЕСЯЦУ
-- messages была пересоздана вручную,
-- поэтому создаём партиционированную таблицу напрямую
-- ============================================

CREATE TABLE messages (
    id                UUID NOT NULL DEFAULT uuid_generate_v4(),
    chat_id           UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    sender_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reply_to_id       UUID,
    thread_id         UUID,
    forwarded_from_id UUID,
    type              VARCHAR(20) DEFAULT 'text',
    content           TEXT NOT NULL DEFAULT '',
    is_encrypted      BOOLEAN DEFAULT FALSE,
    is_edited         BOOLEAN DEFAULT FALSE,
    is_deleted        BOOLEAN DEFAULT FALSE,
    is_pinned         BOOLEAN DEFAULT FALSE,
    scheduled_at      TIMESTAMPTZ,
    expires_at        TIMESTAMPTZ,
    views_count       INT DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ DEFAULT NOW(),
    metadata          JSONB DEFAULT '{}',
    topic_id          UUID,
    media_id          UUID,
    format            VARCHAR(20) DEFAULT 'plain',
    is_spoiler        BOOLEAN DEFAULT FALSE,
    quoted_text       TEXT,
    quoted_offset     INT,
    quoted_length     INT,
    forward_sender_id UUID,
    forward_chat_id   UUID,
    forward_date      TIMESTAMPTZ,
    geo               JSONB,
    link_preview      JSONB,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Создаём партиции: текущий месяц + 6 вперёд
DO $$
DECLARE
    start_date DATE;
    end_date   DATE;
    part_name  TEXT;
    i          INT;
BEGIN
    FOR i IN 0..6 LOOP
        start_date := DATE_TRUNC('month', CURRENT_DATE + (i || ' months')::INTERVAL)::DATE;
        end_date   := (start_date + INTERVAL '1 month')::DATE;
        part_name  := 'messages_' || TO_CHAR(start_date, 'YYYY_MM');

        IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relname = part_name) THEN
            EXECUTE format(
                'CREATE TABLE %I PARTITION OF messages FOR VALUES FROM (%L) TO (%L)',
                part_name, start_date, end_date
            );
        END IF;
    END LOOP;
END;
$$;

-- Партиция DEFAULT для всего остального
CREATE TABLE messages_default PARTITION OF messages DEFAULT;

-- Индексы
CREATE INDEX IF NOT EXISTS idx_messages_chat      ON messages(chat_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_sender    ON messages(sender_id);
CREATE INDEX IF NOT EXISTS idx_messages_thread    ON messages(thread_id);
CREATE INDEX IF NOT EXISTS idx_messages_pinned    ON messages(chat_id, is_pinned) WHERE is_pinned = TRUE;
CREATE INDEX IF NOT EXISTS idx_messages_scheduled ON messages(scheduled_at) WHERE scheduled_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_messages_expires   ON messages(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_messages_search    ON messages USING gin(content gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_messages_media     ON messages(media_id) WHERE media_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_messages_type      ON messages(chat_id, type);

-- Восстанавливаем зависимые таблицы которые упали вместе с messages
CREATE TABLE IF NOT EXISTS message_status (
    message_id  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status      VARCHAR(10) NOT NULL DEFAULT 'delivered',
    PRIMARY KEY (message_id, created_at, user_id),
    FOREIGN KEY (message_id, created_at) REFERENCES messages(id, created_at) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS reactions (
    message_id  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji       VARCHAR(10) NOT NULL,
    reacted_at  TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (message_id, created_at, user_id, emoji),
    FOREIGN KEY (message_id, created_at) REFERENCES messages(id, created_at) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS polls (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    question    TEXT NOT NULL,
    options     JSONB NOT NULL DEFAULT '[]',
    votes       JSONB NOT NULL DEFAULT '{}',
    is_closed   BOOLEAN DEFAULT FALSE,
    FOREIGN KEY (message_id, created_at) REFERENCES messages(id, created_at) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS message_edits (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    old_content TEXT NOT NULL,
    edited_at   TIMESTAMPTZ DEFAULT NOW(),
    edit_number INT NOT NULL DEFAULT 1,
    FOREIGN KEY (message_id, created_at) REFERENCES messages(id, created_at) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS pinned_messages (
    chat_id     UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    message_id  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    pinned_by   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pinned_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (chat_id, message_id, created_at),
    FOREIGN KEY (message_id, created_at) REFERENCES messages(id, created_at) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS saved_messages (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    saved_at    TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (user_id, message_id, created_at),
    FOREIGN KEY (message_id, created_at) REFERENCES messages(id, created_at) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS message_reminders (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    remind_at   TIMESTAMPTZ NOT NULL,
    is_sent     BOOLEAN DEFAULT FALSE,
    UNIQUE (user_id, message_id, created_at),
    FOREIGN KEY (message_id, created_at) REFERENCES messages(id, created_at) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS deleted_messages (
    message_id  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deleted_at  TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (message_id, created_at, user_id),
    FOREIGN KEY (message_id, created_at) REFERENCES messages(id, created_at) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS media_search_index (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    media_id    UUID NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    message_id  UUID,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    content     TEXT NOT NULL,
    source      VARCHAR(20) NOT NULL,
    lang        VARCHAR(10) DEFAULT 'ru',
    indexed_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Индексы для восстановленных таблиц
CREATE INDEX IF NOT EXISTS idx_message_edits      ON message_edits(message_id, edit_number DESC);
CREATE INDEX IF NOT EXISTS idx_pinned_chat        ON pinned_messages(chat_id, pinned_at DESC);
CREATE INDEX IF NOT EXISTS idx_saved_user         ON saved_messages(user_id, saved_at DESC);
CREATE INDEX IF NOT EXISTS idx_reminders_pending  ON message_reminders(remind_at) WHERE is_sent = FALSE;
CREATE INDEX IF NOT EXISTS idx_deleted_msgs_user  ON deleted_messages(user_id);
CREATE INDEX IF NOT EXISTS idx_media_search_content ON media_search_index USING gin(content gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_media_search_media   ON media_search_index(media_id);

-- Функция автосоздания партиций (вызывается воркером каждый месяц)
CREATE OR REPLACE FUNCTION create_monthly_partition(target_date DATE DEFAULT CURRENT_DATE)
RETURNS TEXT AS $$
DECLARE
    start_date DATE := DATE_TRUNC('month', target_date)::DATE;
    end_date   DATE := (start_date + INTERVAL '1 month')::DATE;
    part_name  TEXT := 'messages_' || TO_CHAR(start_date, 'YYYY_MM');
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_class WHERE relname = part_name) THEN
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF messages FOR VALUES FROM (%L) TO (%L)',
            part_name, start_date, end_date
        );
        RETURN 'created: ' || part_name;
    END IF;
    RETURN 'exists: ' || part_name;
END;
$$ LANGUAGE plpgsql;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP FUNCTION IF EXISTS create_monthly_partition;
DROP TABLE IF EXISTS media_search_index CASCADE;
DROP TABLE IF EXISTS deleted_messages CASCADE;
DROP TABLE IF EXISTS message_reminders CASCADE;
DROP TABLE IF EXISTS saved_messages CASCADE;
DROP TABLE IF EXISTS pinned_messages CASCADE;
DROP TABLE IF EXISTS message_edits CASCADE;
DROP TABLE IF EXISTS polls CASCADE;
DROP TABLE IF EXISTS reactions CASCADE;
DROP TABLE IF EXISTS message_status CASCADE;
DROP TABLE IF EXISTS messages CASCADE;

-- +goose StatementEnd