package mcservice

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorcon/rcon"
)

// RCONClient defines the interface for communicating with Minecraft via RCON.
type RCONClient interface {
	Execute(command string) (string, error)
	GetPlayers() (players []string, online int, max int, err error)
	SendChatMessage(sender, message string) error
}

// Service manages RCON interactions.
type Service struct {
	address    string
	password   string
	mu         sync.Mutex
	conn       *rcon.Conn
	lastErrLog time.Time
}

// NewService initializes a new RCON-based Minecraft service.
func NewService(address, password string) *Service {
	return &Service{
		address:  address,
		password: password,
	}
}

func (s *Service) logError(format string, v ...interface{}) {
	now := time.Now()
	if now.Sub(s.lastErrLog) > 15*time.Second {
		log.Printf("[RCON] "+format, v...)
		s.lastErrLog = now
	}
}

// getConn retrieves an active connection or dials a new one.
func (s *Service) getConn() (*rcon.Conn, error) {
	if s.conn != nil {
		return s.conn, nil
	}

	conn, err := rcon.Dial(s.address, s.password, rcon.SetDialTimeout(3*time.Second))
	if err != nil {
		s.logError("failed to connect to RCON (%s): %v", s.address, err)
		return nil, fmt.Errorf("failed to connect to RCON (%s): %w", s.address, err)
	}

	s.conn = conn
	return s.conn, nil
}

// closeConn closes the existing connection if any.
func (s *Service) closeConn() {
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}

// Execute runs a raw console command through RCON with automatic reconnect on socket errors.
func (s *Service) Execute(command string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	conn, err := s.getConn()
	if err != nil {
		return "", err
	}

	res, err := conn.Execute(command)
	if err != nil {
		s.logError("command %q failed, attempting reconnect: %v", command, err)
		// Attempt reconnect once upon network failure
		s.closeConn()
		conn, err = s.getConn()
		if err != nil {
			return "", fmt.Errorf("reconnection failed: %w", err)
		}
		res, err = conn.Execute(command)
		if err != nil {
			s.closeConn()
			s.logError("command %q failed again after reconnect: %v", command, err)
			return "", fmt.Errorf("command execution failed: %w", err)
		}
	}

	return res, nil
}

// playerListRegex parses outputs like:
// "There are 2 of a max of 20 players online: Steve, Alex"
var playerListRegex = regexp.MustCompile(`There are (\d+) of a max of? (\d+) players online(?::\s*(.*))?`)

// GetPlayers retrieves the current list of online players along with count metrics.
func (s *Service) GetPlayers() ([]string, int, int, error) {
	output, err := s.Execute("list")
	if err != nil {
		return nil, 0, 0, err
	}

	return ParsePlayerList(output)
}

// ParsePlayerList parses the output of the 'list' command.
func ParsePlayerList(output string) ([]string, int, int, error) {
	trimmed := strings.TrimSpace(output)
	matches := playerListRegex.FindStringSubmatch(trimmed)
	if len(matches) < 3 {
		return nil, 0, 0, fmt.Errorf("unexpected output from list command: %q", trimmed)
	}

	online, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, 0, 0, errors.New("invalid online player count")
	}

	maxPlayers, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, 0, 0, errors.New("invalid max player count")
	}

	var players []string
	if len(matches) >= 4 && strings.TrimSpace(matches[3]) != "" {
		rawList := strings.Split(matches[3], ",")
		for _, p := range rawList {
			name := strings.TrimSpace(p)
			if name != "" {
				players = append(players, name)
			}
		}
	}

	return players, online, maxPlayers, nil
}

// SendChatMessage broadcasts a web-originating message to in-game chat using /tellraw.
func (s *Service) SendChatMessage(sender, message string) error {
	trimmedSender := strings.TrimSpace(sender)
	trimmedMsg := strings.TrimSpace(message)

	if trimmedSender == "" {
		return errors.New("sender name cannot be empty")
	}
	if trimmedMsg == "" {
		return errors.New("chat message cannot be empty")
	}

	// Craft Minecraft JSON Text Component for /tellraw
	type Component struct {
		Text  string `json:"text"`
		Color string `json:"color,omitempty"`
		Bold  bool   `json:"bold,omitempty"`
	}

	payload := []Component{
		{Text: "[Web] ", Color: "aqua", Bold: true},
		{Text: fmt.Sprintf("<%s> ", trimmedSender), Color: "yellow"},
		{Text: trimmedMsg, Color: "white"},
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to format chat json: %w", err)
	}

	cmd := fmt.Sprintf("tellraw @a %s", string(jsonBytes))
	_, err = s.Execute(cmd)
	if err != nil {
		return fmt.Errorf("failed to send chat to server: %w", err)
	}

	return nil
}
