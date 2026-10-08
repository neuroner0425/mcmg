package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// GetPurpurVersions fetches available Minecraft versions from the Purpur API.
func (h *Handler) GetPurpurVersions(c *gin.Context) {
	versions, err := h.installer.FetchVersions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

// GetPurpurBuilds fetches builds for a given Minecraft version from the Purpur API.
func (h *Handler) GetPurpurBuilds(c *gin.Context) {
	version := c.Query("version")
	if version == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version query parameter is required"})
		return
	}

	builds, latest, err := h.installer.FetchBuilds(version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"builds": builds, "latest": latest})
}

// DownloadPurpurRequest payload for downloading a Purpur jar.
type DownloadPurpurRequest struct {
	Version string `json:"version" binding:"required"`
	Build   string `json:"build"`
}

// DownloadPurpur triggers asynchronous download of the selected Purpur binary and auto-sets BlueMap.
func (h *Handler) DownloadPurpur(c *gin.Context) {
	var req DownloadPurpurRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version is required"})
		return
	}

	go func() {
		err := h.installer.DownloadPurpur(req.Version, req.Build, h.cfg.MC.ServerDir, h.cfg.MC.JarName)
		if err == nil {
			// Automatically install and configure squaremap plugin matching downloaded version
			_, _ = h.pluginMgr.AutoInstallSquaremap(req.Version)
		}
	}()

	c.JSON(http.StatusOK, gin.H{"message": "Purpur 다운로드를 시작했습니다."})
}

// GetDownloadStatus returns current binary download progress and status text.
func (h *Handler) GetDownloadStatus(c *gin.Context) {
	isBusy, progress, statusText := h.installer.GetStatus()
	c.JSON(http.StatusOK, gin.H{
		"is_busy":     isBusy,
		"in_progress": isBusy,
		"progress":    progress,
		"status_text": statusText,
		"message":     statusText,
	})
}

// GetInstalledPurpur inspects server directory to return currently installed Purpur binary metadata.
func (h *Handler) GetInstalledPurpur(c *gin.Context) {
	info := h.installer.GetInstalledVersion(h.cfg.MC.ServerDir, h.cfg.MC.JarName)
	c.JSON(http.StatusOK, gin.H{
		"installed": info,
	})
}
