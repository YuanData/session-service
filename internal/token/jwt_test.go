package token

import (
	"testing" // 匯入 testing 套件，提供單元測試基礎工具
	"time"    // 匯入 time 套件，用來檢查 JWT 時間相關欄位

	"github.com/stretchr/testify/require" // 匯入 testify/require，方便進行斷言與錯誤檢查
)

// TestManagerGenerateAndParse 測試使用 Manager.Generate 產生 JWT，並透過 Parse 正確解析出 Claims。
func TestManagerGenerateAndParse(t *testing.T) {
	secret := "test-secret"               // 測試用的 JWT 簽章密鑰
	ttl := time.Hour                      // 設定 token 存活時間為 1 小時
	mgr := NewManager(secret, ttl)        // 依照密鑰與 TTL 建立 JWT Manager
	userID := int64(42)                   // 測試用的 user ID
	tokenStr, err := mgr.Generate(userID) // 呼叫 Generate 產生不含 sessionID 的 JWT
	require.NoError(t, err)               // 斷言產生過程不應該出錯
	require.NotEmpty(t, tokenStr)         // 斷言回傳的 token 字串不應為空

	parsed, err := mgr.Parse(tokenStr) // 使用同一個 Manager 對剛產生的 token 進行解析
	require.NoError(t, err)           // 斷言解析過程不應該出錯
	require.NotNil(t, parsed)         // 斷言解析結果物件不應為 nil

	claims := parsed.Claims                     // 取得解析後的 Claims
	require.Equal(t, userID, claims.UserID)     // 斷言 sub (UserID) 與原本設定一致
	require.Equal(t, "", claims.SessionID)      // 使用 Generate 時 SessionID 應為空字串
	require.NotNil(t, claims.ExpiresAt)         // ExpiresAt 應該被設定
	require.NotNil(t, claims.IssuedAt)          // IssuedAt 應該被設定
	require.True(t, claims.ExpiresAt.After(claims.IssuedAt.Time)) // 斷言過期時間應晚於發行時間
}

// TestManagerGenerateWithSession 測試 GenerateWithSession 會把指定的 sessionID 與 expiresAt 正確寫入 Claims。
func TestManagerGenerateWithSession(t *testing.T) {
	secret := "another-secret"                         // 測試用的另一組密鑰
	mgr := NewManager(secret, time.Hour)               // 建立 Manager，這裡的 ttl 僅用於預設，不影響 GenerateWithSession 的 expiresAt
	userID := int64(7)                                 // 測試用 user ID
	sessionID := "sess-123"                            // 測試用 session ID
	expiresAt := time.Now().Add(2 * time.Hour).Truncate(time.Second) // 預期過期時間，取秒精度避免時間差異

	tokenStr, err := mgr.GenerateWithSession(userID, sessionID, expiresAt) // 產生帶有 sessionID 與指定過期時間的 JWT
	require.NoError(t, err)                                               // 斷言產生過程不應有錯
	require.NotEmpty(t, tokenStr)                                         // 斷言 token 字串不為空

	parsed, err := mgr.Parse(tokenStr) // 對產生出的 token 做解析
	require.NoError(t, err)           // 斷言解析正常
	require.NotNil(t, parsed)         // 解析結果不為 nil

	claims := parsed.Claims                                   // 取得 Claims
	require.Equal(t, userID, claims.UserID)                   // 確認 sub 與輸入的 userID 一致
	require.Equal(t, sessionID, claims.SessionID)             // 確認 sid 與輸入的 sessionID 一致
	require.WithinDuration(t, expiresAt, claims.ExpiresAt.Time, time.Second) // 容許 1 秒內的小誤差比對 expiresAt
}

// TestManagerParseInvalidToken 測試 Parse 對於明顯錯誤的 token 字串必須回傳錯誤。
func TestManagerParseInvalidToken(t *testing.T) {
	mgr := NewManager("secret", time.Hour) // 建立 Manager，測試重點在 Parse 行為

	parsed, err := mgr.Parse("not-a-valid-jwt") // 傳入明顯不是 JWT 的字串
	require.Error(t, err)                       // 斷言應該回傳錯誤
	require.Nil(t, parsed)                      // 解析結果應為 nil
}


