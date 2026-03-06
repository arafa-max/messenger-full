-- name: CreateSession :one
INSERT INTO sessions(user_id,device_id,refresh_token,ip_address,expires_at)
VALUES ($1,$2,$3,$4,$5) RETURNING *;

-- name: GetSessionByToken :one
SELECT *FROM sessions WHERE refresh_token=$1 AND expires_at>NOW();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE refresh_token =$1;

-- name: DeleteAllUserSessions :exec
DELETE FROM sessions WHERE user_id = $1;