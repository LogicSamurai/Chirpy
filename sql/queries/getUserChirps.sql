-- name: GetChirpsForUser :many
SELECT id, created_at, updated_at, body, user_id FROM chirps
WHERE user_id = sqlc.arg('user_id')
ORDER BY
  CASE WHEN sqlc.arg('sort_direction') = 'desc' THEN created_at END DESC,
  CASE WHEN sqlc.arg('sort_direction') <> 'desc' THEN created_at END ASC;
