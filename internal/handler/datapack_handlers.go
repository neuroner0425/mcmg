package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"minecraft_server_manager/internal/mcservice"
)

// GetDatapacks returns the current enabled/disabled datapacks and experimental gameplay toggles.
func (h *Handler) GetDatapacks(c *gin.Context) {
	var rcon mcservice.RCONClient
	if h.procMgr != nil && h.procMgr.GetStatus().Status == mcservice.StatusRunning {
		rcon = h.mcService
	}
	status, err := h.datapackMgr.GetStatus(rcon)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "데이터팩 상태 조회 실패: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, status)
}

// ApplyDatapackExperimentsRequest represents payload for toggling experiments.
type ApplyDatapackExperimentsRequest struct {
	Experiments []string `json:"experiments"`
}

// ApplyDatapackExperiments updates server.properties, level.dat and live server with selected experiments.
func (h *Handler) ApplyDatapackExperiments(c *gin.Context) {
	var req ApplyDatapackExperimentsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "잘못된 요청 데이터입니다."})
		return
	}

	if err := h.datapackMgr.ApplyExperiments(req.Experiments, h.mcService); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "실험적 기능 적용 실패: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "선택한 실험적 게임플레이가 서버 설정 및 월드에 성공적으로 적용되었습니다.",
		"experiments": req.Experiments,
	})
}
