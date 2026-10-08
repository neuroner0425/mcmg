package handler

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"minecraft_server_manager/internal/config"
	"minecraft_server_manager/internal/mcservice"
	"minecraft_server_manager/internal/middleware"
)

// Handler coordinates all HTTP routing and request handling.
type Handler struct {
	cfg        *config.Config
	mcService  mcservice.RCONClient
	propMgr    *mcservice.PropertiesManager
	purpurMgr  *mcservice.PurpurConfigManager
	procMgr    *mcservice.ProcessManager
	installer  *mcservice.Installer
	pluginMgr  *mcservice.PluginManager
	chatSvc    *mcservice.ChatService
	metricsSvc *mcservice.MetricsService
	playerMgmt *mcservice.PlayerMgmtService
	backupMgr   *mcservice.BackupManager
	datapackMgr *mcservice.DatapackManager
	updater     *mcservice.Updater
	wsHub       *mcservice.WSHub
	blueMapURL  *url.URL
}

// NewHandler constructs a new Handler instance.
func NewHandler(
	cfg *config.Config,
	mc mcservice.RCONClient,
	propMgr *mcservice.PropertiesManager,
	purpurMgr *mcservice.PurpurConfigManager,
	procMgr *mcservice.ProcessManager,
	installer *mcservice.Installer,
	pluginMgr *mcservice.PluginManager,
	chatSvc *mcservice.ChatService,
	metricsSvc *mcservice.MetricsService,
	playerMgmt *mcservice.PlayerMgmtService,
	backupMgr *mcservice.BackupManager,
	wsHub *mcservice.WSHub,
) (*Handler, error) {
	bmURL, err := url.Parse(cfg.BlueMap.URL)
	if err != nil {
		return nil, err
	}

	return &Handler{
		cfg:         cfg,
		mcService:   mc,
		propMgr:     propMgr,
		purpurMgr:   purpurMgr,
		procMgr:     procMgr,
		installer:   installer,
		pluginMgr:   pluginMgr,
		chatSvc:     chatSvc,
		metricsSvc:  metricsSvc,
		playerMgmt:  playerMgmt,
		backupMgr:   backupMgr,
		datapackMgr: mcservice.NewDatapackManager(cfg.MC.ServerDir),
		updater:     mcservice.NewUpdater("."),
		wsHub:       wsHub,
		blueMapURL:  bmURL,
	}, nil
}

const mapOfflineHTML = `<!DOCTYPE html>
<html lang="ko">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>squaremap 지도 대기 중</title>
  <style>
    body {
      margin: 0;
      padding: 0;
      background: #0d0d12;
      color: #f4f4f5;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
      display: flex;
      align-items: center;
      justify-content: center;
      height: 100vh;
      text-align: center;
    }
    .box {
      background: #16161e;
      border: 1px solid rgba(255, 255, 255, 0.08);
      border-radius: 16px;
      padding: 44px 32px;
      max-width: 440px;
      box-shadow: 0 10px 40px rgba(0, 0, 0, 0.6);
    }
    .icon { font-size: 52px; margin-bottom: 16px; }
    h2 { margin: 0 0 12px 0; font-size: 1.25rem; font-weight: 700; color: #fff; }
    p { margin: 0 0 24px 0; font-size: 0.875rem; color: #9494a0; line-height: 1.6; }
    .spinner {
      display: inline-block;
      width: 26px;
      height: 26px;
      border: 3px solid rgba(255, 255, 255, 0.1);
      border-radius: 50%;
      border-top-color: #e05660;
      animation: spin 1s ease-in-out infinite;
    }
    @keyframes spin { to { transform: rotate(360deg); } }
  </style>
  <script>
    setTimeout(function() { location.reload(); }, 4000);
  </script>
</head>
<body>
  <div class="box">
    <div class="icon">🗺️</div>
    <h2>squaremap 지도 준비 중</h2>
    <p>마인크래프트 서버가 오프라인이거나 squaremap 플러그인을 불러오는 중입니다.<br>서버가 시작되면 자동으로 지도가 활성화됩니다.</p>
    <div class="spinner"></div>
  </div>
</body>
</html>`

// BlueMapProxy forwards requests to internal squaremap/BlueMap server with query and path preservation.
func (h *Handler) BlueMapProxy(c *gin.Context) {
	targetPath := c.Param("proxyPath")
	if targetPath == "" || targetPath == "/" {
		targetPath = "/index.html"
	}

	// 1. If Minecraft server is not running, return friendly waiting page instead of 502
	if h.procMgr != nil && h.procMgr.GetStatus().Status != mcservice.StatusRunning {
		if strings.HasSuffix(targetPath, ".js") || strings.HasSuffix(targetPath, ".css") || strings.HasSuffix(targetPath, ".png") || strings.HasSuffix(targetPath, ".json") {
			c.Status(http.StatusServiceUnavailable)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(mapOfflineHTML))
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(h.blueMapURL)

	// 2. Custom ErrorHandler to intercept connection refused and prevent Cloudflare 502
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		if strings.HasSuffix(req.URL.Path, ".js") || strings.HasSuffix(req.URL.Path, ".css") || strings.HasSuffix(req.URL.Path, ".png") || strings.HasSuffix(req.URL.Path, ".json") {
			http.Error(rw, "map backend unavailable", http.StatusServiceUnavailable)
			return
		}
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(mapOfflineHTML))
	}

	proxy.Director = func(req *http.Request) {
		req.Header = c.Request.Header.Clone()
		req.Host = h.blueMapURL.Host
		req.URL.Scheme = h.blueMapURL.Scheme
		req.URL.Host = h.blueMapURL.Host
		req.URL.Path = targetPath
		req.URL.RawQuery = c.Request.URL.RawQuery

		proto := c.Request.Header.Get("X-Forwarded-Proto")
		if proto == "" {
			if c.Request.TLS != nil {
				proto = "https"
			} else {
				proto = "http"
			}
		}
		req.Header.Set("X-Forwarded-Host", c.Request.Host)
		req.Header.Set("X-Forwarded-Proto", proto)
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

// HandleWebSocket upgrades connection and streams real-time console logs, chat, and metrics.
func (h *Handler) HandleWebSocket(c *gin.Context) {
	tokenStr := ""
	if cookie, err := c.Cookie(middleware.CookieKey); err == nil && cookie != "" {
		tokenStr = cookie
	}
	if tokenStr == "" {
		tokenStr = c.Query("token")
	}

	if tokenStr == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	claims, err := middleware.ParseToken(tokenStr, h.cfg.Security.JWTSecret)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
		return
	}

	if h.wsHub == nil {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "websocket service unavailable"})
		return
	}

	if err := h.wsHub.HandleWebSocket(c.Writer, c.Request, claims.Nickname, claims.Role); err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
