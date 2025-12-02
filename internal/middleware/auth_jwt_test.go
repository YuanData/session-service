package middleware

import (
	"context"              // 匯入 context，用於 Redis 與 SessionService 呼叫
	"net/http"             // 匯入 net/http，提供 HTTP 方法與狀態碼常數
	"net/http/httptest"    // 匯入 httptest，建立 HTTP 測試伺服器與請求
	"testing"              // 匯入 testing 套件，提供單元測試框架
	"time"                 // 匯入 time，用於設定與檢查 JWT 過期時間

	"github.com/alicebob/miniredis/v2" // 匯入 miniredis，提供記憶體內的 Redis 測試伺服器
	"github.com/gin-gonic/gin"         // 匯入 gin，建立測試用路由與 middleware
	"github.com/redis/go-redis/v9"     // 匯入 go-redis，用於連線到 miniredis
	"github.com/stretchr/testify/require" // 匯入 testify/require，簡化斷言撰寫

	"sessionservice/internal/config" // 匯入 config 套件，建立測試用設定
	"sessionservice/internal/infra"  // 匯入 infra 套件，產生 Redis key
	"sessionservice/internal/session" // 匯入 session 套件，建立 SessionService
	"sessionservice/internal/token"   // 匯入 token 套件，產生與解析 JWT
)

// newTestSessionService 建立一個只用於測試 middleware 的 SessionService。
// 這個 SessionService 只會使用到 Redis 與 Config，不會觸發資料庫或 Asynq 的邏輯。
func newTestSessionService(t *testing.T) (*session.SessionService, *token.Manager, *miniredis.Miniredis, *redis.Client) {
	t.Helper() // 標記為測試輔助函式，錯誤行號會指向呼叫端

	mr, err := miniredis.Run()             // 啟動一個記憶體內的 Redis 測試實例
	require.NoError(t, err)                // 確保啟動成功

	rdb := redis.NewClient(&redis.Options{ // 使用 go-redis 連線到剛啟動的 miniredis
		Addr: mr.Addr(),               // 設定位址為 miniredis 提供的位址
		DB:   0,                       // 使用預設 DB 0
	})

	cfg := &config.Config{                // 建立一份只包含本測試需要欄位的設定
		SessionTTL:         time.Hour, // 將 Session TTL 設為 1 小時
		MaxSessionsPerUser: 10,        // 測試中不需觸發 session 上限
	}

	sessSvc := session.NewSessionService(nil, rdb, cfg, nil) // 建立 SessionService，資料庫與 Asynq 參數傳入 nil 即可
	jwtMgr := token.NewManager("test-secret", time.Hour)     // 建立 JWT Manager，測試用密鑰與 TTL

	return sessSvc, jwtMgr, mr, rdb // 回傳 SessionService、JWT Manager、miniredis handler 與 Redis client，以便測試使用與關閉
}

// setupAuthRoute 建立一條掛上 AuthJWT middleware 的測試路由。
func setupAuthRoute(jwtMgr *token.Manager, sessSvc *session.SessionService) *gin.Engine {
	gin.SetMode(gin.TestMode)                                   // 設定 Gin 為測試模式
	r := gin.New()                                              // 建立新的 Gin Engine
	r.Use(NewAuthJWTMiddleware(jwtMgr, sessSvc))                // 在全域掛上 JWT 驗證 middleware
	r.GET("/me", func(c *gin.Context) {                         // 建立測試用的 /me 路由
		userID, _ := c.Get(ContextKeyUserID)                // 從 context 取出 userID
		sessionID, _ := c.Get(ContextKeySessionID)          // 從 context 取出 sessionID
		c.JSON(http.StatusOK, gin.H{                        // 回應 200，並把兩個值回傳，方便驗證
			"user_id":    userID,
			"session_id": sessionID,
		})
	})
	return r // 回傳設定完成的 router
}

// TestAuthJWTMiddleware_Success 測試帶入合法 JWT 並在 Redis 中存在對應 session 時，請求應通過且 context 被正確填值。
func TestAuthJWTMiddleware_Success(t *testing.T) {
	sessSvc, jwtMgr, mr, rdb := newTestSessionService(t) // 建立測試用 SessionService / JWT Manager / miniredis / Redis client
	defer mr.Close()                                     // 測試結束時關閉 miniredis
	defer rdb.Close()                                    // 測試結束時關閉 Redis client

	ctx := context.Background()                   // 建立背景 context，用於 Redis 操作
	userID := int64(100)                          // 測試用 user ID
	sessionID := "sid-success"                    // 測試用 session ID

	// 在 Redis 中預先寫入一筆對應的 session 資料，讓 IsSessionValid 可以通過。
	err := rdb.HSet(ctx, infra.SessKey(sessionID), map[string]interface{}{
		"user_id":    userID,           // 存入 user_id 欄位
		"created_at": time.Now().Unix(), // 存入建立時間
		"expires_at": time.Now().Add(time.Hour).Unix(), // 存入過期時間
	}).Err()
	require.NoError(t, err) // 確保 Redis 寫入成功

	// 產生帶有對應 userID 與 sessionID 的 JWT，過期時間設為未來。
	tokenStr, err := jwtMgr.GenerateWithSession(userID, sessionID, time.Now().Add(time.Hour))
	require.NoError(t, err) // 產生 token 不應失敗

	r := setupAuthRoute(jwtMgr, sessSvc)                              // 建立掛好 middleware 與測試 handler 的 router
	req := httptest.NewRequest(http.MethodGet, "/me", nil)            // 準備呼叫 /me 的 HTTP 請求
	req.Header.Set("Authorization", "Bearer "+tokenStr)               // 在 header 中帶入合法的 Bearer token
	w := httptest.NewRecorder()                                       // 建立 ResponseRecorder 捕捉回應

	r.ServeHTTP(w, req)                                               // 執行請求
	require.Equal(t, http.StatusOK, w.Code)                           // 斷言狀態碼為 200，代表 middleware 放行
	require.Contains(t, w.Body.String(), `"user_id":100`)             // 回應 JSON 應包含正確的 user_id
	require.Contains(t, w.Body.String(), `"session_id":"sid-success"`) // 回應 JSON 應包含正確的 session_id
}

// TestAuthJWTMiddleware_MissingHeader 測試缺少 Authorization header 時，應直接回傳 401。
func TestAuthJWTMiddleware_MissingHeader(t *testing.T) {
	sessSvc, jwtMgr, mr, rdb := newTestSessionService(t) // 建立測試用 SessionService 與 JWT Manager
	defer mr.Close()                                     // 測試結束關閉 miniredis
	defer rdb.Close()                                    // 測試結束關閉 Redis client

	r := setupAuthRoute(jwtMgr, sessSvc)          // 建立測試 router
	req := httptest.NewRequest(http.MethodGet, "/me", nil) // 建立未帶 Authorization header 的請求
	w := httptest.NewRecorder()                            // 建立 ResponseRecorder

	r.ServeHTTP(w, req)                              // 執行請求
	require.Equal(t, http.StatusUnauthorized, w.Code) // 斷言為 401 Unauthorized
}

// TestAuthJWTMiddleware_InvalidHeaderFormat 測試 Authorization header 格式錯誤時，應回傳 401。
func TestAuthJWTMiddleware_InvalidHeaderFormat(t *testing.T) {
	sessSvc, jwtMgr, mr, rdb := newTestSessionService(t) // 建立測試用 SessionService 與 JWT Manager
	defer mr.Close()                                     // 測試結束關閉 miniredis
	defer rdb.Close()                                    // 測試結束關閉 Redis client

	r := setupAuthRoute(jwtMgr, sessSvc)                          // 建立測試 router
	req := httptest.NewRequest(http.MethodGet, "/me", nil)        // 建立請求
	req.Header.Set("Authorization", "Token something")            // 使用錯誤的前綴 Token 而非 Bearer
	w := httptest.NewRecorder()                                   // 建立 ResponseRecorder

	r.ServeHTTP(w, req)                                           // 執行請求
	require.Equal(t, http.StatusUnauthorized, w.Code)             // 斷言為 401 Unauthorized
}

// TestAuthJWTMiddleware_EmptyToken 測試 Authorization: Bearer 後面是空字串時，應回傳 401。
func TestAuthJWTMiddleware_EmptyToken(t *testing.T) {
	sessSvc, jwtMgr, mr, rdb := newTestSessionService(t) // 建立測試用 SessionService 與 JWT Manager
	defer mr.Close()                                     // 測試結束關閉 miniredis
	defer rdb.Close()                                    // 測試結束關閉 Redis client

	r := setupAuthRoute(jwtMgr, sessSvc)                          // 建立測試 router
	req := httptest.NewRequest(http.MethodGet, "/me", nil)        // 建立請求
	req.Header.Set("Authorization", "Bearer   ")                  // 帶入只有空白的 Bearer token
	w := httptest.NewRecorder()                                   // 建立 ResponseRecorder

	r.ServeHTTP(w, req)                                           // 執行請求
	require.Equal(t, http.StatusUnauthorized, w.Code)             // 斷言為 401 Unauthorized
}

// TestAuthJWTMiddleware_NoSessionIDInToken 測試 JWT 存在但 claims 中沒有 sessionID（使用 Generate）時，應回傳 401。
func TestAuthJWTMiddleware_NoSessionIDInToken(t *testing.T) {
	sessSvc, jwtMgr, mr, rdb := newTestSessionService(t) // 建立測試用 SessionService 與 JWT Manager
	defer mr.Close()                                     // 測試結束關閉 miniredis
	defer rdb.Close()                                    // 測試結束關閉 Redis client

	// 使用 Manager.Generate 產生不含 sessionID 的 token
	tokenStr, err := jwtMgr.Generate(1)
	require.NoError(t, err) // 確保產生成功

	r := setupAuthRoute(jwtMgr, sessSvc)                          // 建立測試 router
	req := httptest.NewRequest(http.MethodGet, "/me", nil)        // 建立請求
	req.Header.Set("Authorization", "Bearer "+tokenStr)           // 帶入不含 sessionID 的 token
	w := httptest.NewRecorder()                                   // 建立 ResponseRecorder

	r.ServeHTTP(w, req)                                           // 執行請求
	require.Equal(t, http.StatusUnauthorized, w.Code)             // 斷言為 401 Unauthorized
}

// TestAuthJWTMiddleware_SessionInvalid 測試 JWT 合法但 Redis 中沒有對應 session 時，應視為無效 session。
func TestAuthJWTMiddleware_SessionInvalid(t *testing.T) {
	sessSvc, jwtMgr, mr, rdb := newTestSessionService(t) // 建立測試用 SessionService 與 JWT Manager
	defer mr.Close()                                     // 測試結束關閉 miniredis
	defer rdb.Close()                                    // 測試結束關閉 Redis client

	// 產生一顆帶有 sessionID 但實際上 Redis 並不存在該 sess key 的 token
	tokenStr, err := jwtMgr.GenerateWithSession(10, "missing-sid", time.Now().Add(time.Hour))
	require.NoError(t, err) // 產生 token 不應失敗

	r := setupAuthRoute(jwtMgr, sessSvc)                          // 建立測試 router
	req := httptest.NewRequest(http.MethodGet, "/me", nil)        // 建立請求
	req.Header.Set("Authorization", "Bearer "+tokenStr)           // 在 header 中帶入 token
	w := httptest.NewRecorder()                                   // 建立 ResponseRecorder

	r.ServeHTTP(w, req)                                           // 執行請求
	require.Equal(t, http.StatusUnauthorized, w.Code)             // 因為 Redis 中沒有對應 session，應回傳 401
}


