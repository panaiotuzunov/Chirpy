-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email)
VALUES (
    id = $1,
    created_at = $2,
    updated_at = $3,
    email = $4
)
RETURNING *;