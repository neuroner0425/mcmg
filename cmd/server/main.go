package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"minecraft_server_manager/internal/config"
	"minecraft_server_manager/internal/handler"
	"minecraft_server_manager/internal/mcservice"
	"minecraft_server_manager/internal/middleware"
	"minecraft_server_manager/internal/web"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to config file")
	flag.Parse()

	// 1. Ensure logs directory and setup HTTP access log separation
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Fatalf("[FATAL] Failed to create logs directory: %v", err)
	}

	accessLogPath := filepath.Join("logs", "access.log")
	accessLogFile, err := os.OpenFile(accessLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("[FATAL] Failed to open access.log: %v", err)
	}
	defer accessLogFile.Close()

	gin.DefaultWriter = accessLogFile

	log.SetOutput(os.Stdout)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmsgprefix)
	log.SetPrefix("[OPERATIONAL] ")

	// 2. Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 3. Initialize Services
	mcSvc := mcservice.NewService(cfg.MC.RconAddress, cfg.MC.RconPassword)
	propMgr := mcservice.NewPropertiesManager(cfg.MC.PropertiesPath, cfg.Data.BackupDir)
	purpurMgr := mcservice.NewPurpurConfigManager(cfg.MC.ServerDir, cfg.Data.BackupDir)
	chatSvc := mcservice.NewChatService(200)
	installer := mcservice.NewInstaller()
	procMgr := mcservice.NewProcessManager(
		cfg.MC.ServerDir,
		cfg.MC.JarName,
		cfg.MC.JavaPath,
		cfg.MC.MinMemory,
		cfg.MC.MaxMemory,
		mcSvc,
		chatSvc,
	)
	rconPort := mcservice.ParseRconPort(cfg.MC.RconAddress)
	procMgr.SetRconConfig(cfg.MC.RconPassword, rconPort)
	mcservice.EnsureServerProperties(cfg.MC.ServerDir, cfg.MC.RconPassword, rconPort)
	pluginMgr := mcservice.NewPluginManager(cfg.MC.ServerDir)
	metricsSvc := mcservice.NewMetricsService(cfg.MC.ServerDir, procMgr, mcSvc, propMgr)
	playerMgmt := mcservice.NewPlayerMgmtService(cfg.MC.ServerDir, mcSvc, propMgr)
	playerMgmt.SetProcessManager(procMgr)
	backupMgr := mcservice.NewBackupManager(cfg.MC.ServerDir, cfg.Data.BackupDir, mcSvc, procMgr)
	wsHub := mcservice.NewWSHub(procMgr, chatSvc, metricsSvc, mcSvc)

	h, err := handler.NewHandler(cfg, mcSvc, propMgr, purpurMgr, procMgr, installer, pluginMgr, chatSvc, metricsSvc, playerMgmt, backupMgr, wsHub)
	if err != nil {
		log.Fatalf("Failed to initialize handler: %v", err)
	}

	// 4. Initialize Gin Engine
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.LoggerWithWriter(accessLogFile))

	// 5. Register WebSocket Live Stream Route
	router.GET("/ws", h.HandleWebSocket)

	// 6. Register Embedded Static Web Routes
	if err := web.RegisterRoutes(router); err != nil {
		log.Fatalf("Failed to register web assets: %v", err)
	}

	// 7. Register API Routes
	api := router.Group("/api")
	{
		// Public Auth Routes
		api.POST("/login", h.Login)
		api.POST("/logout", h.Logout)

		// User Authenticated Routes
		userGroup := api.Group("")
		userGroup.Use(middleware.RequireRole(cfg.Security.JWTSecret, middleware.RoleUser))
		{
			userGroup.GET("/me", h.GetMe)
			userGroup.GET("/players", h.GetPlayers)
			userGroup.GET("/chat", h.GetChatMessages)
			userGroup.POST("/chat", h.SendChat)
			userGroup.GET("/world/environment", h.GetWorldEnvironment)
			userGroup.POST("/whitelist/register", h.RegisterSelfWhitelist)
			userGroup.GET("/process/status", h.GetProcessStatus)
			userGroup.POST("/process/start", h.StartProcess)
		}

		// Admin Authenticated Routes
		adminGroup := api.Group("")
		adminGroup.Use(middleware.RequireRole(cfg.Security.JWTSecret, middleware.RoleAdmin))
		{
			// Console & Commands
			adminGroup.POST("/command", h.ExecuteCommand)

			// World Environment Controls (Time & Weather)
			adminGroup.POST("/admin/world/time", h.SetWorldTime)
			adminGroup.POST("/admin/world/weather", h.SetWorldWeather)

			// Detailed Schema-Driven Settings (Properties + Purpur)
			adminGroup.GET("/admin/config/schema", h.GetConfigSchema)
			adminGroup.POST("/admin/config/save", h.SaveConfig)

			// Datapacks & Experimental Gameplay
			adminGroup.GET("/admin/datapacks", h.GetDatapacks)
			adminGroup.POST("/admin/datapacks/experiments", h.ApplyDatapackExperiments)

			// Process Lifecycle Control
			adminGroup.GET("/admin/process/status", h.GetProcessStatus)
			adminGroup.POST("/admin/process/start", h.StartProcess)
			adminGroup.POST("/admin/process/stop", h.StopProcess)
			adminGroup.POST("/admin/process/restart", h.RestartProcess)
			adminGroup.GET("/admin/process/logs", h.GetProcessLogs)

			// System Metrics (Admin Only)
			adminGroup.GET("/admin/metrics", h.GetMetrics)

			// Player Management & Whitelist (Admin Only)
			adminGroup.GET("/admin/whitelist", h.GetWhitelist)
			adminGroup.POST("/admin/whitelist/config", h.UpdateWhitelistConfig)
			adminGroup.POST("/admin/whitelist/add", h.AddWhitelistPlayer)
			adminGroup.POST("/admin/whitelist/remove", h.RemoveWhitelistPlayer)
			adminGroup.POST("/admin/players/action", h.ExecutePlayerAction)

			// Purpur Auto-Installer
			adminGroup.GET("/admin/purpur/versions", h.GetPurpurVersions)
			adminGroup.GET("/admin/purpur/builds", h.GetPurpurBuilds)
			adminGroup.GET("/admin/purpur/installed", h.GetInstalledPurpur)
			adminGroup.POST("/admin/purpur/download", h.DownloadPurpur)
			adminGroup.GET("/admin/purpur/download/status", h.GetDownloadStatus)

			// Plugin Management & Toggle
			adminGroup.GET("/admin/plugins", h.ListPlugins)
			adminGroup.POST("/admin/plugins/upload", h.UploadPlugin)
			adminGroup.POST("/admin/plugins/download", h.DownloadPlugin)
			adminGroup.POST("/admin/plugins/toggle/:name", h.TogglePlugin)
			adminGroup.DELETE("/admin/plugins/:name", h.DeletePlugin)
			adminGroup.POST("/admin/bluemap/auto-install", h.AutoInstallBlueMap)
			adminGroup.POST("/admin/squaremap/auto-install", h.AutoInstallSquaremap)

			// World Backups (Admin Only)
			adminGroup.GET("/admin/backups", h.ListBackups)
			adminGroup.POST("/admin/backups/create", h.CreateBackup)
			adminGroup.DELETE("/admin/backups/:filename", h.DeleteBackup)
			adminGroup.GET("/admin/backups/download/:filename", h.DownloadBackup)

			// Zero-Downtime System Update (Admin Only)
			adminGroup.GET("/admin/system/update/check", h.CheckSystemUpdate)
			adminGroup.POST("/admin/system/update/apply", h.ApplySystemUpdate)
		}
	}

	// World Map Reverse Proxy (squaremap & BlueMap backward compatibility)
	router.Any("/squaremap/*proxyPath", middleware.RequireRole(cfg.Security.JWTSecret, middleware.RoleUser), h.BlueMapProxy)
	router.Any("/bluemap/*proxyPath", middleware.RequireRole(cfg.Security.JWTSecret, middleware.RoleUser), h.BlueMapProxy)

	// 8. Start HTTP / HTTPS Server with Graceful Shutdown
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if cfg.Server.TLS.Enabled {
			if err := config.EnsureCertificate(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile, cfg.Server.TLS.AutoCert); err != nil {
				log.Fatalf("TLS certificate setup failed: %v", err)
			}
			log.Printf("[OPERATIONAL] Server listening on https://%s (TLS enabled)", addr)
			if err := srv.ListenAndServeTLS(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("HTTPS Listen error: %v", err)
			}
		} else {
			log.Printf("[OPERATIONAL] Server listening on http://%s (HTTP access logs -> %s)", addr, accessLogPath)
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("HTTP Listen error: %v", err)
			}
		}
	}()

	// Wait for interrupt signals
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Received termination signal, shutting down gracefully...")

	// Zero-Downtime architecture: Do NOT stop Minecraft server on web manager shutdown.
	// The Minecraft server will remain running in background and be adopted on restart.
	log.Println("[OPERATIONAL] Web Manager HTTP service stopped (Minecraft server process remains active).")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped cleanly.")
}
