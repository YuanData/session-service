package config

import (
	"os"
	"strconv"
	"time"
)

// Config 收攏服務會用到的設定。
type Config struct {
	HTTPAddr string // 例如 ":8080"
	DBPath   string // SQLite 檔案路徑，例如 "./data/app.db"

	JWTSecret string // HMAC secret，用於簽 JWT

	// Redis
	RedisAddr     string
	RedisPassword string

	// Session 設定
	SessionTTL         time.Duration
	MaxSessionsPerUser int

	// Asynq worker 設定
	AsynqConcurrency int

	// Admin API key
	AdminAPIKey string
}

// Load 從環境變數載入設定，並給預設值。
func Load() *Config {
	// 預設值
	defaultHTTPAddr := ":8080"
	defaultDBPath := "./data/app.db"
	defaultJWTSecret := "dev-secret-change-me" // 開發預設值，正式環境請務必覆蓋

	defaultRedisAddr := "127.0.0.1:6379"
	defaultRedisPassword := ""

	defaultSessionTTLSeconds := 3600 // 1 小時
	defaultMaxSessionsPerUser := 2
	defaultAsynqConcurrency := 10

	ttlSeconds := getenvInt("SESSION_TTL_SECONDS", defaultSessionTTLSeconds)
	maxSessions := getenvInt("MAX_SESSIONS_PER_USER", defaultMaxSessionsPerUser)
	asynqConc := getenvInt("ASYNQ_CONCURRENCY", defaultAsynqConcurrency)

	adminAPIKey := getenv("ADMIN_API_KEY", "dev-admin")

	return &Config{
		HTTPAddr: getenv("APP_HTTP_ADDR", defaultHTTPAddr),
		DBPath:   getenv("APP_DB_PATH", defaultDBPath),
		JWTSecret: getenv("APP_JWT_SECRET", defaultJWTSecret),

		RedisAddr:     getenv("REDIS_ADDR", defaultRedisAddr),
		RedisPassword: getenv("REDIS_PASSWORD", defaultRedisPassword),

		SessionTTL:         time.Duration(ttlSeconds) * time.Second,
		MaxSessionsPerUser: maxSessions,

		AsynqConcurrency: asynqConc,
		AdminAPIKey:      adminAPIKey,
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}



