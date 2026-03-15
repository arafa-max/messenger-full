-- +goose Up
-- +goose StatementBegin

-- ============================================
-- SOCIAL RECOVERY — Shamir's Secret Sharing
-- Пользователь делит ключ восстановления на N частей
-- среди доверенных контактов (guardians).
-- Для восстановления нужно K из N частей.
-- ============================================

CREATE TABLE IF NOT EXISTS recovery_sessions (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    threshold       INT NOT NULL,           -- K — минимум частей для восстановления
    total_shares    INT NOT NULL,           -- N — всего частей
    status          VARCHAR(20) DEFAULT 'pending', -- pending, active, recovered, cancelled
    encrypted_secret TEXT NOT NULL,        -- зашифрованный секрет (для верификации)
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    expires_at      TIMESTAMPTZ DEFAULT NOW() + INTERVAL '30 days'
);

CREATE TABLE IF NOT EXISTS recovery_shares (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id      UUID NOT NULL REFERENCES recovery_sessions(id) ON DELETE CASCADE,
    guardian_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    share_index     INT NOT NULL,           -- номер шарда (1..N)
    share_data      TEXT NOT NULL,          -- зашифрованный шард (base64)
    is_submitted    BOOLEAN DEFAULT FALSE,  -- guardian подтвердил участие
    submitted_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (session_id, guardian_id),
    UNIQUE (session_id, share_index)
);

CREATE TABLE IF NOT EXISTS recovery_requests (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    session_id      UUID NOT NULL REFERENCES recovery_sessions(id) ON DELETE CASCADE,
    requester_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status          VARCHAR(20) DEFAULT 'pending', -- pending, approved, rejected
    collected_shares JSONB DEFAULT '[]',   -- собранные шарды от guardians
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    expires_at      TIMESTAMPTZ DEFAULT NOW() + INTERVAL '24 hours'
);

CREATE INDEX IF NOT EXISTS idx_recovery_sessions_user ON recovery_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_recovery_shares_session ON recovery_shares(session_id);
CREATE INDEX IF NOT EXISTS idx_recovery_shares_guardian ON recovery_shares(guardian_id);
CREATE INDEX IF NOT EXISTS idx_recovery_requests_session ON recovery_requests(session_id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS recovery_requests CASCADE;
DROP TABLE IF EXISTS recovery_shares CASCADE;
DROP TABLE IF EXISTS recovery_sessions CASCADE;
-- +goose StatementEnd