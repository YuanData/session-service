package http

import (
	"time"

	"github.com/gin-gonic/gin"

	"sessionservice/internal/db"
	"sessionservice/internal/middleware"
	"sessionservice/internal/session"
	"sessionservice/internal/token"
)

// NewRouter 建立並回傳一個已註冊好路由的 *gin.Engine。
// Phase 2：處理 /health, /auth/signup, /auth/login, /auth/logout, /me。
func NewRouter(q *db.Queries, jwtMgr *token.Manager, sessSvc *session.SessionService, tokenTTL time.Duration) *gin.Engine {
	r := gin.Default()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	authHandler := NewAuthHandler(q, jwtMgr, sessSvc, tokenTTL)

	// 不需驗證的 auth 路由
	auth := r.Group("/auth")
	{
		auth.POST("/signup", authHandler.Signup)
		auth.POST("/login", authHandler.Login)
	}

	// 需要 JWT 的路由
	authRequired := r.Group("/")
	authRequired.Use(middleware.NewAuthJWTMiddleware(jwtMgr, sessSvc))
	{
		authRequired.GET("/me", authHandler.Me)
		authRequired.POST("/auth/logout", authHandler.Logout)
	}

	return r
}


