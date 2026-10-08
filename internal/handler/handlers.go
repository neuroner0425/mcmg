package handler

import (
	"net/http"
	"net/http/httputil"
	"net/url"

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

// BlueMapProxy forwards requests to internal BlueMap server with query and path preservation.
func (h *Handler) BlueMapProxy(c *gin.Context) {
	proxy := httputil.NewSingleHostReverseProxy(h.blueMapURL)
	targetPath := c.Param("proxyPath")
	if targetPath == "" || targetPath == "/" {
		targetPath = "/index.html"
	}
	proxy.Director = func(req *http.Request) {
		req.Header = c.Request.Header.Clone()
		req.Host = h.blueMapURL.Host
		req.URL.Scheme = h.blueMapURL.Scheme
		req.URL.Host = h.blueMapURL.Host
		req.URL.Path = targetPath
		req.URL.RawQuery = c.Request.URL.RawQuery
		req.Header.Set("X-Forwarded-Host", c.Request.Host)
		req.Header.Set("X-Forwarded-Proto", "http")
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
