package mcservice

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow same-origin and local network access
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WSClient represents an active WebSocket connection.
type WSClient struct {
	hub      *WSHub
	conn     *websocket.Conn
	send     chan []byte
	nickname string
	role     string
}

// WSMessage represents a typed frame exchanged over WebSocket.
type WSMessage struct {
	Type    string      `json:"type"`              // "init", "log", "chat", "metrics", "command_result"
	Payload interface{} `json:"payload,omitempty"`
}

// WSHub coordinates live real-time streams across connected browsers.
type WSHub struct {
	mu         sync.RWMutex
	clients    map[*WSClient]bool
	procMgr    *ProcessManager
	chatSvc    *ChatService
	metricsSvc *MetricsService
	rcon       RCONClient
}

// NewWSHub initializes the WebSocket hub and binds event hooks.
func NewWSHub(procMgr *ProcessManager, chatSvc *ChatService, metricsSvc *MetricsService, rcon RCONClient) *WSHub {
	hub := &WSHub{
		clients:    make(map[*WSClient]bool),
		procMgr:    procMgr,
		chatSvc:    chatSvc,
		metricsSvc: metricsSvc,
		rcon:       rcon,
	}

	// Hook ProcessManager stdout stream
	if procMgr != nil {
		procMgr.SetOnLogListener(func(line string) {
			hub.BroadcastLog(line)
			if metricsSvc != nil {
				metricsSvc.IngestLogLine(line)
			}
		})
	}

	// Hook ChatService message stream
	if chatSvc != nil {
		chatSvc.SetOnMessageListener(func(msg ChatMessage) {
			hub.BroadcastChat(msg)
		})
	}

	// Start background metric ticker for connected admin clients (every 2s)
	go hub.startMetricsTicker()

	return hub
}

func (h *WSHub) startMetricsTicker() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if h.hasClients() && h.metricsSvc != nil {
			m := h.metricsSvc.CollectCurrentMetrics()
			h.BroadcastMetrics(m)
		}
	}
}

func (h *WSHub) hasClients() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients) > 0
}

// HandleWebSocket upgrades an HTTP connection to a duplex WebSocket connection.
func (h *WSHub) HandleWebSocket(w http.ResponseWriter, r *http.Request, nickname, role string) error {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return err
	}

	client := &WSClient{
		hub:      h,
		conn:     conn,
		send:     make(chan []byte, 256),
		nickname: nickname,
		role:     role,
	}

	h.registerClient(client)

	// Send initial snapshot on connection
	go h.sendInitialSnapshot(client)

	// Launch write and read pumps
	go client.writePump()
	go client.readPump()

	return nil
}

func (h *WSHub) registerClient(c *WSClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = true
}

func (h *WSHub) unregisterClient(c *WSClient) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

func (h *WSHub) sendInitialSnapshot(c *WSClient) {
	var logs []string
	if c.role == "admin" && h.procMgr != nil {
		logs = h.procMgr.GetLogs(200)
	}

	var chats []ChatMessage
	if h.chatSvc != nil {
		chats = h.chatSvc.GetMessagesSince(0)
	}

	var curMetrics *SystemMetrics
	if h.metricsSvc != nil {
		m := h.metricsSvc.CollectCurrentMetrics()
		curMetrics = &m
	}

	snap := map[string]interface{}{
		"chats": chats,
	}
	if curMetrics != nil {
		snap["metrics"] = curMetrics
	}
	if c.role == "admin" {
		snap["logs"] = logs
	}

	msgBytes, err := json.Marshal(WSMessage{
		Type:    "init",
		Payload: snap,
	})
	if err == nil {
		c.send <- msgBytes
	}
}

// BroadcastLog sends a console log line to all active admin clients.
func (h *WSHub) BroadcastLog(line string) {
	msgBytes, err := json.Marshal(WSMessage{
		Type:    "log",
		Payload: line,
	})
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.role == "admin" {
			select {
			case c.send <- msgBytes:
			default:
			}
		}
	}
}

// BroadcastChat sends a chat message to all connected clients.
func (h *WSHub) BroadcastChat(msg ChatMessage) {
	msgBytes, err := json.Marshal(WSMessage{
		Type:    "chat",
		Payload: msg,
	})
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- msgBytes:
		default:
		}
	}
}

// BroadcastMetrics sends real-time system metrics to all admin clients.
func (h *WSHub) BroadcastMetrics(m SystemMetrics) {
	msgBytes, err := json.Marshal(WSMessage{
		Type:    "metrics",
		Payload: m,
	})
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- msgBytes:
		default:
		}
	}
}

// client pumps
func (c *WSClient) readPump() {
	defer func() {
		c.hub.unregisterClient(c)
		_ = c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}

		var req struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Command string `json:"command"`
			Ticks   int    `json:"ticks"`
			Weather string `json:"weather"`
		}
		if err := json.Unmarshal(message, &req); err != nil {
			continue
		}

		switch req.Type {
		case "chat":
			if req.Text != "" && c.hub.chatSvc != nil && c.hub.rcon != nil {
				// Broadcast to in-game via tellraw
				tellrawCmd := fmt.Sprintf(`tellraw @a {"text":"[웹:%s] %s","color":"aqua"}`, c.nickname, req.Text)
				_, _ = c.hub.rcon.Execute(tellrawCmd)
				c.hub.chatSvc.AddMessage(c.nickname, req.Text, false)
			}
		case "command":
			if c.role == "admin" && req.Command != "" {
				var respText string
				var execErr error
				if c.hub.rcon != nil {
					res, err := c.hub.rcon.Execute(req.Command)
					if err == nil {
						respText = res
					} else {
						execErr = err
					}
				}
				// Fallback to process stdin if RCON failed or unavailable
				if execErr != nil || c.hub.rcon == nil {
					if c.hub.procMgr != nil {
						if inErr := c.hub.procMgr.WriteStdin(req.Command); inErr == nil {
							respText = fmt.Sprintf("명령어가 서버 콘솔(stdin)으로 직접 전송되었습니다: %s", req.Command)
						} else {
							respText = fmt.Sprintf("Error: RCON 실패 (%v) 및 Stdin 실패 (%v)", execErr, inErr)
						}
					} else if execErr != nil {
						respText = fmt.Sprintf("Error: %v", execErr)
					}
				}

				resMsg, _ := json.Marshal(WSMessage{
					Type: "command_result",
					Payload: map[string]string{
						"command":  req.Command,
						"response": respText,
					},
				})
				c.send <- resMsg
			}
		case "set_time":
			if c.role == "admin" && c.hub.metricsSvc != nil {
				_, _ = c.hub.metricsSvc.SetWorldTime(req.Ticks)
				m := c.hub.metricsSvc.CollectCurrentMetrics()
				c.hub.BroadcastMetrics(m)
			}
		case "set_weather":
			if c.role == "admin" && c.hub.metricsSvc != nil {
				_, _ = c.hub.metricsSvc.SetWorldWeather(req.Weather)
				m := c.hub.metricsSvc.CollectCurrentMetrics()
				c.hub.BroadcastMetrics(m)
			}
		}
	}
}

func (c *WSClient) writePump() {
	ticker := time.NewTicker(25 * time.Second)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			_, _ = w.Write(msg)
			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
