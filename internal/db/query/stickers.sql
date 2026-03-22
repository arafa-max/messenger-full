-- name: GetUserStickerPacks :many
SELECT
    sp.id,
    sp.name,
    sp.thumb_url,
    sp.is_official,
    usp.position,
    usp.added_at
FROM sticker_packs sp
JOIN user_sticker_packs usp ON usp.pack_id = sp.id
WHERE usp.user_id = $1
ORDER BY usp.added_at DESC;

-- name: GetPublicStickerPacks :many
SELECT
    sp.id,
    sp.name,
    sp.thumb_url,
    sp.is_official,
    sp.created_at
FROM sticker_packs sp
ORDER BY sp.is_official DESC, sp.created_at DESC;

-- name: InstallStickerPack :exec
INSERT INTO user_sticker_packs (user_id, pack_id, position)
VALUES (
    $1, $2,
    (SELECT COALESCE(MAX(position) + 1, 0) FROM user_sticker_packs WHERE user_id = $1)
)
ON CONFLICT (user_id, pack_id) DO NOTHING;

-- name: UninstallStickerPack :exec
DELETE FROM user_sticker_packs
WHERE user_id = $1 AND pack_id = $2;

-- name: GetPackStickers :many
SELECT
    s.id,
    s.emoji,
    s.position,
    s.format,
    s.lottie_url,
    m.object_key,
    m.mime_type,
    m.width,
    m.height
FROM stickers s
JOIN media m ON m.id = s.media_id
WHERE s.pack_id = $1
ORDER BY s.position ASC;

-- name: SearchStickers :many
SELECT
    s.id,
    s.emoji,
    s.format,
    s.lottie_url,
    sp.name AS pack_name,
    m.object_key,
    m.mime_type,
    m.width,
    m.height
FROM stickers s
JOIN sticker_packs sp ON sp.id = s.pack_id
JOIN media m ON m.id = s.media_id
WHERE s.emoji ILIKE '%' || sqlc.arg(query) || '%'
LIMIT 50;

-- name: GetStickersByEmoji :many
-- Используется для inline suggestions: клиент присылает точный эмодзи
-- Сначала стикеры из установленных паков пользователя, потом остальные
SELECT
    s.id,
    s.emoji,
    s.format,
    s.lottie_url,
    sp.name  AS pack_name,
    sp.id    AS pack_id,
    m.object_key,
    m.mime_type,
    m.width,
    m.height,
    -- installed = 1 если пак установлен у пользователя, иначе 0
    CASE WHEN usp.user_id IS NOT NULL THEN 1 ELSE 0 END AS is_installed
FROM stickers s
JOIN sticker_packs sp ON sp.id = s.pack_id
JOIN media m ON m.id = s.media_id
LEFT JOIN user_sticker_packs usp
    ON usp.pack_id = sp.id AND usp.user_id = sqlc.arg(user_id)
WHERE s.emoji = sqlc.arg(emoji)
ORDER BY is_installed DESC, s.position ASC
LIMIT 24;

-- name: CreateStickerPack :one
INSERT INTO sticker_packs (name, author_id, thumb_url, is_official)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: AddStickerToPack :one
INSERT INTO stickers (pack_id, media_id, emoji, position, format, lottie_url)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetPremiumStickerPacks :many
SELECT id, name, thumb_url, is_official, is_premium, created_at
FROM sticker_packs
WHERE is_premium = TRUE
ORDER BY created_at DESC;

-- name: GetPublicStickerPacksForUser :many
SELECT
    sp.id, sp.name, sp.thumb_url, sp.is_official, sp.is_premium, sp.created_at,
    CASE WHEN usp.user_id IS NOT NULL THEN TRUE ELSE FALSE END AS is_installed
FROM sticker_packs sp
LEFT JOIN user_sticker_packs usp ON usp.pack_id = sp.id AND usp.user_id = sqlc.arg(user_id)
WHERE sp.is_premium = FALSE OR sqlc.arg(is_premium)::boolean = TRUE
ORDER BY sp.is_official DESC, sp.created_at DESC;