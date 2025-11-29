package http

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"sessionservice/internal/db"
	"sessionservice/internal/middleware"
	"sessionservice/internal/token"
)

// AuthHandler 負責處理與帳號/登入相關的 HTTP 請求。
type AuthHandler struct {
	q      *db.Queries
	jwtMgr *token.Manager
}

// NewAuthHandler 建立 AuthHandler。
func NewAuthHandler(q *db.Queries, jwtMgr *token.Manager) *AuthHandler {
	return &AuthHandler{
		q:      q,
		jwtMgr: jwtMgr,
	}
}

type signupRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Signup 處理使用者註冊。
func (h *AuthHandler) Signup(c *gin.Context) {
	var req signupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	ctx := c.Request.Context()
	user, err := h.q.CreateUser(ctx, db.CreateUserParams{
		Username:     req.Username,
		PasswordHash: string(hashed),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to create user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":       user.ID,
		"username": user.Username,
	})
}

type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"` // seconds
}

// Login 處理登入並回傳 JWT。
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	ctx := c.Request.Context()
	user, err := h.q.GetUserByUsername(ctx, req.Username)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query user"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	tokenStr, err := h.jwtMgr.Generate(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// jwtMgr 的 TTL 是在建立時設定的，這裡示意用 24h。
	c.JSON(http.StatusOK, loginResponse{
		AccessToken: tokenStr,
		ExpiresIn:   h.jwtMgrTTL(),
	})
}

// Me 回傳目前登入使用者的簡單資訊。
func (h *AuthHandler) Me(c *gin.Context) {
	userIDVal, ok := c.Get(middleware.ContextKeyUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user in context"})
		return
	}

	userID, ok := userIDVal.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id type"})
		return
	}

	ctx := c.Request.Context()
	user, err := h.q.GetUserByID(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":       user.ID,
		"username": user.Username,
		"created":  user.CreatedAt,
	})
}

// h.jwtMgr 的 TTL 我們沒有對外暴露，這裡簡單寫死 24h，確保回應有值。
// 若你想更精準，可以將 TTL 存在 config 並在 handler 也保存。
func (h *AuthHandler) jwtMgrTTL() int64 {
	// 24 hours
	return 24 * 60 * 60
}


