package config

import (
	"os"
)

// Config 收攏 Phase 1 會用到的設定。
type Config struct {
	HTTPAddr string // 例如 ":8080"
	DBPath   string // SQLite 檔案路徑，例如 "./data/app.db"

	JWTSecret string // HMAC secret，用於簽 JWT
}

// Load 從環境變數載入設定，並給預設值。
func Load() *Config {
	return &Config{
		HTTPAddr: getenv("APP_HTTP_ADDR", ":8080"),
		DBPath:   getenv("APP_DB_PATH", "./data/app.db"),
		JWTSecret: getenv(
			"APP_JWT_SECRET",
			"dev-secret-change-me", // 開發預設值，正式環境請務必覆蓋
		),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}


