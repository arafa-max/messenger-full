-- +goose Up
-- +goose StatementBegin

-- ============================================
-- 2FA поля в users
-- ============================================
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS totp_secret  TEXT,
    ADD COLUMN IF NOT EXISTS totp_pending TEXT,
    ADD COLUMN IF NOT EXISTS totp_backup  TEXT[]; -- hashed backup codes

-- ============================================
-- SESSION: fingerprint + revocation
-- ============================================
ALTER TABLE sessions
    ADD COLUMN IF NOT EXISTS user_agent    TEXT,
    ADD COLUMN IF NOT EXISTS fingerprint   TEXT,
    ADD COLUMN IF NOT EXISTS revoked_at    TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_sessions_revoked ON sessions(revoked_at) WHERE revoked_at IS NULL;

-- ============================================
-- PASSKEYS (WebAuthn / FIDO2)
-- ============================================
CREATE TABLE IF NOT EXISTS passkeys (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id   TEXT UNIQUE NOT NULL,       -- base64url encoded
    public_key      BYTEA NOT NULL,             -- COSE public key
    aaguid          UUID,                       -- authenticator type
    sign_count      BIGINT DEFAULT 0,
    device_type     VARCHAR(32),                -- single-device, multi-device
    backed_up       BOOLEAN DEFAULT FALSE,
    transports      TEXT[],                     -- usb, nfc, ble, internal
    name            VARCHAR(100),               -- "Face ID on iPhone 15"
    last_used_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_passkeys_user ON passkeys(user_id);

-- ============================================
-- E2EE — Signal Protocol key tables
-- ============================================

-- Identity keys (long-term, per device)
CREATE TABLE IF NOT EXISTS identity_keys (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       UUID REFERENCES devices(id) ON DELETE CASCADE,
    public_key      TEXT NOT NULL,              -- base64 encoded Ed25519
    registration_id INT NOT NULL,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, device_id)
);

-- Signed PreKeys (rotate weekly)
CREATE TABLE IF NOT EXISTS signed_prekeys (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       UUID REFERENCES devices(id) ON DELETE CASCADE,
    key_id          INT NOT NULL,
    public_key      TEXT NOT NULL,
    signature       TEXT NOT NULL,              -- signed with identity key
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_id, device_id, key_id)
);

-- One-Time PreKeys (X3DH — used once then deleted)
CREATE TABLE IF NOT EXISTS one_time_prekeys (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       UUID REFERENCES devices(id) ON DELETE CASCADE,
    key_id          INT NOT NULL,
    public_key      TEXT NOT NULL,
    is_used         BOOLEAN DEFAULT FALSE,
    used_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_otpk_available ON one_time_prekeys(user_id, device_id) WHERE is_used = FALSE;

-- Key Bundle cache (what we give to senders)
CREATE TABLE IF NOT EXISTS key_bundles (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id       UUID REFERENCES devices(id) ON DELETE CASCADE,
    bundle          JSONB NOT NULL,             -- {identity_key, signed_prekey, one_time_prekey}
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, device_id)
);

-- ============================================
-- MESSAGE EDIT HISTORY
-- ============================================
CREATE TABLE IF NOT EXISTS message_edits (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    old_content     TEXT NOT NULL,
    edited_at       TIMESTAMPTZ DEFAULT NOW(),
    edit_number     INT NOT NULL DEFAULT 1
);
CREATE INDEX IF NOT EXISTS idx_message_edits ON message_edits(message_id, edit_number DESC);

-- ============================================
-- LINK PREVIEWS (cache)
-- ============================================
CREATE TABLE IF NOT EXISTS link_previews (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    url             TEXT UNIQUE NOT NULL,
    title           TEXT,
    description     TEXT,
    image_url       TEXT,
    site_name       TEXT,
    fetched_at      TIMESTAMPTZ DEFAULT NOW(),
    expires_at      TIMESTAMPTZ DEFAULT NOW() + INTERVAL '24 hours'
);
CREATE INDEX IF NOT EXISTS idx_link_previews_url ON link_previews(url);
CREATE INDEX IF NOT EXISTS idx_link_previews_exp ON link_previews(expires_at);

-- ============================================
-- AUDIT LOG
-- ============================================
CREATE TABLE IF NOT EXISTS audit_log (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    actor_id        UUID REFERENCES users(id) ON DELETE SET NULL,
    action          VARCHAR(64) NOT NULL,       -- user.login, message.delete, chat.kick...
    target_type     VARCHAR(32),                -- user, message, chat, session
    target_id       UUID,
    ip_address      INET,
    user_agent      TEXT,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_actor ON audit_log(actor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action, created_at DESC);

-- ============================================
-- RATE LIMITS (fallback if Redis is down)
-- ============================================
CREATE TABLE IF NOT EXISTS rate_limits (
    key             TEXT NOT NULL,
    count           INT DEFAULT 1,
    window_start    TIMESTAMPTZ NOT NULL,
    PRIMARY KEY(key, window_start)
);

-- ============================================
-- CHAT TOPICS (like Telegram forum groups)
-- ============================================
CREATE TABLE IF NOT EXISTS chat_topics (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    chat_id         UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    name            VARCHAR(128) NOT NULL,
    icon_emoji      VARCHAR(10),
    icon_color      VARCHAR(7),                 -- hex color
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    is_closed       BOOLEAN DEFAULT FALSE,
    is_hidden       BOOLEAN DEFAULT FALSE,
    message_count   INT DEFAULT 0,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_topics_chat ON chat_topics(chat_id);

-- Add topic_id to messages
ALTER TABLE messages
    ADD COLUMN IF NOT EXISTS topic_id UUID REFERENCES chat_topics(id) ON DELETE SET NULL;

-- ============================================
-- VOICE ROOMS (persistent, like Discord)
-- ============================================
CREATE TABLE IF NOT EXISTS voice_rooms (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    chat_id         UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    name            VARCHAR(100) NOT NULL,
    type            VARCHAR(20) DEFAULT 'voice', -- voice, stage, video
    user_limit      INT DEFAULT 0,               -- 0 = unlimited
    is_active       BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS voice_room_participants (
    room_id         UUID NOT NULL REFERENCES voice_rooms(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    is_muted        BOOLEAN DEFAULT FALSE,
    is_deafened     BOOLEAN DEFAULT FALSE,
    is_video        BOOLEAN DEFAULT FALSE,
    is_speaking     BOOLEAN DEFAULT FALSE,
    joined_at       TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (room_id, user_id)
);

-- ============================================
-- PREMIUM SUBSCRIPTIONS
-- ============================================
CREATE TABLE IF NOT EXISTS subscriptions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    plan            VARCHAR(20) NOT NULL,        -- free, premium, business
    status          VARCHAR(20) DEFAULT 'active', -- active, cancelled, expired
    started_at      TIMESTAMPTZ DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,
    auto_renew      BOOLEAN DEFAULT TRUE,
    payment_ref     TEXT,                        -- external payment ID
    metadata        JSONB DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_subs_user ON subscriptions(user_id, status);

-- Add premium flag to users for quick check
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS is_premium      BOOLEAN DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS premium_until   TIMESTAMPTZ;

-- ============================================
-- LIVE LOCATION
-- ============================================
CREATE TABLE IF NOT EXISTS live_locations (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id         UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    message_id      UUID REFERENCES messages(id) ON DELETE CASCADE,
    latitude        DOUBLE PRECISION NOT NULL,
    longitude       DOUBLE PRECISION NOT NULL,
    heading         INT,                         -- 0-360 degrees
    accuracy        FLOAT,
    expires_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_live_loc_active ON live_locations(user_id, expires_at) ;

-- ============================================
-- INVITE LINKS (extended)
-- ============================================
CREATE TABLE IF NOT EXISTS invite_links (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    code            VARCHAR(32) UNIQUE NOT NULL,
    chat_id         UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    created_by      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    max_uses        INT DEFAULT 0,               -- 0 = unlimited
    uses_count      INT DEFAULT 0,
    expires_at      TIMESTAMPTZ,
    is_revoked      BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_invite_code ON invite_links(code) WHERE is_revoked = FALSE;

-- ============================================
-- CHANNEL ANALYTICS
-- ============================================
CREATE TABLE IF NOT EXISTS channel_stats (
    channel_id      UUID NOT NULL REFERENCES chats(id) ON DELETE CASCADE,
    date            DATE NOT NULL,
    views           INT DEFAULT 0,
    shares          INT DEFAULT 0,
    new_members     INT DEFAULT 0,
    left_members    INT DEFAULT 0,
    PRIMARY KEY (channel_id, date)
);

-- ============================================
-- Trigger: keep member_count in sync
-- ============================================
CREATE OR REPLACE FUNCTION update_chat_member_count()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        UPDATE chats SET member_count = member_count + 1 WHERE id = NEW.chat_id;
    ELSIF TG_OP = 'DELETE' THEN
        UPDATE chats SET member_count = GREATEST(member_count - 1, 0) WHERE id = OLD.chat_id;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_chat_member_count ON chat_members;
CREATE TRIGGER trg_chat_member_count
AFTER INSERT OR DELETE ON chat_members
FOR EACH ROW EXECUTE FUNCTION update_chat_member_count();

-- ============================================
-- Trigger: auto-save message edit history
-- ============================================
CREATE OR REPLACE FUNCTION save_message_edit_history()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.content IS DISTINCT FROM NEW.content THEN
        INSERT INTO message_edits (message_id, old_content, edit_number)
        VALUES (OLD.id, OLD.content,
                COALESCE((SELECT MAX(edit_number) FROM message_edits WHERE message_id = OLD.id), 0) + 1);
        NEW.is_edited := TRUE;
        NEW.updated_at := NOW();
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_message_edit ON messages;
CREATE TRIGGER trg_message_edit
BEFORE UPDATE ON messages
FOR EACH ROW EXECUTE FUNCTION save_message_edit_history();

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS trg_message_edit ON messages;
DROP FUNCTION IF EXISTS save_message_edit_history();
DROP TRIGGER IF EXISTS trg_chat_member_count ON chat_members;
DROP FUNCTION IF EXISTS update_chat_member_count();
DROP TABLE IF EXISTS channel_stats CASCADE;
DROP TABLE IF EXISTS invite_links CASCADE;
DROP TABLE IF EXISTS live_locations CASCADE;
DROP TABLE IF EXISTS subscriptions CASCADE;
DROP TABLE IF EXISTS voice_room_participants CASCADE;
DROP TABLE IF EXISTS voice_rooms CASCADE;
DROP TABLE IF EXISTS chat_topics CASCADE;
DROP TABLE IF EXISTS rate_limits CASCADE;
DROP TABLE IF EXISTS audit_log CASCADE;
DROP TABLE IF EXISTS link_previews CASCADE;
DROP TABLE IF EXISTS message_edits CASCADE;
DROP TABLE IF EXISTS key_bundles CASCADE;
DROP TABLE IF EXISTS one_time_prekeys CASCADE;
DROP TABLE IF EXISTS signed_prekeys CASCADE;
DROP TABLE IF EXISTS identity_keys CASCADE;
DROP TABLE IF EXISTS passkeys CASCADE;
ALTER TABLE sessions DROP COLUMN IF EXISTS user_agent, DROP COLUMN IF EXISTS fingerprint, DROP COLUMN IF EXISTS revoked_at;
ALTER TABLE users DROP COLUMN IF EXISTS totp_secret, DROP COLUMN IF EXISTS totp_pending, DROP COLUMN IF EXISTS totp_backup;
ALTER TABLE users DROP COLUMN IF EXISTS is_premium, DROP COLUMN IF EXISTS premium_until;
ALTER TABLE messages DROP COLUMN IF EXISTS topic_id;
-- +goose StatementEnd
