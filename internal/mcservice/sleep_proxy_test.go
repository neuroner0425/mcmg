package mcservice

import (
	"bytes"
	"net"
	"testing"
	"time"

	"minecraft_server_manager/internal/config"
)

func TestVarIntReadWrite(t *testing.T) {
	testValues := []int{0, 1, 255, 25565, 769, 1048576}
	for _, val := range testValues {
		var buf bytes.Buffer
		if err := writeVarInt(&buf, val); err != nil {
			t.Fatalf("failed to write varint %d: %v", val, err)
		}
		readVal, err := readVarInt(&buf)
		if err != nil {
			t.Fatalf("failed to read varint for %d: %v", val, err)
		}
		if readVal != val {
			t.Errorf("expected %d, got %d", val, readVal)
		}
	}
}

func TestSleepProxyStatusPing(t *testing.T) {
	cfg := config.AutoSleepConfig{
		Enabled:            true,
		Port:               25599, // use test port
		IdleTimeoutMinutes: 0,
		WakeMOTD:           "Test MOTD",
		WakeMessage:        "Test Wake",
	}

	sp := NewSleepProxy(cfg, nil, nil)
	l, err := net.Listen("tcp", "127.0.0.1:25599")
	if err != nil {
		t.Fatalf("failed to listen on test port: %v", err)
	}
	defer l.Close()
	sp.listener = l

	go sp.acceptLoop(l)

	// Connect as client
	conn, err := net.Dial("tcp", "127.0.0.1:25599")
	if err != nil {
		t.Fatalf("failed to dial sleep proxy: %v", err)
	}
	defer conn.Close()

	// Send Handshake packet (State = 1, Status)
	var hsPayload bytes.Buffer
	_ = writeVarInt(&hsPayload, 769)
	_ = writeString(&hsPayload, "localhost")
	_ = writeVarInt(&hsPayload, 25599) // port as varint or ushort
	_ = writeVarInt(&hsPayload, 1)     // nextState = 1 (status)
	_ = writePacket(conn, 0x00, hsPayload.Bytes())

	// Send Status Request packet (0x00, empty)
	_ = writePacket(conn, 0x00, nil)

	// Read Status Response packet
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	respID, respPayload, err := readPacket(conn)
	if err != nil {
		t.Fatalf("failed to read status response: %v", err)
	}
	if respID != 0x00 {
		t.Errorf("expected packet id 0x00, got 0x%02x", respID)
	}

	reader := bytes.NewReader(respPayload)
	jsonStr, err := readString(reader)
	if err != nil {
		t.Fatalf("failed to read json string: %v", err)
	}
	if !bytes.Contains([]byte(jsonStr), []byte("Test MOTD")) {
		t.Errorf("expected response to contain 'Test MOTD', got %s", jsonStr)
	}
}
