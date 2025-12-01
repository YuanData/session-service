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
    created_at,
    is_banned;

-- name: GetUserByUsername :one
SELECT
    id,
    username,
    password_hash,
    created_at,
    is_banned
FROM users
WHERE username = ?1
LIMIT 1;

-- name: GetUserByID :one
SELECT
    id,
    username,
    password_hash,
    created_at,
    is_banned
FROM users
WHERE id = ?1
LIMIT 1;

-- name: BanUser :exec
UPDATE users
SET is_banned = 1
WHERE id = ?1;

-- name: UnbanUser :exec
UPDATE users
SET is_banned = 0
WHERE id = ?1;




