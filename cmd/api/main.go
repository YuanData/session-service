package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"sessionservice/internal/config"
	"sessionservice/internal/db"
	httpapi "sessionservice/internal/http"
	"sessionservice/internal/token"

	_ "modernc.org/sqlite"
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

	// 執行最初的 migration，確保 users table 存在。
	if err := runMigrations(sqlDB); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// 建立 sqlc Queries
	q := db.New(sqlDB)

	// JWT manager（這裡預設存活 24h）
	jwtMgr := token.NewManager(cfg.JWTSecret, 24*time.Hour)

	// 建立 router
	r := httpapi.NewRouter(q, jwtMgr)

	// 啟動 HTTP server
	gin.SetMode(gin.ReleaseMode)
	log.Printf("starting api on %s", cfg.HTTPAddr)
	if err := r.Run(cfg.HTTPAddr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// runMigrations 執行最基本的 migration：001_init.sql。
// 這裡用最簡單的方式直接 Exec 檔案內容即可。
func runMigrations(dbConn *sql.DB) error {
	path := "db/migrations/001_init.sql"
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = dbConn.Exec(string(content))
	return err
}


