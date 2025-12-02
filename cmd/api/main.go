package main

import (
	"database/sql"  // 提供通用 SQL 資料庫操作介面
	"log"           // 用於輸出啟動與錯誤日誌
	"os"            // 檔案與路徑相關操作（例如建立資料夾）
	"path/filepath" // 處理檔案路徑（例如取 DB 目錄）

	"github.com/gin-gonic/gin" // Gin HTTP 框架

	"github.com/golang-migrate/migrate/v4"                               // 資料庫 migration 主套件
	migratesqlite "github.com/golang-migrate/migrate/v4/database/sqlite" // SQLite 專用的 migrate driver
	_ "github.com/golang-migrate/migrate/v4/source/file"                 // 檔案系統作為 migration source（使用 file://）

	"sessionservice/internal/config"       // 讀取服務設定（包含 DBPath / Redis / JWT 等）
	"sessionservice/internal/db"           // sqlc 產生的 DB 存取層
	httpapi "sessionservice/internal/http" // HTTP router 與 handler
	"sessionservice/internal/infra"        // Redis / Asynq 等基礎設施
	"sessionservice/internal/session"      // SessionService 登入 / 登出邏輯
	"sessionservice/internal/token"        // JWT 管理

	_ "modernc.org/sqlite" // 使用 modernc SQLite driver，對應 DSN 名稱 "sqlite"
)

func main() {
	cfg := config.Load()

	// 確保資料夾存在
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		log.Fatalf("failed to create data dir: %v", err)
	}

	// 開啟 SQLite
	sqlDB, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open sqlite: %v", err)
	}
	defer sqlDB.Close()

	// 簡單檢查連線
	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("failed to ping sqlite: %v", err)
	}

	// 執行 migrations，確保 users / sessions table 存在。
	if err := runMigrations(sqlDB); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// 建立 sqlc Queries
	q := db.New(sqlDB)

	// Redis
	rdb := infra.NewRedisClient(cfg)
	defer rdb.Close()

	// Asynq client（給 SessionService 使用）
	asynqClient := infra.NewAsynqClient(cfg)
	defer asynqClient.Close()

	// Session service
	sessSvc := session.NewSessionService(q, rdb, cfg, asynqClient)

	// JWT manager（預設存活時間使用 cfg.SessionTTL）
	jwtMgr := token.NewManager(cfg.JWTSecret, cfg.SessionTTL)

	// 建立 router
	r := httpapi.NewRouter(q, jwtMgr, sessSvc, cfg.SessionTTL, cfg.AdminAPIKey)

	// 啟動 HTTP server
	gin.SetMode(gin.ReleaseMode)
	log.Printf("starting api on %s", cfg.HTTPAddr)
	if err := r.Run(cfg.HTTPAddr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// runMigrations 使用 golang-migrate 套件執行 db/migrations 目錄下的 SQL migration。 // 這裡改用標準化 migration 工具，取代手寫逐檔 Exec
func runMigrations(dbConn *sql.DB) error {
	// 建立 SQLite 專用的 migrate driver，重用現有的 *sql.DB 連線 // 這樣可以共用同一個連線池與 modernc sqlite driver
	driver, err := migratesqlite.WithInstance(dbConn, &migratesqlite.Config{}) // 初始化 migrate 用的 SQLite driver
	if err != nil {                                                            // 若 driver 建立失敗
		return err // 回傳錯誤，中止啟動流程
	}

	// 建立 migrate 實例，指定來源為檔案系統（file://db/migrations）與資料庫名稱 "sqlite" // 來源路徑會掃描 001_xxx.up.sql 等檔案並依版本排序
	m, err := migrate.NewWithDatabaseInstance(
		"file://db/migrations", // migration 檔案所在目錄（需使用 file:// 前綴）
		"sqlite",               // 資料庫名稱（此字串僅作識別用，與驅動名稱分離）
		driver,                 // 上面建立好的 SQLite driver 實例
	)
	if err != nil { // 若建立 migrate 實例失敗
		return err // 回傳錯誤，中止啟動
	}

	// 執行向上遷移，將資料庫 schema 套用到最新版本 // 會依檔名順序依序執行 *.up.sql
	if err := m.Up(); err != nil && err != migrate.ErrNoChange { // 若發生錯誤且不是「沒有變更」的情況
		return err // 回傳錯誤，讓呼叫端決定是否中止服務啟動
	}

	return nil // migration 正常完成或本來就是最新狀態，回傳 nil
}
