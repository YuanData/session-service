-- name: InsertLoginEvent :exec
INSERT INTO login_events (
    user_id,
    username,
    success,
    reason,
    ip,
    user_agent
) VALUES (
    ?1,
    ?2,
    ?3,
    ?4,
    ?5,
    ?6
);


