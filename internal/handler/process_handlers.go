package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetProcessStatus returns the current server process state (status, PID, uptime, memory, CPU, etc.).
func (h *Handler) GetProcessStatus(c *gin.Context) {
	status := h.procMgr.GetStatus()
	c.JSON(http.StatusOK, status)
}

// StartProcess launches the Minecraft server process.
func (h *Handler) StartProcess(c *gin.Context) {
	if err := h.procMgr.Start(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "서버 프로세스를 시작합니다."})
}

// StopProcess gracefully terminates the Minecraft server process.
func (h *Handler) StopProcess(c *gin.Context) {
	if err := h.procMgr.Stop(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "서버 프로세스를 안전하게 종료합니다."})
}

// RestartProcess stops and re-launches the Minecraft server process asynchronously.
func (h *Handler) RestartProcess(c *gin.Context) {
	go func() {
		_ = h.procMgr.Restart()
	}()
	c.JSON(http.StatusOK, gin.H{"message": "서버를 재시작합니다."})
}
