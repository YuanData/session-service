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
// 處理 /health, /auth/*, /me, 以及 /admin/* 管理端 API。
func NewRouter(
	q *db.Queries,
	jwtMgr *token.Manager,
	sessSvc *session.SessionService,
	tokenTTL time.Duration,
	adminAPIKey string,
) *gin.Engine {
	r := gin.Default()

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	authHandler := NewAuthHandler(q, jwtMgr, sessSvc, tokenTTL)
	adminHandler := NewAdminHandler(sessSvc)

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

	// Admin routes（用簡單的 API key middleware 保護）
	adminGroup := r.Group("/admin")
	adminGroup.Use(middleware.NewAdminAPIKeyMiddleware(adminAPIKey))
	{
		adminGroup.GET("/users/:id/sessions", adminHandler.ListUserSessions)
		adminGroup.POST("/users/:id/kick", adminHandler.KickUserSessions)
		adminGroup.POST("/users/:id/ban", adminHandler.BanUser)
		adminGroup.POST("/users/:id/unban", adminHandler.UnbanUser)
	}

	return r
}


