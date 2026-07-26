-- name: UpdateUserDetails :one
UPDATE users SET email=$1, hashed_password=$2 WHERE id=$3
RETURNING *;