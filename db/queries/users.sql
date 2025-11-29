-- name: CreateUser :one
INSERT INTO users (
    username,
    password_hash
) VALUES (
    ?1,
    ?2
)
RETURNING
    id,
    username,
    password_hash,
    created_at;

-- name: GetUserByUsername :one
SELECT
    id,
    username,
    password_hash,
    created_at
FROM users
WHERE username = ?1
LIMIT 1;

-- name: GetUserByID :one
SELECT
    id,
    username,
    password_hash,
    created_at
FROM users
WHERE id = ?1
LIMIT 1;



