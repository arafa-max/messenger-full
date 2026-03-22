-- name: CreateReport :one
INSERT INTO reports (
    reporter_id,
    reported_user_id,
    reported_message_id,
    reason,
    description
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetPendingReports :many
SELECT * FROM reports
WHERE status = 'pending'
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateReportStatus :exec
UPDATE reports
SET status = $1,
    reviewed_by = $2,
    reviewed_at = NOW()
WHERE id = $3;

-- name: BanUser :one
INSERT INTO user_bans (
    user_id,
    banned_by,
    reason,
    permanent,
    expires_at
) VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: IsUserBanned :one
SELECT CAST(is_user_banned($1::uuid) AS boolean) AS banned;
-- name: GetUserReportsCount :one
SELECT COUNT(*)::bigint FROM reports
WHERE reported_user_id = $1
AND status = 'actioned';