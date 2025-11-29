package http

import (
	"github.com/gin-gonic/gin"

	"sessionservice/internal/db"
	"sessionservice/internal/middleware"
	"sessionservice/internal/token"
)

// NewRouter 建立並回傳一個已註冊好路由的 *gin.Engine。
// Phase 1：只處理 /health, /auth/signup, /auth/login, /me。
func NewRouter(q *db.Queries, jwtMgr *token.Manager) *gin.Engine {
	r := gin.Default()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	authHandler := NewAuthHandler(q, jwtMgr)

	auth := r.Group("/auth")
	{
		auth.POST("/signup", authHandler.Signup)
		auth.POST("/login", authHandler.Login)
	}

	// 需要 JWT 的路由
	authRequired := r.Group("/")
	authRequired.Use(middleware.NewAuthJWTMiddleware(jwtMgr))
	{
		authRequired.GET("/me", authHandler.Me)
	}

	return r
}


