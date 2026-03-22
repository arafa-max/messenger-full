-- name: GetUserByUsername :one
SELECT * FROM users WHERE username = $1 LIMIT 1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1 LIMIT 1;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1 LIMIT 1;

-- name: CreateUser :one
INSERT INTO users (
    username,
    email,
    password,
    language,
    birth_year,
    accepted_terms,
    terms_accepted_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING *;

-- name: UpdateUserOnlineStatus :exec
UPDATE users
SET is_online = $2, last_seen = NOW()
WHERE id = $1;

-- name: UpdateUserAvatar :exec
UPDATE users
SET avatar_url = $2
WHERE id = $1;

-- name: UpdateUserTerms :exec
UPDATE users
SET accepted_terms = $2, terms_accepted_at = $3
WHERE id = $1;