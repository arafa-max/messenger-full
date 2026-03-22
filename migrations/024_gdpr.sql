-- migrations/024_gdpr.sql
-- GDPR: таблица для логирования запросов на удаление/экспорт

-- +goose Up
CREATE TABLE IF NOT EXISTS gdpr_requests (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL,
    type        VARCHAR(10) NOT NULL CHECK (type IN ('export', 'delete')),
    status      VARCHAR(10) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'done')),
    requested_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_gdpr_requests_user_id ON gdpr_requests(user_id);

-- +goose Down
DROP TABLE IF EXISTS gdpr_requests;