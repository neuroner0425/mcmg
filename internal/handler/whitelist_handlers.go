package handler

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
)

// GetWhitelist returns current whitelist status and entries.
func (h *Handler) GetWhitelist(c *gin.Context) {
	status, err := h.playerMgmt.GetWhitelistStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

// WhitelistConfigReq defines configuration payload.
type WhitelistConfigReq struct {
	Enabled bool `json:"enabled"`
	Enforce bool `json:"enforce"`
}

// UpdateWhitelistConfig toggles white-list and enforce-whitelist settings.
func (h *Handler) UpdateWhitelistConfig(c *gin.Context) {
	var req WhitelistConfigReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.playerMgmt.SetWhitelistConfig(req.Enabled, req.Enforce); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "화이트리스트 설정이 저장되었습니다."})
}

// WhitelistNameReq defines player name payload for whitelist operations.
type WhitelistNameReq struct {
	Name string `json:"name" binding:"required"`
}

// AddWhitelistPlayer manually adds a player to the whitelist (Admin only).
func (h *Handler) AddWhitelistPlayer(c *gin.Context) {
	var req WhitelistNameReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if err := h.playerMgmt.AddWhitelist(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("플레이어 '%s'이(가) 화이트리스트에 등록되었습니다.", req.Name)})
}

// RegisterSelfWhitelist allows standard users to self-register their own Minecraft username to whitelist without list management access.
func (h *Handler) RegisterSelfWhitelist(c *gin.Context) {
	var req WhitelistNameReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "플레이어 닉네임을 입력해주세요."})
		return
	}
	cleanName := strings.TrimSpace(req.Name)
	if len(cleanName) < 3 || len(cleanName) > 16 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "마인크래프트 닉네임은 3~16자여야 합니다."})
		return
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_]{3,16}$`, cleanName)
	if !matched {
		c.JSON(http.StatusBadRequest, gin.H{"error": "닉네임은 영문 대소문자, 숫자, 밑줄(_)만 가능합니다."})
		return
	}
	if err := h.playerMgmt.AddWhitelist(cleanName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("플레이어 '%s'이(가) 화이트리스트에 성공적으로 등록되었습니다.", cleanName)})
}

// RemoveWhitelistPlayer removes a player from the whitelist (Admin only).
func (h *Handler) RemoveWhitelistPlayer(c *gin.Context) {
	var req WhitelistNameReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if err := h.playerMgmt.RemoveWhitelist(req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("플레이어 '%s'이(가) 화이트리스트에서 제거되었습니다.", req.Name)})
}
