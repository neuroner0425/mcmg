package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed static/*
var staticFS embed.FS

// RegisterRoutes registers the frontend static assets and fallbacks.
// Prioritizes live disk files in internal/web/static if present for zero-latency development,
// with graceful fallback to embedded FS for standalone binaries.
func RegisterRoutes(router *gin.Engine) error {
	var targetFS fs.FS
	if _, err := os.Stat("internal/web/static"); err == nil {
		targetFS = os.DirFS("internal/web/static")
	} else {
		sub, err := fs.Sub(staticFS, "static")
		if err != nil {
			return err
		}
		targetFS = sub
	}

	// Middleware to prevent stale client caching
	noCacheMid := func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}

	staticGroup := router.Group("/static", noCacheMid)
	staticGroup.StaticFS("", http.FS(targetFS))

	// Serve index.html for root and SPA paths
	serveIndex := func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		indexData, err := fs.ReadFile(targetFS, "index.html")
		if err != nil {
			c.String(http.StatusInternalServerError, "Frontend build error: index.html not found")
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexData)
	}

	pageRoutes := []string{
		"/",
		"/login",
		"/dashboard",
		"/map",
		"/whitelist",
		"/admin/whitelist",
		"/admin/metrics",
		"/admin/console",
		"/admin/settings",
		"/admin/config",
		"/admin/installer",
		"/admin/plugins",
		"/admin/backups",
	}

	for _, p := range pageRoutes {
		router.GET(p, serveIndex)
		router.HEAD(p, serveIndex)
	}

	router.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasPrefix(path, "/api") && !strings.HasPrefix(path, "/bluemap") && !strings.HasPrefix(path, "/squaremap") && !strings.HasPrefix(path, "/ws") {
			serveIndex(c)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
	})

	return nil
}
