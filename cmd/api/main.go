package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"

	"sessionservice/internal/config"
	"sessionservice/internal/db"
	"sessionservice/internal/infra"
	httpapi "sessionservice/internal/http"
	"sessionservice/internal/session"
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

	// 執行 migrations，確保 users / sessions table 存在。
	if err := runMigrations(sqlDB); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	// 建立 sqlc Queries
	q := db.New(sqlDB)

	// Redis
	rdb := infra.NewRedisClient(cfg)
	defer rdb.Close()

	// Session service
	sessSvc := session.NewSessionService(q, rdb, cfg)

	// JWT manager（預設存活時間使用 cfg.SessionTTL）
	jwtMgr := token.NewManager(cfg.JWTSecret, cfg.SessionTTL)

	// 建立 router
	r := httpapi.NewRouter(q, jwtMgr, sessSvc, cfg.SessionTTL)

	// 啟動 HTTP server
	gin.SetMode(gin.ReleaseMode)
	log.Printf("starting api on %s", cfg.HTTPAddr)
	if err := r.Run(cfg.HTTPAddr); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// runMigrations 執行所有 db/migrations/*.sql。
// 這裡用最簡單的方式依檔名排序後逐一 Exec。
func runMigrations(dbConn *sql.DB) error {
	pattern := "db/migrations/*.sql"
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if _, err := dbConn.Exec(string(content)); err != nil {
			return err
		}
	}
	return nil
}


