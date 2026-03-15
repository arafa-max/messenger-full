-- ============================================
-- ФИНАЛЬНАЯ СХЕМА ДЛЯ SQLC
-- НЕ применять к БД — только для sqlc generate
-- Для применения к БД используй goose
-- ============================================

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- ============================================
-- ПОЛЬЗОВАТЕЛИ (001 + 002 + 003)
-- ============================================
CREATE TABLE users (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username            VARCHAR(32) UNIQUE NOT NULL,
    phone               VARCHAR(20) UNIQUE,
    email               VARCHAR(255) UNIQUE,
    password            TEXT NOT NULL,
    avatar_url          TEXT,
    bio                 TEXT,
    is_online           BOOLEAN DEFAULT FALSE,
    last_seen           TIMESTAMPTZ DEFAULT NOW(),
    is_verified         BOOLEAN DEFAULT FALSE,
    is_bot              BOOLEAN DEFAULT FALSE,
    is_deleted          BOOLEAN DEFAULT FALSE,
    delete_at           TIMESTAMPTZ,
    language            VARCHAR(10) DEFAULT 'ru',
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    metadata            JSONB DEFAULT '{}',
    totp_secret         TEXT,
    totp_pending        TEXT,
    totp_backup         TEXT[],
    is_premium          BOOLEAN DEFAULT FALSE,
    premium_until       TIMESTAMPTZ,
    last_seen_privacy   VARCHAR(10) DEFAULT 'everyone',
    online_privacy      VARCHAR(10) DEFAULT 'everyone'
);

-- ============================================
-- УСТРОЙСТВА (001)
-- ============================================
CREATE TABLE devices (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    push_token  TEXT,
    platform    VARCHAR(10) NOT NULL,
    device_name VARCHAR(100),
    last_active TIMESTAMPTZ DEFAULT NOW(),
    is_active   BOOLEAN DEFAULT TRUE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    metadata    JSONB DEFAULT '{}'
);

-- ============================================
-- СЕССИИ (001 + 002)
-- ============================================
CREATE TABLE sessions (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id     UUID REFERENCES devices(id) ON DELETE SET NULL,
    refresh_token TEXT UNIQUE NOT NULL,
    ip_address    INET,
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    user_agent    TEXT,
    fingerprint   TEXT,
    revoked_at    TIMESTAMPTZ
);

-- ============================================
-- КОНТАКТЫ (001)
-- ============================================
CREATE TABLE contacts (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nickname   VARCHAR(64),
    is_blocked BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, contact_id)
);

-- ============================================
-- ЧАТЫ (001)
-- ============================================
CREATE TABLE chats (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    type         VARCHAR(10) NOT NULL,
    name         VARCHAR(128),
    username     VARCHAR(32) UNIQUE,
    avatar_url   TEXT,
    description  TEXT,
    owner_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    is_public    BOOLEAN DEFAULT FALSE,
    is_deleted   BOOLEAN DEFAULT FALSE,
    slow_mode    INT DEFAULT 0,
    member_count INT DEFAULT 0,
    invite_link  TEXT UNIQUE,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW(),
    metadata     JSONB DEFAULT '{}'
);

-- ============================================
-- УЧАСТНИКИ ЧАТОВ (001 + 008)
-- ============================================
CREATE TABLE chat_members (
    chat_id      UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role         VARCHAR(10) DEFAULT 'member',
    muted_until  TIMESTAMPTZ,
    is_banned    BOOLEAN DEFAULT FALSE,
    last_read_at TIMESTAMPTZ DEFAULT NOW(),
    joined_at    TIMESTAMPTZ DEFAULT NOW(),
    metadata     JSONB DEFAULT '{}',
    is_archived  BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (chat_id, user_id)
);

-- ============================================
-- МЕДИА (005)
-- ============================================
CREATE TYPE media_type AS ENUM ('image','video','audio','file','voice','sticker','gif');
CREATE TYPE media_status AS ENUM ('pending','uploaded','processed','failed');

CREATE TABLE media (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    uploader_id   UUID NOT NULL REFERENCES users(id) ON DELETE SET NULL,
    type          media_type NOT NULL,
    status        media_status NOT NULL DEFAULT 'pending',
    bucket        TEXT NOT NULL,
    object_key    TEXT NOT NULL,
    original_name TEXT,
    mime_type     TEXT NOT NULL,
    size_bytes    BIGINT,
    width         INT,
    height        INT,
    duration_sec  FLOAT,
    thumb_key     TEXT,
    waveform      JSONB,
    blur_hash     TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at    TIMESTAMPTZ,
    UNIQUE(bucket, object_key)
);

-- ============================================
-- СООБЩЕНИЯ — финальная схема из 019
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
    media_id          UUID REFERENCES media(id) ON DELETE SET NULL,
    format            VARCHAR(20) DEFAULT 'plain',
    is_spoiler        BOOLEAN DEFAULT FALSE,
    quoted_text       TEXT,
    quoted_offset     INT,
    quoted_length     INT,
    forward_sender_id UUID REFERENCES users(id) ON DELETE SET NULL,
    forward_chat_id   UUID REFERENCES chats(id) ON DELETE SET NULL,
    forward_date      TIMESTAMPTZ,
    geo               JSONB,
    link_preview      JSONB,
    PRIMARY KEY (id, created_at)
);

-- ============================================
-- СТАТУСЫ ДОСТАВКИ (019)
-- ============================================
CREATE TABLE message_status (
    message_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status     VARCHAR(10) NOT NULL DEFAULT 'delivered',
    PRIMARY KEY (message_id, created_at, user_id)
);

-- ============================================
-- РЕАКЦИИ (019)
-- ============================================
CREATE TABLE reactions (
    message_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji      VARCHAR(10) NOT NULL,
    reacted_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (message_id, created_at, user_id, emoji)
);

-- ============================================
-- ОПРОСЫ (001)
-- ============================================
CREATE TABLE polls (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    question   TEXT NOT NULL,
    options    JSONB NOT NULL DEFAULT '[]',
    votes      JSONB NOT NULL DEFAULT '{}',
    is_closed  BOOLEAN DEFAULT FALSE
);

CREATE TABLE poll_options (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    poll_id     UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    text        TEXT NOT NULL,
    votes_count INT DEFAULT 0,
    position    INT NOT NULL
);

CREATE TABLE poll_votes (
    poll_id    UUID NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
    option_id  UUID NOT NULL REFERENCES poll_options(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (poll_id, user_id, option_id)
);

-- ============================================
-- ИСТОРИЯ РЕДАКТИРОВАНИЯ (002 + 019)
-- ============================================
CREATE TABLE message_edits (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    old_content TEXT NOT NULL,
    edited_at   TIMESTAMPTZ DEFAULT NOW(),
    edit_number INT NOT NULL DEFAULT 1
);

-- ============================================
-- ЗАКРЕПЛЁННЫЕ СООБЩЕНИЯ (003 + 019)
-- ============================================
CREATE TABLE pinned_messages (
    chat_id    UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    message_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    pinned_by  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pinned_at  TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (chat_id, message_id, created_at)
);

-- ============================================
-- ИЗБРАННЫЕ СООБЩЕНИЯ (003 + 019)
-- ============================================
CREATE TABLE saved_messages (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    saved_at   TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (user_id, message_id, created_at)
);

-- ============================================
-- НАПОМИНАНИЯ (003 + 019)
-- ============================================
CREATE TABLE message_reminders (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    remind_at  TIMESTAMPTZ NOT NULL,
    is_sent    BOOLEAN DEFAULT FALSE,
    UNIQUE (user_id, message_id, created_at)
);

-- ============================================
-- УДАЛЁННЫЕ У СЕБЯ (003 + 019)
-- ============================================
CREATE TABLE deleted_messages (
    message_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    deleted_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (message_id, created_at, user_id)
);

-- ============================================
-- БЫСТРЫЕ ОТВЕТЫ (003)
-- ============================================
CREATE TABLE quick_replies (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    shortcut   VARCHAR(32) NOT NULL,
    text       TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (user_id, shortcut)
);

-- ============================================
-- МЕДИА ПОИСКОВЫЙ ИНДЕКС (018 + 019)
-- ============================================
CREATE TABLE media_search_index (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    media_id   UUID NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    message_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    content    TEXT NOT NULL,
    source     VARCHAR(20) NOT NULL,
    lang       VARCHAR(10) DEFAULT 'ru',
    indexed_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- PASSKEYS (002)
-- ============================================
CREATE TABLE passkeys (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id TEXT UNIQUE NOT NULL,
    public_key    BYTEA NOT NULL,
    aaguid        UUID,
    sign_count    BIGINT DEFAULT 0,
    device_type   VARCHAR(32),
    backed_up     BOOLEAN DEFAULT FALSE,
    transports    TEXT[],
    name          VARCHAR(100),
    last_used_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- E2EE КЛЮЧИ (002)
-- ============================================
CREATE TABLE identity_keys (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       UUID REFERENCES devices(id) ON DELETE CASCADE,
    public_key      TEXT NOT NULL,
    registration_id INT NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, device_id)
);

CREATE TABLE signed_prekeys (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id  UUID REFERENCES devices(id) ON DELETE CASCADE,
    key_id     INT NOT NULL,
    public_key TEXT NOT NULL,
    signature  TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, device_id, key_id)
);

CREATE TABLE one_time_prekeys (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id  UUID REFERENCES devices(id) ON DELETE CASCADE,
    key_id     INT NOT NULL,
    public_key TEXT NOT NULL,
    is_used    BOOLEAN DEFAULT FALSE,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE key_bundles (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id  UUID REFERENCES devices(id) ON DELETE CASCADE,
    bundle     JSONB NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, device_id)
);

-- ============================================
-- LINK PREVIEWS (002)
-- ============================================
CREATE TABLE link_previews (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    url         TEXT UNIQUE NOT NULL,
    title       TEXT,
    description TEXT,
    image_url   TEXT,
    site_name   TEXT,
    fetched_at  TIMESTAMPTZ DEFAULT NOW(),
    expires_at  TIMESTAMPTZ DEFAULT NOW() + INTERVAL '24 hours'
);

-- ============================================
-- AUDIT LOG (002)
-- ============================================
CREATE TABLE audit_log (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    actor_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    action      VARCHAR(64) NOT NULL,
    target_type VARCHAR(32),
    target_id   UUID,
    ip_address  INET,
    user_agent  TEXT,
    metadata    JSONB DEFAULT '{}',
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- RATE LIMITS (002)
-- ============================================
CREATE TABLE rate_limits (
    key          TEXT NOT NULL,
    count        INT DEFAULT 1,
    window_start TIMESTAMPTZ NOT NULL,
    PRIMARY KEY(key, window_start)
);

-- ============================================
-- CHAT TOPICS (002)
-- ============================================
CREATE TABLE chat_topics (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    chat_id       UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    name          VARCHAR(128) NOT NULL,
    icon_emoji    VARCHAR(10),
    icon_color    VARCHAR(7),
    created_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    is_closed     BOOLEAN DEFAULT FALSE,
    is_hidden     BOOLEAN DEFAULT FALSE,
    message_count INT DEFAULT 0,
    created_at    TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- VOICE ROOMS (002)
-- ============================================
CREATE TABLE voice_rooms (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    chat_id    UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    name       VARCHAR(100) NOT NULL,
    type       VARCHAR(20) DEFAULT 'voice',
    user_limit INT DEFAULT 0,
    is_active  BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE voice_room_participants (
    room_id     UUID NOT NULL REFERENCES voice_rooms(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    is_muted    BOOLEAN DEFAULT FALSE,
    is_deafened BOOLEAN DEFAULT FALSE,
    is_video    BOOLEAN DEFAULT FALSE,
    is_speaking BOOLEAN DEFAULT FALSE,
    joined_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (room_id, user_id)
);

-- ============================================
-- ПОДПИСКИ — финальная схема (002 + 021)
-- ============================================
CREATE TABLE subscriptions (
    id                   UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan                 TEXT NOT NULL DEFAULT 'free',
    status               TEXT NOT NULL DEFAULT 'inactive',
    started_at           TIMESTAMPTZ DEFAULT NOW(),
    expires_at           TIMESTAMPTZ,
    auto_renew           BOOLEAN DEFAULT TRUE,
    payment_ref          TEXT,
    metadata             JSONB DEFAULT '{}',
    stripe_customer_id   TEXT NOT NULL DEFAULT '',
    stripe_sub_id        TEXT NOT NULL DEFAULT '',
    current_period_end   TIMESTAMPTZ,
    cancel_at_period_end BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================
-- LIVE LOCATION (002)
-- ============================================
CREATE TABLE live_locations (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id    UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    message_id UUID,
    latitude   DOUBLE PRECISION NOT NULL,
    longitude  DOUBLE PRECISION NOT NULL,
    heading    INT,
    accuracy   FLOAT,
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- INVITE LINKS (002)
-- ============================================
CREATE TABLE invite_links (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code       VARCHAR(32) UNIQUE NOT NULL,
    chat_id    UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    max_uses   INT DEFAULT 0,
    uses_count INT DEFAULT 0,
    expires_at TIMESTAMPTZ,
    is_revoked BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- CHANNEL ANALYTICS (002)
-- ============================================
CREATE TABLE channel_stats (
    channel_id   UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    date         DATE NOT NULL,
    views        INT DEFAULT 0,
    shares       INT DEFAULT 0,
    new_members  INT DEFAULT 0,
    left_members INT DEFAULT 0,
    PRIMARY KEY (channel_id, date)
);

-- ============================================
-- ЗВОНКИ (001)
-- ============================================
CREATE TABLE calls (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    chat_id      UUID REFERENCES chats(id) ON DELETE SET NULL,
    initiator_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type         VARCHAR(10) NOT NULL,
    status       VARCHAR(10) NOT NULL,
    started_at   TIMESTAMPTZ,
    ended_at     TIMESTAMPTZ,
    duration     INT,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    metadata     JSONB DEFAULT '{}'
);

CREATE TABLE call_participants (
    call_id      UUID NOT NULL REFERENCES calls(id) ON DELETE CASCADE,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at    TIMESTAMPTZ,
    left_at      TIMESTAMPTZ,
    is_muted     BOOLEAN DEFAULT FALSE,
    is_video_off BOOLEAN DEFAULT FALSE,
    PRIMARY KEY (call_id, user_id)
);

-- ============================================
-- STORIES (001 + 010)
-- ============================================
CREATE TABLE stories (
    id            UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_url     TEXT NOT NULL,
    thumbnail_url TEXT,
    type          VARCHAR(10) DEFAULT 'image',
    caption       TEXT,
    views_count   INT DEFAULT 0,
    expires_at    TIMESTAMPTZ DEFAULT NOW() + INTERVAL '24 hours',
    created_at    TIMESTAMPTZ DEFAULT NOW(),
    metadata      JSONB DEFAULT '{}',
    is_archived   BOOLEAN DEFAULT FALSE,
    audience      VARCHAR(20) DEFAULT 'everyone',
    media_id      UUID REFERENCES media(id) ON DELETE SET NULL,
    sticker_data  JSONB,
    music_data    JSONB
);

CREATE TABLE story_views (
    story_id  UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    user_id   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    viewed_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (story_id, user_id)
);

CREATE TABLE story_reactions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    story_id   UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji      VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (story_id, user_id)
);

CREATE TABLE close_friends (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    friend_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, friend_id)
);

CREATE TABLE story_archive (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_id    UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================
-- ПАПКИ ЧАТОВ (001)
-- ============================================
CREATE TABLE chat_folders (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       VARCHAR(64) NOT NULL,
    emoji      VARCHAR(10),
    position   INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE chat_folder_items (
    folder_id UUID NOT NULL REFERENCES chat_folders(id) ON DELETE CASCADE,
    chat_id   UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    PRIMARY KEY (folder_id, chat_id)
);

-- ============================================
-- СТИКЕРЫ (001 + 009 + 011)
-- ============================================
CREATE TABLE sticker_packs (
    id          UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name        VARCHAR(64) NOT NULL,
    author_id   UUID REFERENCES users(id) ON DELETE SET NULL,
    is_official BOOLEAN DEFAULT FALSE,
    created_at  TIMESTAMPTZ DEFAULT NOW(),
    thumb_url   TEXT
);

CREATE TABLE stickers (
    id         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    pack_id    UUID NOT NULL REFERENCES sticker_packs(id) ON DELETE CASCADE,
    emoji      VARCHAR(10),
    url        TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    media_id   UUID REFERENCES media(id) ON DELETE CASCADE,
    position   INT DEFAULT 0,
    format     VARCHAR(10) DEFAULT 'static',
    lottie_url TEXT
);

CREATE TABLE user_sticker_packs (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    pack_id    UUID NOT NULL REFERENCES sticker_packs(id) ON DELETE CASCADE,
    added_at   TIMESTAMPTZ DEFAULT NOW(),
    position   INT DEFAULT 0,
    PRIMARY KEY (user_id, pack_id)
);

-- ============================================
-- УВЕДОМЛЕНИЯ (001)
-- ============================================
CREATE TABLE notifications (
    id             UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type           VARCHAR(30) NOT NULL,
    title          TEXT,
    body           TEXT,
    is_read        BOOLEAN DEFAULT FALSE,
    reference_id   UUID,
    reference_type VARCHAR(20),
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    metadata       JSONB DEFAULT '{}'
);

-- ============================================
-- EMOJI KEYWORDS (012)
-- ============================================
CREATE TABLE emoji_keywords (
    id      SERIAL PRIMARY KEY,
    keyword VARCHAR(64) NOT NULL,
    emoji   VARCHAR(10) NOT NULL,
    lang    VARCHAR(5) DEFAULT 'ru',
    weight  INT DEFAULT 1
);

-- ============================================
-- ANIMATED EMOJI (013)
-- ============================================
CREATE TABLE animated_emoji (
    id         SERIAL PRIMARY KEY,
    emoji      VARCHAR(10) NOT NULL UNIQUE,
    object_key TEXT NOT NULL,
    bucket     TEXT NOT NULL DEFAULT 'animated-emoji',
    width      INT DEFAULT 512,
    height     INT DEFAULT 512,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- ============================================
-- AI REQUESTS (014)
-- ============================================
CREATE TABLE ai_requests (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    request_type TEXT NOT NULL,
    tokens_used  INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================
-- БОТЫ — финальная схема (015 + 016)
-- ============================================
CREATE TABLE bots (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token          TEXT NOT NULL UNIQUE,
    username       TEXT NOT NULL DEFAULT '',
    name           TEXT NOT NULL DEFAULT '',
    description    TEXT NOT NULL DEFAULT '',
    avatar_url     TEXT NOT NULL DEFAULT '',
    is_active      BOOL NOT NULL DEFAULT TRUE,
    is_ai_enabled  BOOL NOT NULL DEFAULT FALSE,
    webhook_url    TEXT NOT NULL DEFAULT '',
    webhook_secret TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE bot_commands (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id      UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    command     TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    UNIQUE(bot_id, command)
);

CREATE TABLE bot_updates (
    id         BIGSERIAL PRIMARY KEY,
    bot_id     UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    update_id  BIGINT NOT NULL,
    type       TEXT NOT NULL DEFAULT 'message',
    payload    JSONB NOT NULL DEFAULT '{}',
    processed  BOOL NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(bot_id, update_id)
);

-- ============================================
-- ПЛАТЕЖИ (017)
-- ============================================
CREATE TABLE payments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    bot_id      UUID NOT NULL REFERENCES bots(id) ON DELETE CASCADE,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount      BIGINT NOT NULL,
    currency    TEXT NOT NULL DEFAULT 'usd',
    status      TEXT NOT NULL DEFAULT 'pending',
    stripe_id   TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    metadata    JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================
-- SOCIAL RECOVERY (020)
-- ============================================
CREATE TABLE recovery_sessions (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    threshold        INT NOT NULL,
    total_shares     INT NOT NULL,
    status           VARCHAR(20) DEFAULT 'pending',
    encrypted_secret TEXT NOT NULL,
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    expires_at       TIMESTAMPTZ DEFAULT NOW() + INTERVAL '30 days'
);

CREATE TABLE recovery_shares (
    id           UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id   UUID NOT NULL REFERENCES recovery_sessions(id) ON DELETE CASCADE,
    guardian_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    share_index  INT NOT NULL,
    share_data   TEXT NOT NULL,
    is_submitted BOOLEAN DEFAULT FALSE,
    submitted_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (session_id, guardian_id),
    UNIQUE (session_id, share_index)
);

CREATE TABLE recovery_requests (
    id               UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id       UUID NOT NULL REFERENCES recovery_sessions(id) ON DELETE CASCADE,
    requester_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status           VARCHAR(20) DEFAULT 'pending',
    collected_shares JSONB DEFAULT '[]',
    created_at       TIMESTAMPTZ DEFAULT NOW(),
    expires_at       TIMESTAMPTZ DEFAULT NOW() + INTERVAL '24 hours'
);

-- ============================================
-- PREMIUM SETTINGS (021)
-- ============================================
CREATE TABLE premium_settings (
    user_id              UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    hide_phone           BOOLEAN NOT NULL DEFAULT FALSE,
    away_message         TEXT NOT NULL DEFAULT '',
    away_message_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================
-- CHAT LABELS (021)
-- ============================================
CREATE TABLE chat_labels (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id    UUID NOT NULL,
    label      TEXT NOT NULL,
    color      TEXT NOT NULL DEFAULT '#6B7280',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);