package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListPlugins returns installed plugins and their active/disabled status.
func (h *Handler) ListPlugins(c *gin.Context) {
	plugins, err := h.pluginMgr.ListPlugins()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"plugins": plugins})
}

// UploadPlugin handles multipart file upload of .jar plugin files.
func (h *Handler) UploadPlugin(c *gin.Context) {
	file, err := c.FormFile("plugin")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "파일이 업로드되지 않았습니다."})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer src.Close()

	if err := h.pluginMgr.SaveUploadedPlugin(file.Filename, src); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("플러그인 %s 업로드 완료", file.Filename)})
}

// DownloadPluginRequest defines remote URL download payload.
type DownloadPluginRequest struct {
	URL      string `json:"url" binding:"required"`
	FileName string `json:"filename"`
}

// DownloadPlugin downloads a plugin jar from an external HTTP/HTTPS link.
func (h *Handler) DownloadPlugin(c *gin.Context) {
	var req DownloadPluginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	if err := h.pluginMgr.DownloadPluginFromURL(req.URL, req.FileName); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "플러그인 다운로드 완료"})
}

// DeletePlugin removes a plugin file from the server's plugins directory.
func (h *Handler) DeletePlugin(c *gin.Context) {
	name := c.Param("name")
	if err := h.pluginMgr.DeletePlugin(name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("플러그인 %s 삭제 완료", name)})
}

// TogglePlugin renames a plugin to/from .jar.disabled.
func (h *Handler) TogglePlugin(c *gin.Context) {
	name := c.Param("name")
	newName, err := h.pluginMgr.TogglePlugin(name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message":  fmt.Sprintf("플러그인 상태가 변경되었습니다 (%s)", newName),
		"new_name": newName,
	})
}

// AutoInstallSquaremap downloads compatible squaremap plugin jar matching server version and configures internal-webserver port (8100).
func (h *Handler) AutoInstallSquaremap(c *gin.Context) {
	var req struct {
		Version string `json:"version"`
	}
	_ = c.ShouldBindJSON(&req)

	jarName, err := h.pluginMgr.AutoInstallSquaremap(req.Version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("squaremap 자동 다운로드 실패: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "서버 버전에 적합한 squaremap 플러그인이 자동으로 다운로드되고 포트(8100) 설정이 적용되었습니다.",
		"file":    jarName,
	})
}

// AutoInstallBlueMap delegates to AutoInstallSquaremap for backwards compatibility.
func (h *Handler) AutoInstallBlueMap(c *gin.Context) {
	h.AutoInstallSquaremap(c)
}
