-- +goose Up
-- +goose StatementBegin

-- ============================================
-- РАСШИРЕНИЯ
-- ============================================
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ============================================
-- ПОЛЬЗОВАТЕЛИ
-- ============================================
CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username        VARCHAR(32) UNIQUE NOT NULL,
    phone           VARCHAR(20) UNIQUE,
    email           VARCHAR(255) UNIQUE,
    password        TEXT NOT NULL,
    avatar_url      TEXT,
    bio             TEXT,
    is_online       BOOLEAN DEFAULT FALSE,
    last_seen       TIMESTAMPTZ DEFAULT NOW(),
    is_verified     BOOLEAN DEFAULT FALSE,        -- верифицированный аккаунт
    is_bot          BOOLEAN DEFAULT FALSE,        -- бот или человек
    is_deleted      BOOLEAN DEFAULT FALSE,
    delete_at       TIMESTAMPTZ,                  -- самоуничтожение аккаунта через N дней
    language        VARCHAR(10) DEFAULT 'ru',     -- язык интерфейса
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    metadata        JSONB DEFAULT '{}'            -- всё что придумаем в будущем
);

-- ============================================
-- УСТРОЙСТВА
-- ============================================
CREATE TABLE devices (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    push_token      TEXT,                         -- FCM / APNs токен
    platform        VARCHAR(10) NOT NULL,         -- ios, android, web, desktop
    device_name     VARCHAR(100),                 -- "iPhone 14 Pro", "Chrome on Windows"
    last_active     TIMESTAMPTZ DEFAULT NOW(),
    is_active       BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    metadata        JSONB DEFAULT '{}'
);

-- ============================================
-- СЕССИИ
-- ============================================
CREATE TABLE sessions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       UUID REFERENCES devices(id) ON DELETE SET NULL,
    refresh_token   TEXT UNIQUE NOT NULL,
    ip_address      INET,
    expires_at      TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- КОНТАКТЫ И БЛОКИРОВКИ
-- ============================================
CREATE TABLE contacts (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nickname        VARCHAR(64),                  -- своё имя для контакта
    is_blocked      BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, contact_id)
);

-- ============================================
-- ЧАТЫ
-- ============================================
CREATE TABLE chats (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type            VARCHAR(10) NOT NULL,         -- private, group, channel, bot
    name            VARCHAR(128),
    username        VARCHAR(32) UNIQUE,           -- @публичное_имя для каналов
    avatar_url      TEXT,
    description     TEXT,
    owner_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    is_public       BOOLEAN DEFAULT FALSE,
    is_deleted      BOOLEAN DEFAULT FALSE,
    slow_mode       INT DEFAULT 0,                -- секунды между сообщениями
    member_count    INT DEFAULT 0,
    invite_link     TEXT UNIQUE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    metadata        JSONB DEFAULT '{}'
);

-- ============================================
-- УЧАСТНИКИ ЧАТОВ
-- ============================================
CREATE TABLE chat_members (
    chat_id         UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            VARCHAR(10) DEFAULT 'member', -- owner, admin, member, guest
    muted_until     TIMESTAMPTZ,
    is_banned       BOOLEAN DEFAULT FALSE,
    last_read_at    TIMESTAMPTZ DEFAULT NOW(),
    joined_at       TIMESTAMPTZ DEFAULT NOW(),
    metadata        JSONB DEFAULT '{}',
    PRIMARY KEY (chat_id, user_id)
);

-- ============================================
-- СООБЩЕНИЯ
-- ============================================
CREATE TABLE messages (
    id                UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    chat_id           UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    sender_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reply_to_id       UUID REFERENCES messages(id) ON DELETE SET NULL,
    thread_id         UUID REFERENCES messages(id) ON DELETE SET NULL,
    forwarded_from_id UUID REFERENCES messages(id) ON DELETE SET NULL,
    type              VARCHAR(20) DEFAULT 'text', -- text, image, video, audio, file, geo, sticker, gif, poll, system
    content           TEXT,
    is_encrypted      BOOLEAN DEFAULT FALSE,
    is_edited         BOOLEAN DEFAULT FALSE,
    is_deleted        BOOLEAN DEFAULT FALSE,
    is_pinned         BOOLEAN DEFAULT FALSE,
    scheduled_at      TIMESTAMPTZ,               -- отложенная отправка
    expires_at        TIMESTAMPTZ,               -- исчезающие сообщения
    views_count       INT DEFAULT 0,             -- для каналов
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    updated_at        TIMESTAMPTZ DEFAULT NOW(),
    metadata          JSONB DEFAULT '{}'         -- превью ссылки, геолокация и т.д.
);

-- ============================================
-- СТАТУСЫ ДОСТАВКИ
-- ============================================
CREATE TABLE message_status (
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    delivered       BOOLEAN DEFAULT FALSE,
    read            BOOLEAN DEFAULT FALSE,
    delivered_at    TIMESTAMPTZ,
    read_at         TIMESTAMPTZ,
    PRIMARY KEY (message_id, user_id)
);


-- ============================================
-- РЕАКЦИИ
-- ============================================
CREATE TABLE reactions (
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji           VARCHAR(10) NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (message_id, user_id, emoji)
);

-- ============================================
-- ОПРОСЫ
-- ============================================
CREATE TABLE polls (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    question        TEXT NOT NULL,
    is_anonymous    BOOLEAN DEFAULT TRUE,
    is_multiple     BOOLEAN DEFAULT FALSE,
    is_quiz         BOOLEAN DEFAULT FALSE,
    correct_option  INT,
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE poll_options (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    poll_id         UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    text            TEXT NOT NULL,
    votes_count     INT DEFAULT 0,
    position        INT NOT NULL
);

CREATE TABLE poll_votes (
    poll_id         UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id       UUID NOT NULL REFERENCES poll_options(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (poll_id, user_id, option_id)
);

-- ============================================
-- ЗВОНКИ (P2P WebRTC)
-- ============================================
CREATE TABLE calls (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    chat_id         UUID REFERENCES chats(id) ON DELETE SET NULL,
    initiator_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type            VARCHAR(10) NOT NULL,         -- audio, video
    status          VARCHAR(10) NOT NULL,         -- ringing, active, ended, missed, rejected, busy
    started_at      TIMESTAMPTZ,
    ended_at        TIMESTAMPTZ,
    duration        INT,                          -- секунды
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    metadata        JSONB DEFAULT '{}'            -- WebRTC SDP, ICE candidates
);

CREATE TABLE call_participants (
    call_id         UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at       TIMESTAMPTZ,
    left_at         TIMESTAMPTZ,
    is_muted        BOOLEAN DEFAULT FALSE,
    is_video_off    BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (call_id, user_id)
);

-- ============================================
-- STORIES
-- ============================================
CREATE TABLE stories (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_url       TEXT NOT NULL,
    thumbnail_url   TEXT,
    type            VARCHAR(10) DEFAULT 'image',  -- image, video
    caption         TEXT,
    views_count     INT DEFAULT 0,
    expires_at      TIMESTAMPTZ DEFAULT NOW() + INTERVAL '24 hours',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    metadata        JSONB DEFAULT '{}'
);

CREATE TABLE story_views (
    story_id        UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    viewed_at       TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (story_id, user_id)
);

-- ============================================
-- E2EE КЛЮЧИ
-- ============================================
CREATE TABLE encryption_keys (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       UUID REFERENCES devices(id) ON DELETE CASCADE,
    public_key      TEXT NOT NULL,
    key_type        VARCHAR(20) DEFAULT 'identity', -- identity, prekey, onetime
    is_used         BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- БОТЫ
-- ============================================
CREATE TABLE bots (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    owner_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token           TEXT UNIQUE NOT NULL,
    webhook_url     TEXT,
    commands        JSONB DEFAULT '[]',
    is_active       BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    metadata        JSONB DEFAULT '{}'
);

-- ============================================
-- ПАПКИ ЧАТОВ
-- ============================================
CREATE TABLE chat_folders (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            VARCHAR(64) NOT NULL,
    emoji           VARCHAR(10),
    position        INT DEFAULT 0,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE chat_folder_items (
    folder_id       UUID NOT NULL REFERENCES chat_folders(id) ON DELETE CASCADE,
    chat_id         UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    PRIMARY KEY (folder_id, chat_id)
);

-- ============================================
-- СТИКЕРЫ
-- ============================================
CREATE TABLE sticker_packs (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            VARCHAR(64) NOT NULL,
    author_id       UUID REFERENCES users(id) ON DELETE SET NULL,
    is_official     BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE stickers (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pack_id         UUID NOT NULL REFERENCES sticker_packs(id) ON DELETE CASCADE,
    emoji           VARCHAR(10),
    url             TEXT NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE user_sticker_packs (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pack_id         UUID NOT NULL REFERENCES sticker_packs(id) ON DELETE CASCADE,
    added_at        TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, pack_id)
);

-- ============================================
-- УВЕДОМЛЕНИЯ
-- ============================================
CREATE TABLE notifications (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type            VARCHAR(30) NOT NULL,         -- message, call, mention, reaction, system
    title           TEXT,
    body            TEXT,
    is_read         BOOLEAN DEFAULT FALSE,
    reference_id    UUID,
    reference_type  VARCHAR(20),                  -- message, call, chat
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    metadata        JSONB DEFAULT '{}'
);

-- ============================================
-- ИНДЕКСЫ
-- ============================================

-- Пользователи
CREATE INDEX idx_users_username_search ON users USING gin(username gin_trgm_ops);
CREATE INDEX idx_users_phone ON users(phone);
CREATE INDEX idx_users_online ON users(is_online, last_seen);

-- Сообщения
CREATE INDEX idx_messages_chat ON messages(chat_id, created_at DESC);
CREATE INDEX idx_messages_sender ON messages(sender_id);
CREATE INDEX idx_messages_thread ON messages(thread_id);
CREATE INDEX idx_messages_pinned ON messages(chat_id, is_pinned) WHERE is_pinned = TRUE;
CREATE INDEX idx_messages_scheduled ON messages(scheduled_at) WHERE scheduled_at IS NOT NULL;
CREATE INDEX idx_messages_expires ON messages(expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX idx_messages_search ON messages USING gin(content gin_trgm_ops);

-- Чаты
CREATE INDEX idx_chat_members_user ON chat_members(user_id);
CREATE INDEX idx_chats_username ON chats(username) WHERE username IS NOT NULL;

-- Сессии
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_sessions_token ON sessions(refresh_token);
CREATE INDEX idx_sessions_expires ON sessions(expires_at);

-- Устройства
CREATE INDEX idx_devices_user ON devices(user_id);

-- Уведомления
CREATE INDEX idx_notifications_user ON notifications(user_id, is_read, created_at DESC);

-- Stories
CREATE INDEX idx_stories_active ON stories(user_id, expires_at);

-- Звонки
CREATE INDEX idx_calls_chat ON calls(chat_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS user_sticker_packs CASCADE;
DROP TABLE IF EXISTS stickers CASCADE;
DROP TABLE IF EXISTS sticker_packs CASCADE;
DROP TABLE IF EXISTS chat_folder_items CASCADE;
DROP TABLE IF EXISTS chat_folders CASCADE;
DROP TABLE IF EXISTS bots CASCADE;
DROP TABLE IF EXISTS encryption_keys CASCADE;
DROP TABLE IF EXISTS story_views CASCADE;
DROP TABLE IF EXISTS stories CASCADE;
DROP TABLE IF EXISTS call_participants CASCADE;
DROP TABLE IF EXISTS calls CASCADE;
DROP TABLE IF EXISTS poll_votes CASCADE;
DROP TABLE IF EXISTS poll_options CASCADE;
DROP TABLE IF EXISTS polls CASCADE;
DROP TABLE IF EXISTS reactions CASCADE;
DROP TABLE IF EXISTS media CASCADE;
DROP TABLE IF EXISTS message_status CASCADE;
DROP TABLE IF EXISTS messages CASCADE;
DROP TABLE IF EXISTS chat_members CASCADE;
DROP TABLE IF EXISTS chats CASCADE;
DROP TABLE IF EXISTS contacts CASCADE;
DROP TABLE IF EXISTS notifications CASCADE;
DROP TABLE IF EXISTS sessions CASCADE;
DROP TABLE IF EXISTS devices CASCADE;
DROP TABLE IF EXISTS users CASCADE;

-- +goose StatementEnd