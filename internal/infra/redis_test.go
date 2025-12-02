package infra

import (
	"testing" // 匯入 testing 套件，提供單元測試支援

	"github.com/stretchr/testify/require" // 匯入 testify/require，用於簡潔撰寫斷言
)

// TestSessKey 測試 SessKey 是否依照預期組出 Redis session key。
func TestSessKey(t *testing.T) {
	sessionID := "abc123"                      // 構造一個測試用 session ID
	key := SessKey(sessionID)                  // 呼叫被測函式產生 Redis key
	require.Equal(t, "sess:abc123", key)       // 斷言結果必須符合預期格式
}

// TestUserSessKey 測試 UserSessKey 是否依照預期組出 user_sess key。
func TestUserSessKey(t *testing.T) {
	userID := int64(42)                        // 測試用 user ID
	key := UserSessKey(userID)                 // 產生對應的 Redis key
	require.Equal(t, "user_sess:42", key)      // 檢查 key 字串是否正確
}

// TestBannedUserKey 測試 BannedUserKey 是否依照預期組出 banned_user key。
func TestBannedUserKey(t *testing.T) {
	userID := int64(7)                         // 測試用 user ID
	key := BannedUserKey(userID)               // 呼叫函式產生 banned flag key
	require.Equal(t, "banned_user:7", key)     // 斷言 key 與預期值一致
}


