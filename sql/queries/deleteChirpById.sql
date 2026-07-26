-- name: DeleteChirpById :exec
DELETE FROM chirps WHERE ID=$1;