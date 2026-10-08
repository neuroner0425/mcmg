package handler

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

// ListBackups returns all created tar.gz world backup archives.
func (h *Handler) ListBackups(c *gin.Context) {
	list, err := h.backupMgr.ListBackups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"backups": list})
}

// CreateBackupReq payload for manual backup creation.
type CreateBackupReq struct {
	Description string `json:"description"`
}

// CreateBackup flushes RCON world save and packages world directories into tar.gz.
func (h *Handler) CreateBackup(c *gin.Context) {
	var req CreateBackupReq
	_ = c.ShouldBindJSON(&req)

	info, err := h.backupMgr.CreateBackup(req.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "월드 백업이 완료되었습니다.",
		"backup":  info,
	})
}

// DeleteBackup deletes a specific backup archive.
func (h *Handler) DeleteBackup(c *gin.Context) {
	filename := c.Param("filename")
	if err := h.backupMgr.DeleteBackup(filename); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("백업 %s 삭제 완료", filename)})
}

// DownloadBackup streams the tar.gz backup file for browser download.
func (h *Handler) DownloadBackup(c *gin.Context) {
	filename := filepath.Base(c.Param("filename"))
	filePath := filepath.Join(h.cfg.Data.BackupDir, filename)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.File(filePath)
}
