package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"minecraft_server_manager/internal/mcservice"
)

// GetConfigSchema returns predefined setting definitions populated with current values.
func (h *Handler) GetConfigSchema(c *gin.Context) {
	_, propMap, err := h.propMgr.Read()
	if err != nil && propMap == nil {
		propMap = make(map[string]string)
	}

	_, purpurMap, err := h.purpurMgr.Read()
	if err != nil && purpurMap == nil {
		purpurMap = make(map[string]string)
	}

	// Build full schema merging predefined registry and any dynamically discovered keys
	result := mcservice.BuildFullConfigSchema(propMap, purpurMap)

	c.JSON(http.StatusOK, gin.H{
		"schema":     result,
		"properties": propMap,
		"purpur":     purpurMap,
	})
}

// SaveConfigPayload defines updates for properties and purpur.
type SaveConfigPayload struct {
	Properties map[string]string `json:"properties"`
	Purpur     map[string]string `json:"purpur"`
}

// SaveConfig saves changes to server.properties and purpur.yml with automatic backups.
func (h *Handler) SaveConfig(c *gin.Context) {
	var payload SaveConfigPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	var errorsList []string

	if payload.Properties != nil && len(payload.Properties) > 0 {
		if err := h.propMgr.Save(payload.Properties); err != nil {
			errorsList = append(errorsList, "server.properties: "+err.Error())
		}
	}

	if payload.Purpur != nil && len(payload.Purpur) > 0 {
		if err := h.purpurMgr.Save(payload.Purpur); err != nil {
			errorsList = append(errorsList, "purpur.yml: "+err.Error())
		}
	}

	if len(errorsList) > 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": strings.Join(errorsList, "; ")})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "설정이 성공적으로 저장되었습니다 (자동 백업 완료)."})
}
