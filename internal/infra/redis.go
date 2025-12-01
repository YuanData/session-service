package infra

import (
	"fmt"

	"github.com/redis/go-redis/v9"

	"sessionservice/internal/config"
)

// NewRedisClient 根據 config 建立 Redis client。
func NewRedisClient(cfg *config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       0,
	})
}

// Redis key 命名規則：
// sess:{sessionID}   -> Hash: user_id, created_at, expires_at, ip, user_agent
// user_sess:{userID} -> Sorted Set: member=sessionID, score=created_at unix
// banned_user:{userID} -> String flag，存在即代表被 ban

func SessKey(sessionID string) string {
	return fmt.Sprintf("sess:%s", sessionID)
}

func UserSessKey(userID int64) string {
	return fmt.Sprintf("user_sess:%d", userID)
}

func BannedUserKey(userID int64) string {
	return fmt.Sprintf("banned_user:%d", userID)
}



