package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"sessionservice/internal/session"
	"sessionservice/internal/token"
)

const (
	// ContextKeyUserID 是 Gin context 裡存放 user ID 的 key。
	ContextKeyUserID    = "userID"
	ContextKeySessionID = "sessionID"
)

// NewAuthJWTMiddleware 建立一個 Gin middleware：
// - 從 Authorization: Bearer <token> 抽出 JWT
// - 使用 token.Manager 驗證簽章與過期時間
// - 解析出 userID 與 sessionID
// - 呼叫 SessionService.IsSessionValid 進一步確認 Redis session 是否仍存在
// - 將 userID / sessionID 塞進 Gin context
func NewAuthJWTMiddleware(jwtMgr *token.Manager, sessSvc *session.SessionService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing Authorization header"})
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid Authorization header"})
			return
		}

		raw := strings.TrimSpace(parts[1])
		if raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "empty token"})
			return
		}

		parsed, err := jwtMgr.Parse(raw)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		claims := parsed.Claims
		userID := claims.UserID
		sessionID := claims.SessionID
		if sessionID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid_token_no_session"})
			return
		}

		ok, err := sessSvc.IsSessionValid(c.Request.Context(), userID, sessionID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session_check_failed"})
			return
		}
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session_invalid"})
			return
		}

		c.Set(ContextKeyUserID, userID)
		c.Set(ContextKeySessionID, sessionID)
		c.Next()
	}
}


