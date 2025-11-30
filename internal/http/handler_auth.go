package http

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"sessionservice/internal/db"
	"sessionservice/internal/middleware"
	"sessionservice/internal/session"
	"sessionservice/internal/token"
)

// AuthHandler 負責處理與帳號/登入相關的 HTTP 請求。
type AuthHandler struct {
	q         *db.Queries
	jwtMgr    *token.Manager
	sessSvc   *session.SessionService
	tokenTTL  time.Duration
}

// NewAuthHandler 建立 AuthHandler。
func NewAuthHandler(q *db.Queries, jwtMgr *token.Manager, sessSvc *session.SessionService, tokenTTL time.Duration) *AuthHandler {
	return &AuthHandler{
		q:        q,
		jwtMgr:   jwtMgr,
		sessSvc:  sessSvc,
		tokenTTL: tokenTTL,
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

	meta := session.LoginMeta{
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
	}

	user, sessionID, expiresAt, err := h.sessSvc.Login(ctx, req.Username, req.Password, meta)
	if err != nil {
		if err == session.ErrInvalidCredentials {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "login failed"})
		return
	}

	tokenStr, err := h.jwtMgr.GenerateWithSession(user.ID, sessionID, expiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, loginResponse{
		AccessToken: tokenStr,
		ExpiresIn:   int64(h.tokenTTL.Seconds()),
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

// Logout：從 context 取得 userID / sessionID，呼叫 SessionService.Logout。
func (h *AuthHandler) Logout(c *gin.Context) {
	userIDVal, ok := c.Get(middleware.ContextKeyUserID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing user in context"})
		return
	}
	sessionIDVal, ok := c.Get(middleware.ContextKeySessionID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing session in context"})
		return
	}

	userID, ok := userIDVal.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id type"})
		return
	}
	sessionID, ok := sessionIDVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session id type"})
		return
	}

	if err := h.sessSvc.Logout(c.Request.Context(), userID, sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "logout failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}


