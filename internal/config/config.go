package config // 宣告本檔案屬於 config 套件，提供整個專案共用的設定結構與載入邏輯

import (
	"time" // 引入 time 套件，用來處理時間與 Duration 型別

	"github.com/spf13/viper" // 引入 viper 套件，負責讀取環境變數與 .env 設定檔
)

// Config 收攏服務會用到的設定。 // 定義 Config 結構體，集中管理所有服務設定欄位
type Config struct {
	HTTPAddr string // 例如 ":8080"；HTTP 服務監聽位址
	DBPath   string // SQLite 檔案路徑，例如 "./data/app.db"

	JWTSecret string // HMAC secret，用於簽 JWT

	// Redis
	RedisAddr     string // Redis 連線位址，例如 "127.0.0.1:6379"
	RedisPassword string // Redis 密碼，預設空字串代表無密碼

	// Session 設定
	SessionTTL         time.Duration // Session 與 JWT 的存活時間
	MaxSessionsPerUser int           // 單一使用者允許同時存在的 Session 上限

	// Asynq worker 設定
	AsynqConcurrency int // Asynq worker 併發數量

	// Admin API key
	AdminAPIKey string // Admin 後台 API 使用的簡易驗證密鑰
}

// Load 使用 viper 從環境變數與 .env 檔載入設定，並給預設值。 // 對外提供載入設定的統一入口
func Load() *Config {
	// 初始化 viper：優先讀取環境變數，再從 .env 檔補值 // 說明載入順序：環境變數優先，其次 .env，最後才是預設值
	v := viper.New() // 建立一個新的 viper 實例，避免污染全域狀態

	v.SetEnvPrefix("") // 不加前綴，直接使用既有名稱，方便沿用現有環境變數名稱
	v.AutomaticEnv()   // 啟用自動從環境變數讀取的功能

	v.SetConfigName(".env") // 告訴 viper 設定檔名稱為 .env（不含副檔名）
	v.SetConfigType("env")  // 指定設定檔格式為 dotenv 風格的純文字 key=value
	v.AddConfigPath(".")    // 專案根目錄作為預設搜尋路徑

	// 若 .env 不存在，不視為錯誤，方便容器 / 雲端只用環境變數配置 // 容忍沒有 .env 的情況，以利在 Kubernetes / Docker 只用環境變數
	_ = v.ReadInConfig() // 嘗試讀取 .env，若失敗直接忽略錯誤（不會中止程式）

	// 預設值（僅當環境變數與 .env 都沒有時才會用到） // 提供安全的 fallback，確保本機開發即使沒設 .env 也能啟動
	v.SetDefault("APP_HTTP_ADDR", ":8080")             // HTTP 監聽位址預設為 :8080
	v.SetDefault("APP_DB_PATH", "./data/app.db")      // SQLite 檔案預設存放於 ./data/app.db
	v.SetDefault("APP_JWT_SECRET", "dev-secret-change-me") // 開發預設 JWT 密鑰，正式環境請務必覆蓋

	v.SetDefault("REDIS_ADDR", "127.0.0.1:6379") // Redis 預設位址
	v.SetDefault("REDIS_PASSWORD", "")           // Redis 預設無密碼

	v.SetDefault("SESSION_TTL_SECONDS", 3600) // 1 小時；Session 與 JWT 預設存活秒數
	v.SetDefault("MAX_SESSIONS_PER_USER", 2)  // 同一使用者預設最多同時 2 個 Session
	v.SetDefault("ASYNQ_CONCURRENCY", 10)     // Asynq worker 預設併發數為 10
	v.SetDefault("ADMIN_API_KEY", "dev-admin") // 開發預設 admin key，方便本機測試

	// 組合 Config 結構並回傳給呼叫端 // 將剛才透過 viper 取得的值轉成強型別設定物件
	return &Config{
		HTTPAddr:  v.GetString("APP_HTTP_ADDR"),  // 讀取 HTTP 監聽位址字串
		DBPath:    v.GetString("APP_DB_PATH"),    // 讀取 SQLite 檔案路徑字串
		JWTSecret: v.GetString("APP_JWT_SECRET"), // 讀取 JWT 簽章密鑰

		RedisAddr:     v.GetString("REDIS_ADDR"),     // 讀取 Redis 位址
		RedisPassword: v.GetString("REDIS_PASSWORD"), // 讀取 Redis 密碼

		SessionTTL:         time.Duration(v.GetInt("SESSION_TTL_SECONDS")) * time.Second, // 將秒數轉成 time.Duration
		MaxSessionsPerUser: v.GetInt("MAX_SESSIONS_PER_USER"),                            // 讀取單一使用者 Session 上限

		AsynqConcurrency: v.GetInt("ASYNQ_CONCURRENCY"), // 讀取 Asynq worker 併發設定
		AdminAPIKey:      v.GetString("ADMIN_API_KEY"), // 讀取 Admin API 密鑰
	}
}
