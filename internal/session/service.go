package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"

	"sessionservice/internal/config"
	"sessionservice/internal/db"
	"sessionservice/internal/infra"
)

// LoginMeta 描述一個登入請求的額外資訊。
type LoginMeta struct {
	IP        string
	UserAgent string
}

// SessionService 處理與 session 相關的 domain 邏輯。
type SessionService struct {
	q   *db.Queries
	rdb *redis.Client
	cfg *config.Config
}

func NewSessionService(q *db.Queries, rdb *redis.Client, cfg *config.Config) *SessionService {
	return &SessionService{
		q:   q,
		rdb: rdb,
		cfg: cfg,
	}
}

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// Login 驗證帳密，建立 Redis session，並寫入 sessions 資料表。
func (s *SessionService) Login(
	ctx context.Context,
	username, password string,
	meta LoginMeta,
) (user db.User, sessionID string, expiresAt time.Time, err error) {
	// 1. 查詢使用者
	u, err := s.q.GetUserByUsername(ctx, username)
	if err != nil {
		if err == sql.ErrNoRows {
			return db.User{}, "", time.Time{}, ErrInvalidCredentials
		}
		return db.User{}, "", time.Time{}, err
	}

	// 2. 驗證密碼（沿用 Phase 1 的 bcrypt 邏輯）
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return db.User{}, "", time.Time{}, ErrInvalidCredentials
	}

	now := time.Now()
	expiresAt = now.Add(s.cfg.SessionTTL)

	// 3. 控制同時登入數：若超過 MaxSessionsPerUser，踢掉最舊的 session
	if s.cfg.MaxSessionsPerUser > 0 {
		key := infra.UserSessKey(u.ID)
		count, err := s.rdb.ZCard(ctx, key).Result()
		if err != nil && err != redis.Nil {
			return db.User{}, "", time.Time{}, err
		}
		if count >= int64(s.cfg.MaxSessionsPerUser) {
			// 取得最舊的 session（score 最小者）
			oldest, err := s.rdb.ZRange(ctx, key, 0, 0).Result()
			if err != nil && err != redis.Nil {
				return db.User{}, "", time.Time{}, err
			}
			if len(oldest) > 0 {
				oldSID := oldest[0]
				// 刪除 Redis 裡舊的 session 資料
				pipe := s.rdb.TxPipeline()
				pipe.Del(ctx, infra.SessKey(oldSID))
				pipe.ZRem(ctx, key, oldSID)
				_, _ = pipe.Exec(ctx)

				// 資料庫裡的 session 記錄：標記 revoked_at / revoked_by
				_ = s.q.RevokeSession(ctx, db.RevokeSessionParams{
					ID:        oldSID,
					RevokedBy: sql.NullString{String: "system:limit", Valid: true},
				})
			}
		}
	}

	// 4. 為這次登入產生新的 session ID
	newSID := uuid.NewString()

	// 5. 寫入 Redis：sess:{sid} hash + user_sess:{uid} zset
	sessKey := infra.SessKey(newSID)
	userSessKey := infra.UserSessKey(u.ID)

	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, sessKey, map[string]interface{}{
		"user_id":    u.ID,
		"created_at": now.Unix(),
		"expires_at": expiresAt.Unix(),
		"ip":         meta.IP,
		"user_agent": meta.UserAgent,
	})
	pipe.ExpireAt(ctx, sessKey, expiresAt)
	pipe.ZAdd(ctx, userSessKey, redis.Z{
		Score:  float64(now.Unix()),
		Member: newSID,
	})
	if _, err := pipe.Exec(ctx); err != nil {
		return db.User{}, "", time.Time{}, err
	}

	// 6. 寫入 SQLite sessions 表（作為 audit）
	if err := s.q.CreateSession(ctx, db.CreateSessionParams{
		ID:        newSID,
		UserID:    u.ID,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}); err != nil {
		return db.User{}, "", time.Time{}, err
	}

	return u, newSID, expiresAt, nil
}

// Logout 刪除 Redis 內的 session，並更新 SQLite sessions 表。
func (s *SessionService) Logout(ctx context.Context, userID int64, sessionID string) error {
	sessKey := infra.SessKey(sessionID)
	userSessKey := infra.UserSessKey(userID)

	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, sessKey)
	pipe.ZRem(ctx, userSessKey, sessionID)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	// 更新資料庫中的 session 狀態（若存在）
	_ = s.q.RevokeSession(ctx, db.RevokeSessionParams{
		ID:        sessionID,
		RevokedBy: sql.NullString{String: "user", Valid: true},
	})

	return nil
}

// IsSessionValid 檢查 Redis 中該 session 是否存在且 user_id 符合。
func (s *SessionService) IsSessionValid(ctx context.Context, userID int64, sessionID string) (bool, error) {
	sessKey := infra.SessKey(sessionID)
	data, err := s.rdb.HGetAll(ctx, sessKey).Result()
	if err != nil && err != redis.Nil {
		return false, err
	}
	if len(data) == 0 {
		return false, nil
	}

	// 簡單比對 user_id 是否一致（以字串形式比對）
	if uidStr, ok := data["user_id"]; ok {
		if uidStr != "" && uidStr != stringFromInt64(userID) {
			return false, nil
		}
	}

	return true, nil
}

// stringFromInt64 將 int64 轉成字串（避免在 service 內直接依賴 strconv）。
func stringFromInt64(v int64) string {
	return fmt.Sprintf("%d", v)
}


