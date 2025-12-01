package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// NewAdminAPIKeyMiddleware 檢查 X-Admin-Token 是否與設定值相符。
func NewAdminAPIKeyMiddleware(adminKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if adminKey == "" {
			// 若沒設定 admin key，仍允許請求通過，但建議只在本地開發時使用。
			c.Next()
			return
		}

		token := c.GetHeader("X-Admin-Token")
		if token == "" || token != adminKey {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error": "forbidden",
			})
			return
		}

		c.Next()
	}
}


