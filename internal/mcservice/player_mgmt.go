package mcservice

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type WhitelistEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type WhitelistStatus struct {
	Enabled bool             `json:"enabled"`
	Enforce bool             `json:"enforce"`
	Entries []WhitelistEntry `json:"entries"`
}

type PlayerActionReq struct {
	Player string `json:"player" binding:"required"`
	Action string `json:"action" binding:"required"` // msg, kick, ban, op, deop, gamemode, give, clear, spawn
	Param1 string `json:"param1"`                   // msg content, reason, gamemode, item ID
	Param2 string `json:"param2"`                   // item count, etc.
}

type PlayerMgmtService struct {
	mu        sync.Mutex
	serverDir string
	rcon      RCONClient
	procMgr   *ProcessManager
	propMgr   *PropertiesManager
}

func NewPlayerMgmtService(serverDir string, rcon RCONClient, propMgr *PropertiesManager) *PlayerMgmtService {
	return &PlayerMgmtService{
		serverDir: serverDir,
		rcon:      rcon,
		propMgr:   propMgr,
	}
}

// SetProcessManager sets the ProcessManager reference for stdin fallback.
func (pms *PlayerMgmtService) SetProcessManager(pm *ProcessManager) {
	pms.mu.Lock()
	defer pms.mu.Unlock()
	pms.procMgr = pm
}

// dispatchCommand sends a Minecraft command via RCON with automatic fallback to server stdin.
func (pms *PlayerMgmtService) dispatchCommand(cmd string) (string, error) {
	clean := strings.TrimSpace(cmd)
	if clean == "" {
		return "", nil
	}

	if pms.rcon != nil {
		res, err := pms.rcon.Execute(clean)
		if err == nil {
			return res, nil
		}
	}

	if pms.procMgr != nil {
		if err := pms.procMgr.WriteStdin(clean); err == nil {
			return fmt.Sprintf("[Stdin 전달 완료] %s", clean), nil
		}
	}

	return "", fmt.Errorf("명령어 실행 실패: RCON 및 서버 콘솔 모두 연결 불가")
}

// GetWhitelistStatus returns whether whitelist is enabled and the list of players.
func (pms *PlayerMgmtService) GetWhitelistStatus() (WhitelistStatus, error) {
	pms.mu.Lock()
	defer pms.mu.Unlock()

	status := WhitelistStatus{
		Enabled: false,
		Enforce: false,
		Entries: []WhitelistEntry{},
	}

	if pms.propMgr != nil {
		if val, ok := pms.propMgr.Get("white-list"); ok && strings.ToLower(val) == "true" {
			status.Enabled = true
		}
		if val, ok := pms.propMgr.Get("enforce-whitelist"); ok && strings.ToLower(val) == "true" {
			status.Enforce = true
		}
	}

	wlPath := filepath.Join(pms.serverDir, "whitelist.json")
	data, err := os.ReadFile(wlPath)
	if err == nil && len(data) > 0 {
		var entries []WhitelistEntry
		if err := json.Unmarshal(data, &entries); err == nil {
			// Auto-migrate offline UUIDs to online Mojang UUIDs if online-mode is true
			isOnlineMode := true
			if pms.propMgr != nil {
				if val, ok := pms.propMgr.Get("online-mode"); ok && strings.ToLower(val) == "false" {
					isOnlineMode = false
				}
			}

			modified := false
			if isOnlineMode {
				for i, e := range entries {
					offlineHash := generateOfflineUUID(e.Name)
					if strings.EqualFold(e.UUID, offlineHash) {
						if realUUID, err := fetchMojangUUID(e.Name); err == nil && realUUID != "" {
							entries[i].UUID = realUUID
							modified = true
						}
					}
				}
				if modified {
					if out, err := json.MarshalIndent(entries, "", "  "); err == nil {
						_ = os.WriteFile(wlPath, out, 0644)
						if pms.rcon != nil {
							_, _ = pms.rcon.Execute("whitelist reload")
						}
					}
				}
			}
			status.Entries = entries
		}
	}

	return status, nil
}

// SetWhitelistConfig enables or disables whitelist enforcement.
func (pms *PlayerMgmtService) SetWhitelistConfig(enabled, enforce bool) error {
	pms.mu.Lock()
	defer pms.mu.Unlock()

	enabledStr := "false"
	if enabled {
		enabledStr = "true"
	}
	enforceStr := "false"
	if enforce {
		enforceStr = "true"
	}

	if pms.propMgr != nil {
		_ = pms.propMgr.Set("white-list", enabledStr)
		_ = pms.propMgr.Set("enforce-whitelist", enforceStr)
	}

	if enabled {
		_, _ = pms.dispatchCommand("whitelist on")
	} else {
		_, _ = pms.dispatchCommand("whitelist off")
	}
	_, _ = pms.dispatchCommand("whitelist reload")

	return nil
}

// AddWhitelist registers a player to the whitelist.
func (pms *PlayerMgmtService) AddWhitelist(name string) error {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return fmt.Errorf("player nickname cannot be empty")
	}

	pms.mu.Lock()
	defer pms.mu.Unlock()

	wlPath := filepath.Join(pms.serverDir, "whitelist.json")
	var entries []WhitelistEntry
	data, err := os.ReadFile(wlPath)
	if err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &entries)
	}

	for _, e := range entries {
		if strings.EqualFold(e.Name, cleanName) {
			return fmt.Errorf("player '%s' is already in whitelist", cleanName)
		}
	}

	// Determine appropriate UUID (Mojang online UUID for online-mode, offline hash otherwise)
	targetUUID := pms.resolvePlayerUUID(cleanName)
	entries = append(entries, WhitelistEntry{
		Name: cleanName,
		UUID: targetUUID,
	})

	out, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(wlPath, out, 0644); err != nil {
		return fmt.Errorf("failed to write whitelist.json: %w", err)
	}

	_, _ = pms.dispatchCommand(fmt.Sprintf("whitelist add %s", cleanName))
	_, _ = pms.dispatchCommand("whitelist reload")

	return nil
}

// RemoveWhitelist removes a player from the whitelist.
func (pms *PlayerMgmtService) RemoveWhitelist(name string) error {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return fmt.Errorf("player nickname cannot be empty")
	}

	pms.mu.Lock()
	defer pms.mu.Unlock()

	wlPath := filepath.Join(pms.serverDir, "whitelist.json")
	var entries []WhitelistEntry
	data, err := os.ReadFile(wlPath)
	if err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &entries)
	}

	filtered := make([]WhitelistEntry, 0, len(entries))
	found := false
	for _, e := range entries {
		if strings.EqualFold(e.Name, cleanName) {
			found = true
			continue
		}
		filtered = append(filtered, e)
	}

	if !found {
		return fmt.Errorf("player '%s' not found in whitelist", cleanName)
	}

	out, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(wlPath, out, 0644); err != nil {
		return fmt.Errorf("failed to write whitelist.json: %w", err)
	}

	_, _ = pms.dispatchCommand(fmt.Sprintf("whitelist remove %s", cleanName))
	_, _ = pms.dispatchCommand("whitelist reload")

	return nil
}

// ExecutePlayerAction performs interactive admin operations on a target player.
func (pms *PlayerMgmtService) ExecutePlayerAction(req PlayerActionReq) (string, error) {
	target := strings.TrimSpace(req.Player)
	if target == "" {
		return "", fmt.Errorf("target player is required")
	}

	var cmd string
	switch req.Action {
	case "msg":
		msg := strings.TrimSpace(req.Param1)
		if msg == "" {
			return "", fmt.Errorf("message content cannot be empty")
		}
		cmd = fmt.Sprintf(`tellraw %s {"text":"[관리자 메시지] %s","color":"gold","bold":true}`, target, msg)
	case "kick":
		reason := strings.TrimSpace(req.Param1)
		if reason == "" {
			reason = "관리자에 의해 추방되었습니다."
		}
		cmd = fmt.Sprintf("kick %s %s", target, reason)
	case "ban":
		reason := strings.TrimSpace(req.Param1)
		if reason == "" {
			reason = "관리자에 의해 차단되었습니다."
		}
		cmd = fmt.Sprintf("ban %s %s", target, reason)
	case "op":
		cmd = fmt.Sprintf("op %s", target)
	case "deop":
		cmd = fmt.Sprintf("deop %s", target)
	case "gamemode":
		mode := strings.TrimSpace(req.Param1)
		if mode == "" {
			mode = "survival"
		}
		cmd = fmt.Sprintf("gamemode %s %s", mode, target)
	case "give":
		item := strings.TrimSpace(req.Param1)
		if item == "" {
			return "", fmt.Errorf("item id is required")
		}
		count := strings.TrimSpace(req.Param2)
		if count == "" {
			count = "1"
		}
		cmd = fmt.Sprintf("give %s %s %s", target, item, count)
	case "clear":
		cmd = fmt.Sprintf("clear %s", target)
	case "spawn":
		cmd = fmt.Sprintf("tp %s 0 ~ 0", target)
	default:
		return "", fmt.Errorf("unknown action '%s'", req.Action)
	}

	return pms.dispatchCommand(cmd)
}

func generateOfflineUUID(username string) string {
	h := md5.Sum([]byte("OfflinePlayer:" + username))
	// Set version 3
	h[6] = (h[6] & 0x0f) | 0x30
	// Set variant
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func (pms *PlayerMgmtService) resolvePlayerUUID(username string) string {
	isOnlineMode := true
	if pms.propMgr != nil {
		if val, ok := pms.propMgr.Get("online-mode"); ok && strings.ToLower(val) == "false" {
			isOnlineMode = false
		}
	}

	if isOnlineMode {
		if onlineUUID, err := fetchMojangUUID(username); err == nil && onlineUUID != "" {
			return onlineUUID
		}
	}
	return generateOfflineUUID(username)
}

func fetchMojangUUID(username string) (string, error) {
	url := fmt.Sprintf("https://api.mojang.com/users/profiles/minecraft/%s", username)
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mojang API status %d", resp.StatusCode)
	}

	var data struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	id := strings.TrimSpace(data.ID)
	if len(id) == 32 {
		return fmt.Sprintf("%s-%s-%s-%s-%s",
			id[0:8], id[8:12], id[12:16], id[16:20], id[20:32]), nil
	}
	return id, nil
}
