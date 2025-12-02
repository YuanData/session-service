package session

import (
	"context"          // 匯入 context，用於在 DB 與 Redis 操作中傳遞取消與逾時控制
	"database/sql"     // 匯入 database/sql，建立測試用 SQLite 連線
	"os"               // 匯入 os，用於讀取 migration 檔案內容
	"testing"          // 匯入 testing，提供單元與整合測試框架
	"time"             // 匯入 time，用於檢查 TTL 與時間相關邏輯

	"github.com/alicebob/miniredis/v2" // 匯入 miniredis，提供記憶體內 Redis 測試實例
	"github.com/redis/go-redis/v9"     // 匯入 go-redis，用於連線到 miniredis
	"github.com/stretchr/testify/require" // 匯入 testify/require，簡化斷言撰寫
	"golang.org/x/crypto/bcrypt"          // 匯入 bcrypt 套件，產生與驗證密碼雜湊

	"sessionservice/internal/config" // 匯入 config 套件，建立測試用設定
	"sessionservice/internal/db"     // 匯入 db 套件，建立 sqlc Queries
	"sessionservice/internal/infra"  // 匯入 infra 套件，存取 Redis key helper

	_ "modernc.org/sqlite" // 匯入 modernc sqlite driver，讓 sql.Open(\"sqlite\", ...) 可以運作
)

// testEnv 封裝 SessionService 測試所需的周邊資源。
type testEnv struct {
	ctx     context.Context    // 測試共用的背景 context
	sqlDB   *sql.DB           // SQLite 連線
	q       *db.Queries       // sqlc 產生的 Queries，用於 DB 操作
	rdb     *redis.Client     // Redis client，連線到 miniredis
	mr      *miniredis.Miniredis // miniredis 實例，用於模擬 Redis
	cfg     *config.Config    // 測試用設定
	sessSvc *SessionService   // 被測試的 SessionService
}

// newTestEnv 建立一份完整的測試環境：SQLite（套用 migrations）、miniredis、SessionService。
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()                          // 標記為測試輔助函式
	ctx := context.Background()         // 建立背景 context

	sqlDB, err := sql.Open("sqlite", ":memory:") // 建立記憶體內 SQLite DB，避免產生實體檔案
	require.NoError(t, err)                      // 確保開啟成功

	// 套用所有 migration，確保 schema 與正式環境一致。
	applyMigrations(t, sqlDB)        // 呼叫輔助函式讀取並執行 migration SQL

	q := db.New(sqlDB)               // 建立 sqlc Queries 實例

	mr, err := miniredis.Run()       // 啟動一個記憶體內 Redis 測試伺服器
	require.NoError(t, err)          // 確保啟動成功

	rdb := redis.NewClient(&redis.Options{ // 透過 go-redis 連線到 miniredis
		Addr: mr.Addr(),              // 使用 miniredis 提供的位址
		DB:   0,                      // 使用預設 DB 0
	})

	cfg := &config.Config{               // 建立測試用設定
		SessionTTL:         time.Hour, // 讓 session 與 token TTL 為 1 小時
		MaxSessionsPerUser: 2,         // 設定每個使用者最多同時 2 個 session
	}

	sessSvc := NewSessionService(q, rdb, cfg, nil) // 建立 SessionService，Asynq client 傳 nil 即可（測試中不排任務）

	t.Cleanup(func() {           // 註冊清理邏輯，確保測試結束時釋放資源
		_ = sqlDB.Close()    // 關閉 SQLite 連線
		rdb.Close()          // 關閉 Redis client
		mr.Close()           // 關閉 miniredis 伺服器
	})

	return &testEnv{             // 回傳封裝好的測試環境
		ctx:     ctx,
		sqlDB:   sqlDB,
		q:       q,
		rdb:     rdb,
		mr:      mr,
		cfg:     cfg,
		sessSvc: sessSvc,
	}
}

// applyMigrations 將 db/migrations 目錄下的所有 *.up.sql 依序套用到指定 DB。
func applyMigrations(t *testing.T, sqlDB *sql.DB) {
	t.Helper()                                                  // 標記為測試輔助函式
	migrationFiles := []string{                                 // 列出所有需要套用的 migration 檔案，相依順序與正式環境一致
		"../../db/migrations/001_init.up.sql",
		"../../db/migrations/002_add_sessions.up.sql",
		"../../db/migrations/003_add_login_events.up.sql",
		"../../db/migrations/004_add_user_ban.up.sql",
	} // 注意：測試在 internal/session 目錄下執行時，需回到專案根目錄再進入 db/migrations

	for _, path := range migrationFiles {                       // 逐一處理每個 migration
		data, err := os.ReadFile(path)                      // 讀取 SQL 檔案內容
		require.NoErrorf(t, err, "failed to read migration %s", path) // 若讀取失敗則直接中止測試

		_, err = sqlDB.Exec(string(data))                   // 直接在測試用 SQLite 上執行這段 SQL
		require.NoErrorf(t, err, "failed to apply migration %s", path) // 確保 migration 成功套用
	}
}

// createTestUser 建立一個測試用使用者，回傳建立後的 db.User。
func createTestUser(t *testing.T, env *testEnv, username, passwordHash string) db.User {
	t.Helper()                                                // 標記為測試輔助函式
	user, err := env.q.CreateUser(env.ctx, db.CreateUserParams{ // 呼叫 sqlc 產生的 CreateUser
		Username:     username,                          // 使用傳入的使用者名稱
		PasswordHash: passwordHash,                      // 使用傳入的密碼雜湊
	})
	require.NoError(t, err)                                   // 確保建立成功
	return user                                               // 回傳建立好的 user
}

// TestSessionServiceLoginSuccess 測試登入成功時：會建立 Redis session、寫入 sessions 表，並回傳正確的 user 與 sessionID。
func TestSessionServiceLoginSuccess(t *testing.T) {
	env := newTestEnv(t)                     // 建立完整測試環境

	rawPassword := "password123"            // 定義測試用明文密碼
	hashed, err := bcryptGenerate(rawPassword) // 使用與正式程式相符的 bcrypt 來產生雜湊
	require.NoError(t, err)                 // 確保加密成功

	user := createTestUser(t, env, "alice", hashed) // 在 DB 中建立一個 user

	meta := LoginMeta{                     // 準備登入時額外的紀錄資訊
		IP:        "127.0.0.1",       // 模擬來源 IP
		UserAgent: "test-agent",      // 模擬 User-Agent
	}

	u, sessionID, expiresAt, err := env.sessSvc.Login(env.ctx, "alice", rawPassword, meta) // 呼叫 Login 執行實際登入流程
	require.NoError(t, err)                        // 確保登入沒有錯誤
	require.Equal(t, user.ID, u.ID)                // 回傳的 user ID 應與資料庫中的一致
	require.NotEmpty(t, sessionID)                 // 應回傳非空的 sessionID

	require.WithinDuration(t, time.Now().Add(env.cfg.SessionTTL), expiresAt, 2*time.Second) // expiresAt 應接近現在 + TTL，容許小幅誤差

	// 檢查 Redis 中是否存在對應的 sess:{sid} 與 user_sess:{uid}。
	sessKey := infra.SessKey(sessionID)                               // 產出 sess key
	userSessKey := infra.UserSessKey(user.ID)                         // 產出 user_sess key

	data, err := env.rdb.HGetAll(env.ctx, sessKey).Result()           // 從 Redis 讀取該 session hash
	require.NoError(t, err)                                           // 操作不應失敗
	require.Equal(t, stringFromInt64(user.ID), data["user_id"])       // user_id 欄位應與登入的 user 一致

	zCount, err := env.rdb.ZCard(env.ctx, userSessKey).Result()       // 檢查 user_sess zset 內的 session 數量
	require.NoError(t, err)                                           // 操作不應失敗
	require.EqualValues(t, 1, zCount)                                 // 登入一次後應該只有一個 session

	// 檢查 SQLite sessions 表是否真的有一筆紀錄（利用原生 SQL 查詢計數）。
	var cnt int64                                                    // 用於接收 SELECT COUNT(*) 結果
	err = env.sqlDB.QueryRowContext(env.ctx, "SELECT COUNT(*) FROM sessions").Scan(&cnt) // 查詢 sessions 表筆數
	require.NoError(t, err)                                          // 查詢不應失敗
	require.EqualValues(t, 1, cnt)                                   // 預期有一筆 session 紀錄
}

// TestSessionServiceLoginInvalidPassword 測試密碼錯誤時會回傳 ErrInvalidCredentials，並且不會建立任何 session。
func TestSessionServiceLoginInvalidPassword(t *testing.T) {
	env := newTestEnv(t)                     // 建立測試環境

	hashed, err := bcryptGenerate("correct-password") // 建立與正確密碼對應的雜湊
	require.NoError(t, err)                 // 確保加密成功

	user := createTestUser(t, env, "bob", hashed) // 建立帳號 bob

	meta := LoginMeta{                     // 準備登入 meta
		IP:        "127.0.0.1",       // 模擬 IP
		UserAgent: "test-agent",      // 模擬 UA
	}

	_, sessionID, _, err := env.sessSvc.Login(env.ctx, "bob", "wrong-password", meta) // 使用錯誤密碼登入
	require.Error(t, err)                         // 應該回傳錯誤
	require.ErrorIs(t, err, ErrInvalidCredentials) // 錯誤型態應為 ErrInvalidCredentials
	require.Empty(t, sessionID)                  // 不應產出 sessionID

	// 檢查 Redis 的 user_sess zset 中不應有任何 session。
	userSessKey := infra.UserSessKey(user.ID)                                // 產出 user_sess key
	zCount, err := env.rdb.ZCard(env.ctx, userSessKey).Result()              // 讀取 zset 內成員數量
	require.NoError(t, err)                                                  // 操作不應失敗
	require.EqualValues(t, 0, zCount)                                        // 因登入失敗，不應建立任何 session
}

// TestSessionServiceLoginBannedUserDB 測試當 user 在 DB 中被標記 is_banned 時，登入應回傳 ErrUserBanned。
func TestSessionServiceLoginBannedUserDB(t *testing.T) {
	env := newTestEnv(t)                     // 建立測試環境

	hashed, err := bcryptGenerate("password") // 產生密碼雜湊
	require.NoError(t, err)                 // 確保雜湊成功

	user := createTestUser(t, env, "charlie", hashed) // 建立使用者 charlie
	err = env.q.BanUser(env.ctx, user.ID)             // 將該使用者在 DB 中標記為 is_banned = 1
	require.NoError(t, err)                           // 確保標記成功

	meta := LoginMeta{                     // 準備登入 meta
		IP:        "127.0.0.1",       // 模擬 IP
		UserAgent: "test-agent",      // 模擬 UA
	}

	_, sessionID, _, err := env.sessSvc.Login(env.ctx, "charlie", "password", meta) // 嘗試登入被 ban 的帳號
	require.Error(t, err)                      // 應該回傳錯誤
	require.ErrorIs(t, err, ErrUserBanned)     // 錯誤型態應是 ErrUserBanned
	require.Empty(t, sessionID)                // 不應產生 sessionID
}

// TestSessionServiceLoginMaxSessionsLimit 測試超過 MaxSessionsPerUser 上限時，最舊的 session 會被自動踢除。
func TestSessionServiceLoginMaxSessionsLimit(t *testing.T) {
	env := newTestEnv(t)                     // 建立測試環境

	rawPassword := "password"              // 定義測試密碼
	hashed, err := bcryptGenerate(rawPassword) // 產生對應雜湊
	require.NoError(t, err)                 // 確保雜湊成功

	user := createTestUser(t, env, "david", hashed) // 建立測試用 user

	meta := LoginMeta{                     // 建立共用 meta
		IP:        "127.0.0.1",       // 模擬 IP
		UserAgent: "test-agent",      // 模擬 UA
	}

	var sess1, sess2, sess3 string                              // 用於記錄三次登入產生的 sessionID
	_, sess1, _, err = env.sessSvc.Login(env.ctx, "david", rawPassword, meta) // 第一次登入
	require.NoError(t, err)                                       // 應登入成功
	time.Sleep(10 * time.Millisecond)                             // 稍微等待，確保 created_at 有時間差

	_, sess2, _, err = env.sessSvc.Login(env.ctx, "david", rawPassword, meta) // 第二次登入
	require.NoError(t, err)                                       // 應登入成功
	time.Sleep(10 * time.Millisecond)                             // 再等待一點時間

	_, sess3, _, err = env.sessSvc.Login(env.ctx, "david", rawPassword, meta) // 第三次登入，預期會觸發舊 session 被踢
	require.NoError(t, err)                                       // 應登入成功

	userSessKey := infra.UserSessKey(user.ID)                     // 取得 user_sess key
	sessionIDs, err := env.rdb.ZRange(env.ctx, userSessKey, 0, -1).Result() // 讀取所有 active sessionID
	require.NoError(t, err)                                       // 操作不應失敗
	require.Len(t, sessionIDs, 2)                                 // 依 config 設定，最多只保留 2 個

	require.NotContains(t, sessionIDs, sess1)                     // 最舊的 sess1 應被移除
	require.Contains(t, sessionIDs, sess2)                        // 较新的 sess2 應仍存在
	require.Contains(t, sessionIDs, sess3)                        // 最新的 sess3 應仍存在
}

// TestSessionServiceLogout 測試 Logout 會刪除 Redis 內的 session，並在 DB 中標記 revoked_by 為 "user"。
func TestSessionServiceLogout(t *testing.T) {
	env := newTestEnv(t)                     // 建立測試環境

	rawPassword := "password"              // 測試密碼
	hashed, err := bcryptGenerate(rawPassword) // 產生雜湊
	require.NoError(t, err)                 // 確保雜湊成功

	user := createTestUser(t, env, "eve", hashed) // 建立 user eve

	meta := LoginMeta{                     // 準備 meta
		IP:        "127.0.0.1",       // 模擬 IP
		UserAgent: "test-agent",      // 模擬 UA
	}

	_, sessID, _, err := env.sessSvc.Login(env.ctx, "eve", rawPassword, meta) // 先登入取得 sessionID
	require.NoError(t, err)                        // 確保登入成功

	err = env.sessSvc.Logout(env.ctx, user.ID, sessID) // 呼叫 Logout
	require.NoError(t, err)                           // Logout 本身不應回傳錯誤

	// Redis 中應已刪除對應 sess key 與 zset 成員。
	sessKey := infra.SessKey(sessID)                                   // 取得 sess key
	userSessKey := infra.UserSessKey(user.ID)                          // 取得 user_sess key

	exists, err := env.rdb.Exists(env.ctx, sessKey).Result()           // 檢查 sess hash 是否還存在
	require.NoError(t, err)                                            // 操作不應失敗
	require.EqualValues(t, 0, exists)                                  // 應該已刪除

	zCount, err := env.rdb.ZCard(env.ctx, userSessKey).Result()        // 檢查 zset 內 session 數量
	require.NoError(t, err)                                            // 操作不應失敗
	require.EqualValues(t, 0, zCount)                                  // 應該不再有任何 session

	// DB 中的 revoked_by 應被設為 "user"。
	var revokedBy sql.NullString                                       // 用來接收 revoked_by 欄位
	err = env.sqlDB.QueryRowContext(env.ctx, "SELECT revoked_by FROM sessions WHERE id = ?", sessID).Scan(&revokedBy) // 查詢該 session 的 revoked_by
	require.NoError(t, err)                                            // 查詢不應失敗
	require.True(t, revokedBy.Valid)                                   // revoked_by 應有值
	require.Equal(t, "user", revokedBy.String)                         // 值應為 "user"
}

// TestSessionServiceBanAndUnbanUser 測試 BanUser 會更新 DB 與 Redis，並踢掉所有 session；UnbanUser 則會解除 DB 與 Redis 的封鎖。
func TestSessionServiceBanAndUnbanUser(t *testing.T) {
	env := newTestEnv(t)                     // 建立測試環境

	rawPassword := "password"              // 測試密碼
	hashed, err := bcryptGenerate(rawPassword) // 產生雜湊
	require.NoError(t, err)                 // 確保雜湊成功

	user := createTestUser(t, env, "frank", hashed) // 建立 user frank

	meta := LoginMeta{                     // 準備 meta
		IP:        "127.0.0.1",       // 模擬 IP
		UserAgent: "test-agent",      // 模擬 UA
	}

	_, sessID, _, err := env.sessSvc.Login(env.ctx, "frank", rawPassword, meta) // 登入一次，產生一個 session
	require.NoError(t, err)                        // 確保登入成功
	require.NotEmpty(t, sessID)                   // 確保 sessionID 非空

	err = env.sessSvc.BanUser(env.ctx, user.ID)   // 執行 BanUser
	require.NoError(t, err)                       // BanUser 應成功

	// DB 中 is_banned 應被設為 1。
	dbUser, err := env.q.GetUserByID(env.ctx, user.ID) // 重新讀取使用者資料
	require.NoError(t, err)                            // 查詢不應失敗
	require.True(t, dbUser.IsBanned)                   // is_banned 應為 true

	// Redis 中應存在 banned_user flag，且所有 session 已被踢除。
	banKey := infra.BannedUserKey(user.ID)                                // 取得 banned flag key
	exists, err := env.rdb.Exists(env.ctx, banKey).Result()               // 檢查 banned flag 是否存在
	require.NoError(t, err)                                               // 操作不應失敗
	require.EqualValues(t, 1, exists)                                     // flag 應存在

	userSessKey := infra.UserSessKey(user.ID)                             // 取得 user_sess key
	zCount, err := env.rdb.ZCard(env.ctx, userSessKey).Result()           // 檢查 ZSet 長度
	require.NoError(t, err)                                               // 操作不應失敗
	require.EqualValues(t, 0, zCount)                                     // BanUser 會踢掉所有 session

	// 呼叫 UnbanUser 應解除 DB 與 Redis 中的 ban 狀態。
	err = env.sessSvc.UnbanUser(env.ctx, user.ID)                         // 執行 UnbanUser
	require.NoError(t, err)                                               // UnbanUser 應成功

	dbUser, err = env.q.GetUserByID(env.ctx, user.ID)                     // 再次查詢使用者狀態
	require.NoError(t, err)                                               // 查詢不應失敗
	require.False(t, dbUser.IsBanned)                                     // is_banned 應恢復為 false

	exists, err = env.rdb.Exists(env.ctx, banKey).Result()                // 檢查 Redis flag 是否已刪除
	require.NoError(t, err)                                               // 操作不應失敗
	require.EqualValues(t, 0, exists)                                     // flag 應被移除
}

// TestIsSessionValid 測試 IsSessionValid 會根據 Redis 內容與 user_id 是否一致來判斷 session 是否有效。
func TestIsSessionValid(t *testing.T) {
	env := newTestEnv(t)                     // 建立測試環境

	userID := int64(1)                      // 測試用 user ID
	sessionID := "sid-check"                // 測試用 session ID

	sessKey := infra.SessKey(sessionID)     // 產出 sess key

	// 在 Redis 建立一筆正確的 session 紀錄。
	err := env.rdb.HSet(env.ctx, sessKey, map[string]interface{}{ // 寫入 hash 欄位
		"user_id":    stringFromInt64(userID),           // user_id 與呼叫者的 userID 一致
		"created_at": time.Now().Unix(),                // 建立時間
		"expires_at": time.Now().Add(time.Hour).Unix(), // 過期時間
	}).Err()
	require.NoError(t, err)                              // 寫入不應失敗

	ok, err := env.sessSvc.IsSessionValid(env.ctx, userID, sessionID) // 檢查正確 userID 與 sessionID
	require.NoError(t, err)                              // 檢查過程不應失敗
	require.True(t, ok)                                  // session 應被視為有效

	// 使用不同的 userID 檢查，預期會因 user_id 不符而被視為無效。
	ok, err = env.sessSvc.IsSessionValid(env.ctx, userID+1, sessionID) // 換成另一個 userID
	require.NoError(t, err)                              // 檢查不應失敗
	require.False(t, ok)                                 // 因 user_id 不一致，應回傳 false

	// 若 Redis 中查不到該 sess key，則也應被視為無效。
	ok, err = env.sessSvc.IsSessionValid(env.ctx, userID, "missing-sid") // 傳入不存在的 sessionID
	require.NoError(t, err)                              // 檢查不應失敗
	require.False(t, ok)                                 // 因不存在，應回傳 false
}

// bcryptGenerate 封裝 bcrypt.GenerateFromPassword，方便在測試中重用，並與正式程式邏輯保持一致。
func bcryptGenerate(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost) // 使用預設成本參數計算雜湊
	if err != nil {                                                                  // 若計算過程發生錯誤
		return "", err                                                           // 回傳空字串與錯誤
	}
	return string(hashed), nil                                                      // 將位元組切片轉成字串回傳
}


