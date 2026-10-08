package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetMetrics returns real-time resource telemetry and historical charts.
func (h *Handler) GetMetrics(c *gin.Context) {
	current := h.metricsSvc.CollectCurrentMetrics()
	history := h.metricsSvc.GetHistory()
	c.JSON(http.StatusOK, gin.H{
		"current": current,
		"history": history,
	})
}

// GetWorldEnvironment returns world time, day/night phase, and current weather.
func (h *Handler) GetWorldEnvironment(c *gin.Context) {
	ticks, timeStr, phase, weather := h.metricsSvc.GetWorldEnvironment()
	c.JSON(http.StatusOK, gin.H{
		"ticks":   ticks,
		"time":    timeStr,
		"phase":   phase,
		"weather": weather,
	})
}

// SetTimeReq payload for updating in-game world time.
type SetTimeReq struct {
	Ticks int `json:"ticks"`
}

// SetWorldTime changes the in-game world time ticks and broadcasts updated state.
func (h *Handler) SetWorldTime(c *gin.Context) {
	var req SetTimeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ticks is required"})
		return
	}
	resp, err := h.metricsSvc.SetWorldTime(req.Ticks)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.wsHub != nil {
		m := h.metricsSvc.CollectCurrentMetrics()
		h.wsHub.BroadcastMetrics(m)
	}
	c.JSON(http.StatusOK, gin.H{
		"message":  "시간이 성공적으로 변경되었습니다.",
		"response": resp,
	})
}

// SetWeatherReq payload for changing in-game world weather.
type SetWeatherReq struct {
	Weather string `json:"weather" binding:"required"`
}

// SetWorldWeather changes the in-game world weather and broadcasts updated state.
func (h *Handler) SetWorldWeather(c *gin.Context) {
	var req SetWeatherReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "weather is required"})
		return
	}
	resp, err := h.metricsSvc.SetWorldWeather(req.Weather)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.wsHub != nil {
		m := h.metricsSvc.CollectCurrentMetrics()
		h.wsHub.BroadcastMetrics(m)
	}
	c.JSON(http.StatusOK, gin.H{
		"message":  "날씨가 성공적으로 변경되었습니다.",
		"response": resp,
	})
}
