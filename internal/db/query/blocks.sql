-- name: BlockUser :exec
INSERT INTO user_blocks (blocker_id, blocked_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: UnblockUser :exec
DELETE FROM user_blocks
WHERE blocker_id = $1 AND blocked_id = $2;

-- name: GetBlockedUsers :many
SELECT u.id, u.username, u.avatar_url
FROM user_blocks b
JOIN users u ON u.id = b.blocked_id
WHERE b.blocker_id = $1
ORDER BY b.created_at DESC;

-- name: IsBlocked :one
SELECT EXISTS (
    SELECT 1 FROM user_blocks
    WHERE blocker_id = $1 AND blocked_id = $2
) AS blocked;