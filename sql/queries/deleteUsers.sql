-- name: DeleteUsers :many
DELETE FROM users
RETURNING *;