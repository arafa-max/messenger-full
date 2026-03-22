-- +goose Up
-- ──────────────────────────────────────────────────────────────
-- 026_halal.sql — ToS + Report System + Bans
-- ──────────────────────────────────────────────────────────────

ALTER TABLE users ADD COLUMN IF NOT EXISTS accepted_terms BOOLEAN DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS terms_accepted_at TIMESTAMPTZ;
ALTER TABLE users ADD COLUMN IF NOT EXISTS birth_year INT;

CREATE TABLE IF NOT EXISTS reports (
    id BIGSERIAL PRIMARY KEY,
    reporter_id UUID NOT NULL REFERENCES users(id),
    reported_user_id UUID NOT NULL REFERENCES users(id),
    reported_message_id BIGINT,
    reason VARCHAR(50) NOT NULL,
    description TEXT,
    status VARCHAR(20) DEFAULT 'pending',
    reviewed_by UUID REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_reports_status ON reports(status);
CREATE INDEX idx_reports_reported_user ON reports(reported_user_id);

CREATE TABLE IF NOT EXISTS user_bans (
    id BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    banned_by UUID REFERENCES users(id),
    reason TEXT NOT NULL,
    permanent BOOLEAN DEFAULT false,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_bans_user ON user_bans(user_id);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION is_user_banned(uid UUID) RETURNS BOOLEAN AS $$
BEGIN
    RETURN EXISTS (
        SELECT 1 FROM user_bans
        WHERE user_id = uid
        AND (permanent = true OR expires_at > NOW())
    );
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION IF EXISTS is_user_banned(UUID);
DROP TABLE IF EXISTS user_bans;
DROP TABLE IF EXISTS reports;
ALTER TABLE users DROP COLUMN IF EXISTS birth_year;
ALTER TABLE users DROP COLUMN IF EXISTS terms_accepted_at;
ALTER TABLE users DROP COLUMN IF EXISTS accepted_terms;