-- name: RecordChannelStat :exec
INSERT INTO channel_stats (channel_id, date, views, shares, new_members, left_members)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (channel_id, date) DO UPDATE SET
    views        = channel_stats.views + EXCLUDED.views,
    shares       = channel_stats.shares + EXCLUDED.shares,
    new_members  = channel_stats.new_members + EXCLUDED.new_members,
    left_members = channel_stats.left_members + EXCLUDED.left_members;

-- name: GetChannelStats :many
SELECT * FROM channel_stats
WHERE channel_id = $1
  AND date >= $2
  AND date <= $3
ORDER BY date ASC;

-- name: GetChannelStatsSummary :one
SELECT
    SUM(views)        AS total_views,
    SUM(shares)       AS total_shares,
    SUM(new_members)  AS total_new_members,
    SUM(left_members) AS total_left_members
FROM channel_stats
WHERE channel_id = $1
  AND date >= $2
  AND date <= $3;
