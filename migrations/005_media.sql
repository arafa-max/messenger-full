-- +goose Up

CREATE TYPE media_type AS ENUM (
    'image', 'video', 'audio', 'file', 'voice', 'sticker', 'gif'
);

CREATE TYPE media_status AS ENUM (
    'pending',    -- presigned URL выдан, файл ещё не загружен
    'uploaded',   -- клиент загрузил файл напрямую в MinIO
    'processed',  -- сервер обработал (thumbnail, транскодинг)
    'failed'      -- обработка упала
);

CREATE TABLE media (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    uploader_id     UUID NOT NULL REFERENCES users(id) ON DELETE SET NULL,
    type            media_type NOT NULL,
    status          media_status NOT NULL DEFAULT 'pending',

    -- MinIO
    bucket          TEXT NOT NULL,
    object_key      TEXT NOT NULL,              -- путь внутри bucket
    original_name   TEXT,                       -- оригинальное имя файла
    mime_type       TEXT NOT NULL,
    size_bytes      BIGINT,                     -- заполняется после upload

    -- для изображений и видео
    width           INT,
    height          INT,
    duration_sec    FLOAT,                      -- для audio/video

    -- thumbnail (для video/image)
    thumb_key       TEXT,                       -- путь к превью в MinIO

    -- waveform для voice/audio (JSON массив float)
    waveform        JSONB,

    -- blur hash для плавной загрузки изображений
    blur_hash       TEXT,

    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,                -- для временных медиа (исчезающие сообщения)

    UNIQUE(bucket, object_key)
);

CREATE INDEX idx_media_uploader ON media(uploader_id);
CREATE INDEX idx_media_status   ON media(status);

-- +goose Down
DROP TABLE IF EXISTS media;
DROP TYPE IF EXISTS media_status;
DROP TYPE IF EXISTS media_type;