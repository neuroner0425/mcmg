package mcservice

import (
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ChatMessage represents a chat line originating from in-game or web.
type ChatMessage struct {
	ID        int64     `json:"id"`
	Sender    string    `json:"sender"`
	Text      string    `json:"text"`
	IsSystem  bool      `json:"is_system"`
	Timestamp time.Time `json:"timestamp"`
}

// ChatService stores and parses in-game chat events.
type ChatService struct {
	mu        sync.RWMutex
	messages  []ChatMessage
	maxMsg    int
	lastID    atomic.Int64
	onMessage func(ChatMessage)
}

// SetOnMessageListener registers a callback invoked whenever a chat message is recorded.
func (cs *ChatService) SetOnMessageListener(l func(ChatMessage)) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.onMessage = l
}

// NewChatService initializes a new in-memory ChatService.
func NewChatService(capacity int) *ChatService {
	if capacity <= 0 {
		capacity = 200
	}
	return &ChatService{
		messages: make([]ChatMessage, 0, capacity),
		maxMsg:   capacity,
	}
}

// Regex patterns to detect Minecraft chat and player join/quit events
var (
	// Matches: [HH:MM:SS INFO]: <Player> Hello
	// Matches: [HH:MM:SS INFO]: [Not Secure] <Player> Hello
	playerChatRegex = regexp.MustCompile(`\[.*?INFO\]:\s*(?:\[Not Secure\]\s*)?<([^>]+)>\s*(.*)`)

	// Matches: [HH:MM:SS INFO]: Player joined the game / left the game
	playerJoinQuitRegex = regexp.MustCompile(`\[.*?INFO\]:\s*(\w+)\s+(joined the game|left the game)`)
)

// IngestLogLine inspects a server output line and extracts chat or join/leave notifications.
func (cs *ChatService) IngestLogLine(line string) {
	cleanLine := strings.TrimSpace(line)

	// 1. Check for standard player chat
	if matches := playerChatRegex.FindStringSubmatch(cleanLine); len(matches) >= 3 {
		sender := strings.TrimSpace(matches[1])
		msg := strings.TrimSpace(matches[2])
		cs.AddMessage(sender, msg, false)
		return
	}

	// 2. Check for player join / leave event
	if matches := playerJoinQuitRegex.FindStringSubmatch(cleanLine); len(matches) >= 3 {
		player := matches[1]
		action := matches[2]
		sysMsg := player + " 님이 " + action
		if action == "joined the game" {
			sysMsg = player + " 님이 서버에 접속했습니다."
		} else if action == "left the game" {
			sysMsg = player + " 님이 서버에서 퇴장했습니다."
		}
		cs.AddMessage("System", sysMsg, true)
		return
	}
}

// AddMessage appends a chat entry to the ring buffer and notifies websocket listeners.
func (cs *ChatService) AddMessage(sender, text string, isSystem bool) ChatMessage {
	id := cs.lastID.Add(1)
	entry := ChatMessage{
		ID:        id,
		Sender:    sender,
		Text:      text,
		IsSystem:  isSystem,
		Timestamp: time.Now(),
	}

	cs.mu.Lock()
	if len(cs.messages) >= cs.maxMsg {
		cs.messages = cs.messages[1:]
	}
	cs.messages = append(cs.messages, entry)
	listener := cs.onMessage
	cs.mu.Unlock()

	if listener != nil {
		listener(entry)
	}

	return entry
}

// GetMessagesSince returns all messages with ID > sinceID.
func (cs *ChatService) GetMessagesSince(sinceID int64) []ChatMessage {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	if len(cs.messages) == 0 {
		return []ChatMessage{}
	}

	var result []ChatMessage
	for _, msg := range cs.messages {
		if msg.ID > sinceID {
			result = append(result, msg)
		}
	}

	return result
}
