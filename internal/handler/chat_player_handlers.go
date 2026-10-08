package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"minecraft_server_manager/internal/mcservice"
)

// GetPlayers returns the current player list.
func (h *Handler) GetPlayers(c *gin.Context) {
	players, online, max, err := h.mcService.GetPlayers()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"error":   "server offline or query failed: " + err.Error(),
			"online":  0,
			"max":     0,
			"players": []string{},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"online":  online,
		"max":     max,
		"players": players,
	})
}

// GetChatMessages retrieves live in-game and web chat messages since a specific message ID.
func (h *Handler) GetChatMessages(c *gin.Context) {
	sinceStr := c.DefaultQuery("since", "0")
	sinceID, _ := strconv.ParseInt(sinceStr, 10, 64)

	messages := h.chatSvc.GetMessagesSince(sinceID)
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

// ChatRequest defines chat payload.
type ChatRequest struct {
	Sender  string `json:"sender"`
	Message string `json:"message" binding:"required"`
}

// SendChat broadcasts a chat message into the Minecraft server and adds it to the chat service.
func (h *Handler) SendChat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}

	sender := strings.TrimSpace(req.Sender)
	if sender == "" {
		if nick, exists := c.Get("nickname"); exists {
			sender = fmt.Sprintf("%v", nick)
		}
	}
	if sender == "" {
		sender = "WebUser"
	}

	// 1. Broadcast into Minecraft via RCON
	if err := h.mcService.SendChatMessage(sender, req.Message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 2. Ingest into internal chat service so web immediately displays it
	msg := h.chatSvc.AddMessage(sender, req.Message, false)

	c.JSON(http.StatusOK, gin.H{"status": "message broadcasted", "message": msg})
}

// ExecutePlayerAction executes player management operations (kick, ban, op, gamemode, etc.).
func (h *Handler) ExecutePlayerAction(c *gin.Context) {
	var req mcservice.PlayerActionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.playerMgmt.ExecutePlayerAction(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":  "작업이 성공적으로 실행되었습니다.",
		"response": resp,
	})
}
