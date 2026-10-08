package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"minecraft_server_manager/internal/config"
	"minecraft_server_manager/internal/mcservice"
)

type mockRCONClient struct {
	executeFunc   func(cmd string) (string, error)
	getPlayerFunc func() ([]string, int, int, error)
	sendChatFunc  func(sender, message string) error
}

func (m *mockRCONClient) Execute(cmd string) (string, error) {
	if m.executeFunc != nil {
		return m.executeFunc(cmd)
	}
	return "ok", nil
}

func (m *mockRCONClient) GetPlayers() ([]string, int, int, error) {
	if m.getPlayerFunc != nil {
		return m.getPlayerFunc()
	}
	return []string{"Player1"}, 1, 20, nil
}

func (m *mockRCONClient) SendChatMessage(sender, message string) error {
	if m.sendChatFunc != nil {
		return m.sendChatFunc(sender, message)
	}
	return nil
}

func setupTestHandler(t *testing.T, mockClient *mockRCONClient) (*Handler, *config.Config) {
	tmpDir := t.TempDir()
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8080, Host: "0.0.0.0"},
		Security: config.SecurityConfig{
			UserPassword:  "user123",
			AdminPassword: "admin123",
			JWTSecret:     "test-secret",
		},
		MC: config.MCConfig{
			PropertiesPath: tmpDir + "/server.properties",
			ServerDir:      tmpDir,
			JarName:        "purpur.jar",
			JavaPath:       "java",
			MinMemory:      "1G",
			MaxMemory:      "2G",
		},
		BlueMap: config.BlueMapConfig{URL: "http://127.0.0.1:8100"},
	}

	propMgr := mcservice.NewPropertiesManager(cfg.MC.PropertiesPath, tmpDir+"/backups")
	purpurMgr := mcservice.NewPurpurConfigManager(cfg.MC.ServerDir, tmpDir+"/backups")
	chatSvc := mcservice.NewChatService(100)
	installer := mcservice.NewInstaller()
	procMgr := mcservice.NewProcessManager(cfg.MC.ServerDir, cfg.MC.JarName, cfg.MC.JavaPath, cfg.MC.MinMemory, cfg.MC.MaxMemory, mockClient, chatSvc)
	pluginMgr := mcservice.NewPluginManager(cfg.MC.ServerDir)
	metricsSvc := mcservice.NewMetricsService(cfg.MC.ServerDir, procMgr, mockClient, propMgr)
	playerMgmt := mcservice.NewPlayerMgmtService(cfg.MC.ServerDir, mockClient, propMgr)
	backupMgr := mcservice.NewBackupManager(cfg.MC.ServerDir, tmpDir+"/backups", mockClient, procMgr)
	wsHub := mcservice.NewWSHub(procMgr, chatSvc, metricsSvc, mockClient)

	h, err := NewHandler(cfg, mockClient, propMgr, purpurMgr, procMgr, installer, pluginMgr, chatSvc, metricsSvc, playerMgmt, backupMgr, wsHub)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	return h, cfg
}

func TestHandler_Login(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, _ := setupTestHandler(t, &mockRCONClient{})

	router := gin.New()
	router.POST("/login", h.Login)

	tests := []struct {
		name         string
		payload      map[string]string
		expectedCode int
		expectedRole string
	}{
		{
			name:         "Valid user login with nickname",
			payload:      map[string]string{"nickname": "Alex", "password": "user123"},
			expectedCode: http.StatusOK,
			expectedRole: "user",
		},
		{
			name:         "Valid admin login with nickname",
			payload:      map[string]string{"nickname": "Steve", "password": "admin123"},
			expectedCode: http.StatusOK,
			expectedRole: "admin",
		},
		{
			name:         "Missing nickname",
			payload:      map[string]string{"password": "admin123"},
			expectedCode: http.StatusBadRequest,
			expectedRole: "",
		},
		{
			name:         "Invalid password",
			payload:      map[string]string{"nickname": "Steve", "password": "wrong"},
			expectedCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.payload)
			req, _ := http.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.expectedCode {
				t.Fatalf("expected code %d, got %d", tt.expectedCode, w.Code)
			}

			if tt.expectedRole != "" {
				var res map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
					t.Fatalf("invalid json response: %v", err)
				}
				if res["role"] != tt.expectedRole {
					t.Errorf("expected role %s, got %v", tt.expectedRole, res["role"])
				}
			}
		})
	}
}

func TestHandler_SendChatAndLiveSync(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var sentSender, sentMsg string
	mockClient := &mockRCONClient{
		sendChatFunc: func(sender, message string) error {
			sentSender = sender
			sentMsg = message
			return nil
		},
	}
	h, _ := setupTestHandler(t, mockClient)

	router := gin.New()
	router.POST("/chat", h.SendChat)
	router.GET("/chat", h.GetChatMessages)

	// 1. Send chat
	payload := map[string]string{"sender": "WebUser", "message": "Hello Ingame!"}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest(http.MethodPost, "/chat", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected code 200, got %d", w.Code)
	}
	if sentSender != "WebUser" || sentMsg != "Hello Ingame!" {
		t.Errorf("sent mismatch: %s / %s", sentSender, sentMsg)
	}

	// 2. Query chat
	req2, _ := http.NewRequest(http.MethodGet, "/chat?since=0", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected code 200, got %d", w2.Code)
	}

	var res map[string][]mcservice.ChatMessage
	_ = json.Unmarshal(w2.Body.Bytes(), &res)
	if len(res["messages"]) != 1 || res["messages"][0].Text != "Hello Ingame!" {
		t.Errorf("expected 1 chat message synced, got %+v", res["messages"])
	}
}

func TestCheckSystemUpdate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockClient := &mockRCONClient{}
	h, _ := setupTestHandler(t, mockClient)

	router := gin.New()
	router.GET("/admin/system/update/check", h.CheckSystemUpdate)

	req, _ := http.NewRequest(http.MethodGet, "/admin/system/update/check", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var info mcservice.UpdateInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}

	if info.CurrentCommit == "" {
		t.Errorf("expected current_commit to be populated")
	}
}
