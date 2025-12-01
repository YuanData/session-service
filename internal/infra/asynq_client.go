package infra

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hibiken/asynq"

	"sessionservice/internal/config"
)

// 任務類型常數
const (
	TaskTypeSessionExpire = "session:expire"
	TaskTypeLoginAudit    = "login:audit"
)

// SessionExpirePayload 用於 session:expire 任務。
type SessionExpirePayload struct {
	SessionID string `json:"session_id"`
	UserID    int64  `json:"user_id"`
}

// LoginAuditPayload 用於 login:audit 任務。
type LoginAuditPayload struct {
	UserID    *int64 `json:"user_id,omitempty"`
	Username  string `json:"username"`
	Success   bool   `json:"success"`
	Reason    string `json:"reason"`
	IP        string `json:"ip"`
	UserAgent string `json:"user_agent"`
}

// NewAsynqClient 根據 config 建立 Asynq client。
func NewAsynqClient(cfg *config.Config) *asynq.Client {
	return asynq.NewClient(asynq.RedisClientOpt{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       0,
	})
}

// EnqueueSessionExpire 在指定時間執行 session:expire 任務。
func EnqueueSessionExpire(
	ctx context.Context,
	client *asynq.Client,
	sessionID string,
	userID int64,
	processAt time.Time,
) error {
	if client == nil {
		return nil
	}
	payload := SessionExpirePayload{
		SessionID: sessionID,
		UserID:    userID,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TaskTypeSessionExpire, data)
	_, err = client.EnqueueContext(ctx, task, asynq.ProcessAt(processAt))
	return err
}

// EnqueueLoginAudit 立即送出 login:audit 任務。
func EnqueueLoginAudit(
	ctx context.Context,
	client *asynq.Client,
	payload LoginAuditPayload,
) error {
	if client == nil {
		return nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	task := asynq.NewTask(TaskTypeLoginAudit, data)
	_, err = client.EnqueueContext(ctx, task)
	return err
}


