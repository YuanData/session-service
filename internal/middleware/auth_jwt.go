package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"sessionservice/internal/token"
)

const (
	// ContextKeyUserID 是 Gin context 裡存放 user ID 的 key。
	ContextKeyUserID = "userID"
)

// NewAuthJWTMiddleware 建立一個 Gin middleware：
// - 從 Authorization: Bearer <token> 抽出 JWT
// - 使用 token.Manager 驗證簽章與過期時間
// - 解析出 userID，塞進 Gin context
func NewAuthJWTMiddleware(jwtMgr *token.Manager) gin.HandlerFunc {
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

		c.Set(ContextKeyUserID, parsed.Claims.UserID)
		c.Next()
	}
}


