-- name: CreateUser :one
INSERT INTO users(username,phone,email,password,language)
VALUES ($1,$2,$3,$4,$5)
RETURNING *;
-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 AND is_deleted = FALSE;
-- name: GetUserByUsername :one
SELECT * FROM users WHERE username =$1 AND is_deleted=FALSE;
-- name: GetUserByEmail :one
SELECT * FROM users WHERE email =$1 AND is_deleted=FALSE;
-- name: GetUserByPhone :one
SELECT * FROM users WHERE phone =$1 AND is_deleted=FALSE;
-- name: UpdateUserOnlineStatus :exec
UPDATE users SET is_online =$2, last_seen = NOW()WHERE id=$1;
-- name: DeleteUser :exec
UPDATE users SET is_deleted = TRUE,delete_at =NOW()WHERE id =$1;