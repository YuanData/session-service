package http

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"sessionservice/internal/session"
)

// AdminHandler 負責管理端 API（列出 sessions、踢人、ban/unban）。
type AdminHandler struct {
	sessSvc *session.SessionService
}

func NewAdminHandler(sessSvc *session.SessionService) *AdminHandler {
	return &AdminHandler{sessSvc: sessSvc}
}

// ListUserSessions 回傳某 user 的活躍 sessions（從 Redis 讀取）。
func (h *AdminHandler) ListUserSessions(c *gin.Context) {
	userID, err := parseUserIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	ctx := c.Request.Context()
	sessions, err := h.sessSvc.ListActiveSessions(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

type kickUserRequest struct {
	SessionID string `json:"session_id,omitempty"`
	All       bool   `json:"all,omitempty"`
}

// KickUserSessions 踢掉指定 user 的某個或全部 session。
func (h *AdminHandler) KickUserSessions(c *gin.Context) {
	userID, err := parseUserIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	var req kickUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	ctx := c.Request.Context()
	if req.All {
		if err := h.sessSvc.KickAllSessions(ctx, userID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to kick all sessions"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	if req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required unless all=true"})
		return
	}

	if err := h.sessSvc.KickSession(ctx, userID, req.SessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to kick session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BanUser 封鎖使用者並踢掉所有 session。
func (h *AdminHandler) BanUser(c *gin.Context) {
	userID, err := parseUserIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	if err := h.sessSvc.BanUser(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to ban user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// UnbanUser 解除封鎖使用者。
func (h *AdminHandler) UnbanUser(c *gin.Context) {
	userID, err := parseUserIDParam(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	if err := h.sessSvc.UnbanUser(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unban user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func parseUserIDParam(c *gin.Context) (int64, error) {
	idStr := c.Param("id")
	return strconv.ParseInt(idStr, 10, 64)
}


