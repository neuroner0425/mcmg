package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// CheckSystemUpdate handles checking for remote git updates.
func (h *Handler) CheckSystemUpdate(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	info, err := h.updater.CheckUpdate(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "업데이트 확인 중 오류 발생: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, info)
}

// ApplySystemUpdate triggers git pull, compilation, binary swap, and zero-downtime reload.
func (h *Handler) ApplySystemUpdate(c *gin.Context) {
	// Background context as request will finish before restart
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	err := h.updater.ApplyUpdate(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "업데이트 적용 실패: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "업데이트 및 컴파일이 성공했습니다. 서버 매니저가 무중단 재기동되며 약 3초 후 연결이 복원됩니다.",
	})
}
