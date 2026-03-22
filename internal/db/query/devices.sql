-- name: CreateDevice :one
INSERT INTO devices(user_id,push_token,platform,device_name)
VALUES($1,$2,$3,$4)RETURNING *;

-- name: GetDeviceByID :one
SELECT * FROM devices WHERE id =$1;

-- name: GetUserDevices :many
SELECT * FROM devices WHERE user_id = $1 AND is_active= TRUE;

-- name: UpdateDevicePushToken :exec
UPDATE devices SET push_token=$2, last_active = NOW() WHERE id =$1;

-- name: DeactivateDevice :exec
UPDATE devices SET is_active = FALSE WHERE id = $1;

-- name: DeleteDevice :exec
DELETE FROM devices WHERE id = $1;

-- name: GetUserPushTokens :many
SELECT push_token, platform FROM devices
WHERE user_id = $1 AND is_active = TRUE AND push_token IS NOT NULL;