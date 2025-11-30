-- name: CreateSession :exec
INSERT INTO sessions (
    id,
    user_id,
    created_at,
    expires_at,
    revoked_at,
    revoked_by
) VALUES (
    ?1,
    ?2,
    ?3,
    ?4,
    NULL,
    NULL
);

-- name: RevokeSession :exec
UPDATE sessions
SET revoked_at = CURRENT_TIMESTAMP,
    revoked_by = ?2
WHERE id = ?1
  AND revoked_at IS NULL;


