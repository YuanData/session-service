package middleware

import (
	"net/http"           // 匯入 net/http，提供 HTTP 狀態碼常數
	"net/http/httptest" // 匯入 httptest，用於建立 HTTP 測試伺服器與請求
	"testing"            // 匯入 testing 套件，提供單元測試框架

	"github.com/gin-gonic/gin"     // 匯入 gin，建立測試用路由與 handler
	"github.com/stretchr/testify/require" // 匯入 testify/require，用於撰寫斷言
)

// TestAdminAPIKeyMiddleware_NoKeyConfigured 測試當 adminKey 為空時，middleware 應直接放行所有請求。
func TestAdminAPIKeyMiddleware_NoKeyConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)                           // 將 Gin 設為測試模式，避免多餘輸出
	r := gin.New()                                      // 建立新的 Gin Engine
	r.Use(NewAdminAPIKeyMiddleware(""))                 // 掛上 adminKey 為空的 middleware（應無條件放行）
	r.GET("/admin/ping", func(c *gin.Context) {         // 註冊測試用路由 /admin/ping
		c.JSON(http.StatusOK, gin.H{"ok": true})    // 回傳 200 OK 與簡單 JSON
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil) // 建立 GET /admin/ping 測試請求
	w := httptest.NewRecorder()                                    // 建立 ResponseRecorder，用於捕捉回應

	r.ServeHTTP(w, req)                             // 送出請求給測試用 router
	require.Equal(t, http.StatusOK, w.Code)         // 斷言狀態碼應為 200，代表 middleware 沒有阻擋
}

// TestAdminAPIKeyMiddleware_Forbidden 測試設定了 adminKey 但未帶正確 header 時，應回傳 403。
func TestAdminAPIKeyMiddleware_Forbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)                          // 設為測試模式
	r := gin.New()                                     // 建立 Gin Engine
	r.Use(NewAdminAPIKeyMiddleware("secret-key"))      // 設定 adminKey 為 "secret-key"
	r.GET("/admin/ping", func(c *gin.Context) {        // 註冊測試路由
		c.JSON(http.StatusOK, gin.H{"ok": true})   // 若真的進到 handler 會回傳 200
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil) // 建立不帶 header 的請求
	w := httptest.NewRecorder()                                    // 建立 ResponseRecorder

	r.ServeHTTP(w, req)                                    // 執行請求
	require.Equal(t, http.StatusForbidden, w.Code)         // 斷言狀態碼應為 403 Forbidden
}

// TestAdminAPIKeyMiddleware_Success 測試帶上正確 X-Admin-Token header 時，應順利通過。
func TestAdminAPIKeyMiddleware_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)                          // 設為測試模式
	r := gin.New()                                     // 建立 Gin Engine
	r.Use(NewAdminAPIKeyMiddleware("secret-key"))      // 設定正確的 adminKey
	r.GET("/admin/ping", func(c *gin.Context) {        // 註冊測試路由
		c.JSON(http.StatusOK, gin.H{"ok": true})   // 正常 handler 回應 200
	})

	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil) // 建立請求
	req.Header.Set("X-Admin-Token", "secret-key")                   // 在 header 中帶上正確的 admin token
	w := httptest.NewRecorder()                                     // 建立 ResponseRecorder

	r.ServeHTTP(w, req)                                    // 執行請求
	require.Equal(t, http.StatusOK, w.Code)                // 斷言應該通過並回傳 200
}


