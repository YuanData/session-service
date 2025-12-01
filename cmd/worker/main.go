package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"

	"sessionservice/internal/config"
	"sessionservice/internal/db"
	"sessionservice/internal/infra"

	_ "modernc.org/sqlite"
)

func main() {
	cfg := config.Load()

	// SQLite
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		log.Fatalf("failed to create data dir: %v", err)
	}
	sqlDB, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open sqlite: %v", err)
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		log.Fatalf("failed to ping sqlite: %v", err)
	}

	q := db.New(sqlDB)

	// Redis client（給 worker handler 使用）
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       0,
	})
	defer rdb.Close()

	// Asynq server
	srv := asynq.NewServer(
		asynq.RedisClientOpt{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       0,
		},
		asynq.Config{
			Concurrency: cfg.AsynqConcurrency,
		},
	)

	mux := asynq.NewServeMux()

	// session:expire handler
	mux.HandleFunc(infra.TaskTypeSessionExpire, func(ctx context.Context, t *asynq.Task) error {
		var p infra.SessionExpirePayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			log.Printf("session:expire: invalid payload: %v", err)
			return err
		}

		sessKey := infra.SessKey(p.SessionID)
		userSessKey := infra.UserSessKey(p.UserID)

		// 檢查 Redis 是否仍有該 session
		data, err := rdb.HGetAll(ctx, sessKey).Result()
		if err != nil && err != redis.Nil {
			log.Printf("session:expire: redis HGetAll error: %v", err)
			return err
		}
		if len(data) == 0 {
			// 已不存在，可能已手動 logout 或被踢，視為完成
			return nil
		}

		pipe := rdb.TxPipeline()
		pipe.Del(ctx, sessKey)
		pipe.ZRem(ctx, userSessKey, p.SessionID)
		if _, err := pipe.Exec(ctx); err != nil {
			log.Printf("session:expire: redis cleanup error: %v", err)
			return err
		}

		// 更新 DB sessions.revoked_at / revoked_by
		if err := q.RevokeSession(ctx, db.RevokeSessionParams{
			ID:        p.SessionID,
			RevokedBy: sql.NullString{String: "system:expire", Valid: true},
		}); err != nil {
			log.Printf("session:expire: db revoke error: %v", err)
			return err
		}

		return nil
	})

	// login:audit handler
	mux.HandleFunc(infra.TaskTypeLoginAudit, func(ctx context.Context, t *asynq.Task) error {
		var p infra.LoginAuditPayload
		if err := json.Unmarshal(t.Payload(), &p); err != nil {
			log.Printf("login:audit: invalid payload: %v", err)
			return err
		}

		var userID sql.NullInt64
		if p.UserID != nil {
			userID = sql.NullInt64{Int64: *p.UserID, Valid: true}
		}

		// 直接用 Exec 寫入 login_events，避免再擴充 sqlc schema 太多欄位
		_, err := sqlDB.ExecContext(ctx, `
INSERT INTO login_events (
    user_id,
    username,
    success,
    reason,
    ip,
    user_agent,
    created_at
) VALUES (
    ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP
)
`, nullableInt64(userID), p.Username, p.Success, p.Reason, p.IP, p.UserAgent)
		if err != nil {
			log.Printf("login:audit: insert error: %v", err)
			return err
		}
		return nil
	})

	// 啟動 worker
	go func() {
		if err := srv.Run(mux); err != nil {
			log.Fatalf("asynq server stopped: %v", err)
		}
	}()

	log.Printf("asynq worker started with concurrency=%d", cfg.AsynqConcurrency)

	// 等待中斷訊號
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("worker shutting down...")
	srv.Shutdown()
}

func nullableInt64(v sql.NullInt64) interface{} {
	if v.Valid {
		return v.Int64
	}
	return nil
}


