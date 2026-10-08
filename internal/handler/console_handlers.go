package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ExecuteCommand executes a raw console command (Admin only).
func (h *Handler) ExecuteCommand(c *gin.Context) {
	type CommandRequest struct {
		Command string `json:"command" binding:"required"`
	}
	var req CommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
		return
	}

	output, err := h.mcService.Execute(req.Command)
	if err != nil {
		if h.procMgr != nil {
			if inErr := h.procMgr.WriteStdin(req.Command); inErr == nil {
				c.JSON(http.StatusOK, gin.H{
					"command": req.Command,
					"output":  fmt.Sprintf("[서버 콘솔(stdin)으로 명령어를 전달했습니다]: %s", req.Command),
				})
				return
			}
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"command": req.Command,
		"output":  output,
	})
}

// GetProcessLogs retrieves in-memory server console logs.
func (h *Handler) GetProcessLogs(c *gin.Context) {
	limitStr := c.DefaultQuery("limit", "150")
	limit, _ := strconv.Atoi(limitStr)
	logs := h.procMgr.GetLogs(limit)
	c.JSON(http.StatusOK, gin.H{"logs": logs})
}
