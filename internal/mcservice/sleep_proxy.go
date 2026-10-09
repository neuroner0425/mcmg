package mcservice

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"minecraft_server_manager/internal/config"
)

// SleepProxy provides a lightweight Minecraft TCP handshake proxy
// that holds port 25565 when the server is asleep, responding to server list pings
// and automatically booting the server with 0-memory usage during idle periods.
type SleepProxy struct {
	cfg        config.AutoSleepConfig
	procMgr    *ProcessManager
	metricsSvc *MetricsService

	listener net.Listener
	mu       sync.Mutex
	stopCh   chan struct{}
	idleTime time.Duration
}

// NewSleepProxy creates a new sleep proxy instance.
func NewSleepProxy(cfg config.AutoSleepConfig, procMgr *ProcessManager, metricsSvc *MetricsService) *SleepProxy {
	if cfg.Port <= 0 {
		cfg.Port = 25565
	}
	if cfg.WakeMOTD == "" {
		cfg.WakeMOTD = "§6[Sanctum Server] §e절전 모드 대기 중\n§a접속 시 자동으로 서버가 켜집니다"
	}
	if cfg.WakeMessage == "" {
		cfg.WakeMessage = "§6[Sanctum Server]§r\n\n§e서버가 절전 모드에서 기동 중입니다! (약 8초 소요)\n§a잠시 후 다시 접속해주세요."
	}

	return &SleepProxy{
		cfg:        cfg,
		procMgr:    procMgr,
		metricsSvc: metricsSvc,
		stopCh:     make(chan struct{}),
	}
}

// Start begins the auto-sleep monitoring loop and listener management.
func (sp *SleepProxy) Start() {
	if !sp.cfg.Enabled {
		return
	}
	go sp.monitorLoop()
}

// Stop terminates the sleep proxy and frees the listener.
func (sp *SleepProxy) Stop() {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	select {
	case <-sp.stopCh:
	default:
		close(sp.stopCh)
	}

	if sp.listener != nil {
		_ = sp.listener.Close()
		sp.listener = nil
	}
}

func (sp *SleepProxy) monitorLoop() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sp.stopCh:
			return
		case <-ticker.C:
			sp.checkState()
		}
	}
}

func (sp *SleepProxy) checkState() {
	st := sp.procMgr.GetStatus().Status

	switch st {
	case StatusRunning:
		// When Minecraft is actively running, close the proxy listener so Purpur has direct control
		sp.mu.Lock()
		if sp.listener != nil {
			_ = sp.listener.Close()
			sp.listener = nil
		}
		sp.mu.Unlock()

		// Check idle timeout
		if sp.cfg.IdleTimeoutMinutes > 0 && sp.metricsSvc != nil {
			m := sp.metricsSvc.CollectCurrentMetrics()
			if m.ServerRunning {
				if m.OnlinePlayers == 0 {
					sp.idleTime += 15 * time.Second
					if sp.idleTime >= time.Duration(sp.cfg.IdleTimeoutMinutes)*time.Minute {
						log.Printf("[SleepProxy] No players online for %d minutes. Initiating graceful stop for zero-memory deep sleep...", sp.cfg.IdleTimeoutMinutes)
						sp.procMgr.appendLog(fmt.Sprintf("[Auto-Sleep] 접속자가 없어 절전 모드로 진입합니다 (%d분 유휴). 메모리를 100%% 반환합니다.", sp.cfg.IdleTimeoutMinutes))
						_ = sp.procMgr.Stop()
						sp.idleTime = 0
					}
				} else {
					sp.idleTime = 0
				}
			}
		}

	case StatusStopped:
		// When server is stopped, bind 25565 to serve wake requests
		sp.mu.Lock()
		if sp.listener == nil {
			addr := fmt.Sprintf(":%d", sp.cfg.Port)
			l, err := net.Listen("tcp", addr)
			if err == nil {
				sp.listener = l
				log.Printf("[SleepProxy] Minecraft server is asleep. Listening on %s for auto-wake...", addr)
				sp.procMgr.appendLog(fmt.Sprintf("[Auto-Sleep] 절전 모드 대기 중 (포트 %d). 플레이어 접속 시 자동으로 기동됩니다.", sp.cfg.Port))
				go sp.acceptLoop(l)
			}
		}
		sp.mu.Unlock()

	default: // Starting or Stopping
		sp.mu.Lock()
		if sp.listener != nil {
			_ = sp.listener.Close()
			sp.listener = nil
		}
		sp.mu.Unlock()
	}
}

func (sp *SleepProxy) acceptLoop(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return // Listener closed
		}
		go sp.handleConn(conn)
	}
}

func (sp *SleepProxy) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	// 1. Read Handshake Packet (ID 0x00)
	packetID, payload, err := readPacket(conn)
	if err != nil || packetID != 0x00 {
		return
	}

	reader := bytes.NewReader(payload)
	protoVersion, err := readVarInt(reader)
	if err != nil {
		return
	}

	_, _ = readString(reader) // server address (ignored)
	var port uint16
	_ = binary.Read(reader, binary.BigEndian, &port)

	nextState, err := readVarInt(reader)
	if err != nil {
		return
	}

	// 2. State 1: Status (Server List Ping)
	if nextState == 1 {
		// Read Request packet (0x00)
		reqID, _, err := readPacket(conn)
		if err != nil || reqID != 0x00 {
			return
		}

		// Write Status Response JSON
		statusJSON, _ := json.Marshal(map[string]interface{}{
			"version": map[string]interface{}{
				"name":     "1.21.4 (절전 모드)",
				"protocol": protoVersion,
			},
			"players": map[string]interface{}{
				"max":    20,
				"online": 0,
				"sample": []interface{}{},
			},
			"description": map[string]interface{}{
				"text": sp.cfg.WakeMOTD,
			},
		})

		var respBuf bytes.Buffer
		_ = writeString(&respBuf, string(statusJSON))
		_ = writePacket(conn, 0x00, respBuf.Bytes())

		// Read Ping Packet (0x01) and reply with Pong
		pingID, pingPayload, err := readPacket(conn)
		if err == nil && pingID == 0x01 {
			_ = writePacket(conn, 0x01, pingPayload)
		}
		return
	}

	// 3. State 2: Login (Player Connecting -> Trigger Wake)
	if nextState == 2 {
		// Read Login Start Packet (0x00)
		_, _, _ = readPacket(conn)

		// Send Disconnect (Login) with wake notification
		disconnectJSON, _ := json.Marshal(map[string]interface{}{
			"text": sp.cfg.WakeMessage,
		})

		var discBuf bytes.Buffer
		_ = writeString(&discBuf, string(disconnectJSON))
		_ = writePacket(conn, 0x00, discBuf.Bytes())

		log.Printf("[SleepProxy] Player join attempt detected! Waking Minecraft server...")
		sp.procMgr.appendLog("[Auto-Sleep] 플레이어 접속 감지! 서버를 자동으로 깨웁니다...")

		// Release port 25565 immediately so Java can bind it
		sp.mu.Lock()
		if sp.listener != nil {
			_ = sp.listener.Close()
			sp.listener = nil
		}
		sp.mu.Unlock()

		// Start Minecraft in background
		go func() {
			time.Sleep(100 * time.Millisecond)
			if err := sp.procMgr.Start(); err != nil {
				log.Printf("[SleepProxy] Auto-wake start error: %v", err)
			}
		}()
	}
}

// Wire protocol VarInt and packet framing helpers

func readVarInt(r io.Reader) (int, error) {
	var val int
	var shift uint
	buf := make([]byte, 1)
	for {
		if _, err := io.ReadFull(r, buf); err != nil {
			return 0, err
		}
		b := buf[0]
		val |= int(b&0x7F) << shift
		if (b & 0x80) == 0 {
			break
		}
		shift += 7
		if shift >= 35 {
			return 0, errors.New("varint is too big")
		}
	}
	return val, nil
}

func writeVarInt(w io.Writer, val int) error {
	u := uint32(val)
	for {
		if (u & ^uint32(0x7F)) == 0 {
			_, err := w.Write([]byte{byte(u)})
			return err
		}
		if _, err := w.Write([]byte{byte((u & 0x7F) | 0x80)}); err != nil {
			return err
		}
		u >>= 7
	}
}

func readString(r io.Reader) (string, error) {
	length, err := readVarInt(r)
	if err != nil {
		return "", err
	}
	if length < 0 || length > 32767 {
		return "", errors.New("invalid string length")
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func writeString(w io.Writer, s string) error {
	if err := writeVarInt(w, len(s)); err != nil {
		return err
	}
	_, err := w.Write([]byte(s))
	return err
}

func readPacket(r io.Reader) (int, []byte, error) {
	length, err := readVarInt(r)
	if err != nil {
		return 0, nil, err
	}
	if length <= 0 || length > 1048576 {
		return 0, nil, errors.New("invalid packet length")
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(r, data); err != nil {
		return 0, nil, err
	}
	buf := bytes.NewReader(data)
	packetID, err := readVarInt(buf)
	if err != nil {
		return 0, nil, err
	}
	payload := data[len(data)-buf.Len():]
	return packetID, payload, nil
}

func writePacket(w io.Writer, packetID int, payload []byte) error {
	var body bytes.Buffer
	if err := writeVarInt(&body, packetID); err != nil {
		return err
	}
	body.Write(payload)

	var frame bytes.Buffer
	if err := writeVarInt(&frame, body.Len()); err != nil {
		return err
	}
	frame.Write(body.Bytes())

	_, err := w.Write(frame.Bytes())
	return err
}
