-- name: CreateVoiceRoom :one
INSERT INTO voice_rooms (id, chat_id, name, type, user_limit, is_active)
VALUES ($1, $2, $3, $4, $5, TRUE)
RETURNING *;

-- name: GetActiveRoomsByChatID :many
SELECT
    vr.id,
    vr.name,
    vr.type,
    vr.user_limit,
    vr.is_active,
    vr.created_at,
    COUNT(vrp.user_id)::int AS participant_count
FROM voice_rooms vr
LEFT JOIN voice_room_participants vrp ON vrp.room_id = vr.id
WHERE vr.chat_id = $1 AND vr.is_active = TRUE
GROUP BY vr.id, vr.name, vr.type, vr.user_limit, vr.is_active, vr.created_at
ORDER BY vr.created_at ASC;

-- name: GetRoomForJoin :one
SELECT
    vr.user_limit,
    vr.chat_id,
    COUNT(vrp.user_id)::int AS participant_count
FROM voice_rooms vr
LEFT JOIN voice_room_participants vrp ON vrp.room_id = vr.id
WHERE vr.id = $1 AND vr.is_active = TRUE
GROUP BY vr.user_limit, vr.chat_id;

-- name: JoinVoiceRoom :exec
INSERT INTO voice_room_participants (room_id, user_id)
VALUES ($1, $2)
ON CONFLICT (room_id, user_id) DO NOTHING;

-- name: LeaveVoiceRoom :exec
DELETE FROM voice_room_participants
WHERE room_id = $1 AND user_id = $2;

-- name: GetRoomChatID :one
SELECT chat_id
FROM voice_rooms
WHERE id = $1;

-- name: UpdateParticipantState :exec
UPDATE voice_room_participants
SET
    is_muted    = COALESCE($3, is_muted),
    is_deafened = COALESCE($4, is_deafened),
    is_video    = COALESCE($5, is_video)
WHERE room_id = $1 AND user_id = $2;

-- name: GetRoomParticipants :many
SELECT
    vrp.user_id,
    u.username,
    COALESCE(u.avatar_url, '') AS avatar_url,
    vrp.is_muted,
    vrp.is_deafened,
    vrp.is_video,
    vrp.is_speaking,
    vrp.joined_at
FROM voice_room_participants vrp
JOIN users u ON u.id = vrp.user_id
WHERE vrp.room_id = $1
ORDER BY vrp.joined_at ASC;

-- name: CloseVoiceRoom :exec
UPDATE voice_rooms
SET is_active = FALSE
WHERE id = $1;

-- name: DeleteRoomParticipants :exec
DELETE FROM voice_room_participants
WHERE room_id = $1;