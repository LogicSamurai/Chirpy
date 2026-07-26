-- name: UpdateRefreshToken :exec
UPDATE refresh_tokens SET revoked_at = $1 WHERE token = $2;