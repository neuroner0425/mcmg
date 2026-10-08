package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"minecraft_server_manager/internal/middleware"
)

// LoginRequest defines login payload requiring both nickname and password.
type LoginRequest struct {
	Nickname string `json:"nickname" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login handles credential verification and issues a JWT cookie with nickname.
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nickname and password are required"})
		return
	}

	trimmedNick := strings.TrimSpace(req.Nickname)
	if trimmedNick == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nickname cannot be empty"})
		return
	}

	var role string
	if req.Password == h.cfg.Security.AdminPassword {
		role = middleware.RoleAdmin
	} else if req.Password == h.cfg.Security.UserPassword {
		role = middleware.RoleUser
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "incorrect password"})
		return
	}

	tokenStr, err := middleware.GenerateToken(trimmedNick, role, h.cfg.Security.JWTSecret, 24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate authentication token"})
		return
	}

	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.CookieKey, tokenStr, 86400*7, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"message":  "login successful",
		"role":     role,
		"nickname": trimmedNick,
	})
}

// Logout clears the JWT token cookie.
func (h *Handler) Logout(c *gin.Context) {
	c.SetCookie(middleware.CookieKey, "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "logged out successfully"})
}

// GetMe returns current authenticated user info.
func (h *Handler) GetMe(c *gin.Context) {
	roleVal, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	nicknameVal, _ := c.Get("nickname")
	role, _ := roleVal.(string)
	nickname, _ := nicknameVal.(string)

	c.JSON(http.StatusOK, gin.H{
		"role":        role,
		"nickname":    nickname,
		"map_url":     "/squaremap/",
		"bluemap_url": "/squaremap/",
	})
}
