package mcservice

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusStopping = "stopping"
	maxLogBuffer   = 500
)

// Regex patterns to clean prompt residues and ANSI escape codes
var (
	ansiRegex    = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	promptOnlyRe = regexp.MustCompile(`^[>\s]+$`)
)

// ProcessStatusInfo provides snapshot info of server runtime.
type ProcessStatusInfo struct {
	Status   string `json:"status"`
	PID      int    `json:"pid"`
	Uptime   int64  `json:"uptime_seconds"`
	JarExist bool   `json:"jar_exist"`
}

// ProcessManager coordinates the Java child process lifecycle.
type ProcessManager struct {
	serverDir string
	jarName   string
	javaPath  string
	minMemory string
	maxMemory string
	rcon      RCONClient
	chatSvc   *ChatService

	mu        sync.Mutex
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	status    string
	startedAt time.Time
	logs      []string
	onLog     func(string)
}

// SetOnLogListener registers a callback invoked on each incoming clean log line.
func (pm *ProcessManager) SetOnLogListener(l func(string)) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.onLog = l
}

// NewProcessManager initializes a new process manager with chat service and prompt filters.
func NewProcessManager(serverDir, jarName, javaPath, minMemory, maxMemory string, rcon RCONClient, chatSvc *ChatService) *ProcessManager {
	pm := &ProcessManager{
		serverDir: serverDir,
		jarName:   jarName,
		javaPath:  javaPath,
		minMemory: minMemory,
		maxMemory: maxMemory,
		rcon:      rcon,
		chatSvc:   chatSvc,
		status:    StatusStopped,
		logs:      make([]string, 0, maxLogBuffer),
	}
	pm.preloadExistingLogs()
	return pm
}

func (pm *ProcessManager) preloadExistingLogs() {
	logPath := filepath.Join(pm.serverDir, "logs", "latest.log")
	data, err := os.ReadFile(logPath)
	if err != nil || len(data) == 0 {
		return
	}
	lines := strings.Split(string(data), "\n")
	start := 0
	if len(lines) > 200 {
		start = len(lines) - 200
	}
	for _, l := range lines[start:] {
		clean := sanitizeConsoleLine(l)
		if clean != "" {
			pm.logs = append(pm.logs, clean)
		}
	}
}

// appendLog stores a clean log line in memory ring buffer and feeds into chat parser and websocket.
func (pm *ProcessManager) appendLog(line string) {
	clean := sanitizeConsoleLine(line)
	if clean == "" {
		return
	}

	pm.mu.Lock()
	if len(pm.logs) >= maxLogBuffer {
		pm.logs = pm.logs[1:]
	}
	pm.logs = append(pm.logs, clean)
	listener := pm.onLog
	pm.mu.Unlock()

	if listener != nil {
		listener(clean)
	}

	// Ingest into ChatService for real-time web chat sync
	if pm.chatSvc != nil {
		pm.chatSvc.IngestLogLine(clean)
	}
}

// sanitizeConsoleLine removes JLine prompt artifacts (> > > >) and ANSI sequences.
func sanitizeConsoleLine(raw string) string {
	stripped := ansiRegex.ReplaceAllString(raw, "")
	trimmed := strings.TrimSpace(stripped)

	// Discard lines that contain only '>' and whitespace (fixes the "> > > >" prompt issue)
	if promptOnlyRe.MatchString(trimmed) {
		return ""
	}

	// Trim leading prompts like "> [19:00:00 INFO]"
	for strings.HasPrefix(trimmed, "> ") {
		trimmed = strings.TrimPrefix(trimmed, "> ")
	}

	return trimmed
}

// GetLogs returns the most recent log entries.
func (pm *ProcessManager) GetLogs(limit int) []string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if limit <= 0 || limit > len(pm.logs) {
		limit = len(pm.logs)
	}

	start := len(pm.logs) - limit
	result := make([]string, limit)
	copy(result, pm.logs[start:])
	return result
}

// GetStatus returns the current status.
func (pm *ProcessManager) GetStatus() ProcessStatusInfo {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var uptime int64
	pid := 0
	status := pm.status

	if pm.cmd != nil && pm.cmd.Process != nil && (pm.status == StatusRunning || pm.status == StatusStarting) {
		uptime = int64(time.Since(pm.startedAt).Seconds())
		pid = pm.cmd.Process.Pid
	} else if pm.status == StatusStopped {
		if orphanPID := findRunningServerPID(pm.jarName); orphanPID > 0 {
			pid = orphanPID
			status = StatusRunning
		}
	}

	jarPath := filepath.Join(pm.serverDir, pm.jarName)
	jarExists := fileExists(jarPath)

	return ProcessStatusInfo{
		Status:   status,
		PID:      pid,
		Uptime:   uptime,
		JarExist: jarExists,
	}
}

// Start launches the Minecraft server Java process with disabled JLine prompts to prevent "> > > >" spam.
func (pm *ProcessManager) Start() error {
	pm.mu.Lock()
	if pm.status == StatusRunning || pm.status == StatusStarting {
		pm.mu.Unlock()
		return errors.New("server process is already running or starting")
	}

	jarPath := filepath.Join(pm.serverDir, pm.jarName)
	if !fileExists(jarPath) {
		pm.mu.Unlock()
		return fmt.Errorf("server jar not found at %s: please download Purpur first", jarPath)
	}

	// Clean up any stray/orphaned Java server process targeting this jar before starting a new one
	if strayPID := findRunningServerPID(pm.jarName); strayPID > 0 {
		pm.appendLog(fmt.Sprintf("[Manager] Cleaning up stray server process (PID: %d) before start...", strayPID))
		if p, err := os.FindProcess(strayPID); err == nil {
			_ = p.Signal(syscall.SIGTERM)
			time.Sleep(1 * time.Second)
			_ = p.Kill()
		}
	}

	// Auto-ensure server properties, dimension structure links, and BlueMap configs
	EnsureServerProperties(pm.serverDir)
	EnsureWorldStructure(pm.serverDir)
	EnsureBlueMapConfig(pm.serverDir)
	_ = EnsureSquaremapConfig(pm.serverDir, 8100)

	// Disable JLine terminal prompts and ANSI colors when piping stdout
	args := []string{
		fmt.Sprintf("-Xms%s", pm.minMemory),
		fmt.Sprintf("-Xmx%s", pm.maxMemory),
		"-Dterminal.jline=false",
		"-Dterminal.ansi=false",
		"-jar",
		pm.jarName,
		"nogui",
	}

	cmd := exec.Command(pm.javaPath, args...)
	cmd.Dir = pm.serverDir

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		pm.mu.Unlock()
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		pm.mu.Unlock()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		pm.mu.Unlock()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		pm.mu.Unlock()
		return fmt.Errorf("failed to start Java process: %w", err)
	}

	pm.cmd = cmd
	pm.stdin = stdinPipe
	pm.status = StatusStarting
	pm.startedAt = time.Now()
	pm.mu.Unlock()

	pm.appendLog(fmt.Sprintf("[Manager] Server process started (PID: %d)", cmd.Process.Pid))

	go pm.streamOutput(stdoutPipe)
	go pm.streamOutput(stderrPipe)

	go func() {
		time.AfterFunc(12*time.Second, func() {
			pm.mu.Lock()
			if pm.status == StatusStarting {
				pm.status = StatusRunning
			}
			pm.mu.Unlock()
		})

		_ = cmd.Wait()

		pm.mu.Lock()
		pm.status = StatusStopped
		pm.cmd = nil
		pm.stdin = nil
		pm.mu.Unlock()
		pm.appendLog("[Manager] Server process exited.")
	}()

	return nil
}

// Stop initiates a graceful shutdown using RCON stop and stdin fallback.
func (pm *ProcessManager) Stop() error {
	pm.mu.Lock()
	orphanPID := 0
	if pm.cmd == nil {
		orphanPID = findRunningServerPID(pm.jarName)
		if orphanPID == 0 {
			pm.mu.Unlock()
			return errors.New("server is not running")
		}
	} else if pm.status == StatusStopped || pm.status == StatusStopping {
		pm.mu.Unlock()
		return errors.New("server is not running")
	}

	pm.status = StatusStopping
	cmd := pm.cmd
	stdin := pm.stdin
	pm.mu.Unlock()

	pm.appendLog("[Manager] Initiating graceful server stop...")

	// 1. Dispatch RCON stop
	go func() {
		if pm.rcon != nil {
			_, _ = pm.rcon.Execute("stop")
		}
	}()

	// 2. Also write "stop\n" to process stdin in case RCON is disconnected or disabled
	if stdin != nil {
		_, _ = stdin.Write([]byte("stop\n"))
	}

	// 3. Monitor termination in background with phased SIGTERM / SIGKILL fallback
	go func() {
		if orphanPID > 0 {
			p, err := os.FindProcess(orphanPID)
			if err == nil {
				time.Sleep(2 * time.Second)
				_ = p.Signal(syscall.SIGTERM)
				time.Sleep(2 * time.Second)
				_ = p.Kill()
			}
			pm.mu.Lock()
			pm.status = StatusStopped
			pm.mu.Unlock()
			pm.appendLog(fmt.Sprintf("[Manager] Orphaned server process (PID: %d) terminated.", orphanPID))
			return
		}

		if cmd == nil || cmd.Process == nil {
			return
		}

		// Wait up to 10 seconds for clean shutdown
		for i := 0; i < 20; i++ {
			time.Sleep(500 * time.Millisecond)
			pm.mu.Lock()
			stopped := (pm.cmd == nil)
			pm.mu.Unlock()
			if stopped {
				return
			}
		}

		// Phase 2: Send SIGTERM if process is still alive after 10s
		pm.appendLog("[Manager] Graceful stop timed out (10s). Sending SIGTERM...")
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
			time.Sleep(3 * time.Second)
			pm.mu.Lock()
			if pm.cmd != nil && cmd.Process != nil {
				pm.appendLog("[Manager] Process still alive after SIGTERM. Forcing SIGKILL...")
				_ = cmd.Process.Kill()
			}
			pm.mu.Unlock()
		}
	}()

	return nil
}

// findRunningServerPID searches for an already running Minecraft server process targeting this server jar.
func findRunningServerPID(jarName string) int {
	out, err := exec.Command("pgrep", "-f", jarName).Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, l := range lines {
		if pid, err := strconv.Atoi(strings.TrimSpace(l)); err == nil && pid != os.Getpid() {
			return pid
		}
	}
	return 0
}

// Restart stops and restarts the server process.
func (pm *ProcessManager) Restart() error {
	if err := pm.Stop(); err != nil {
		log.Printf("[WARN] Error stopping during restart: %v", err)
	}
	time.Sleep(2 * time.Second)
	return pm.Start()
}

func (pm *ProcessManager) streamOutput(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		text := scanner.Text()
		pm.appendLog(text)
	}
}

// WriteStdin sends a command string directly to the Minecraft server's standard input pipe.
func (pm *ProcessManager) WriteStdin(cmd string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.stdin == nil || pm.status != StatusRunning && pm.status != StatusStarting {
		return errors.New("server stdin pipe is not available or server is not running")
	}

	clean := strings.TrimSpace(cmd)
	if clean == "" {
		return nil
	}

	_, err := pm.stdin.Write([]byte(clean + "\n"))
	return err
}

// EnsureServerProperties guarantees that server.properties enables RCON and disables secure profile enforcement for in-game chat.
func EnsureServerProperties(serverDir string) {
	propPath := filepath.Join(serverDir, "server.properties")
	data, err := os.ReadFile(propPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Pre-seed server.properties so RCON, Whitelist, and unauthenticated chat work out of the box
			preseed := `# Minecraft server properties pre-seeded by Sanctum Manager
enable-rcon=true
rcon.port=25575
rcon.password=rcon_password
white-list=true
enforce-whitelist=true
enforce-secure-profile=false
`
			_ = os.WriteFile(propPath, []byte(preseed), 0644)
		}
		return
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	hasRconEnable := false
	hasRconPort := false
	hasRconPass := false
	hasEnforceWhitelist := false
	hasEnforceSecureProfile := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "enable-rcon=") {
			lines[i] = "enable-rcon=true"
			hasRconEnable = true
		} else if strings.HasPrefix(trimmed, "rcon.port=") {
			lines[i] = "rcon.port=25575"
			hasRconPort = true
		} else if strings.HasPrefix(trimmed, "rcon.password=") {
			lines[i] = "rcon.password=rcon_password"
			hasRconPass = true
		} else if strings.HasPrefix(trimmed, "enforce-whitelist=") {
			lines[i] = "enforce-whitelist=true"
			hasEnforceWhitelist = true
		} else if strings.HasPrefix(trimmed, "enforce-secure-profile=") {
			lines[i] = "enforce-secure-profile=false"
			hasEnforceSecureProfile = true
		}
	}

	if !hasRconEnable {
		lines = append(lines, "enable-rcon=true")
	}
	if !hasRconPort {
		lines = append(lines, "rcon.port=25575")
	}
	if !hasRconPass {
		lines = append(lines, "rcon.password=rcon_password")
	}
	if !hasEnforceWhitelist {
		lines = append(lines, "enforce-whitelist=true")
	}
	if !hasEnforceSecureProfile {
		lines = append(lines, "enforce-secure-profile=false")
	}

	_ = os.WriteFile(propPath, []byte(strings.Join(lines, "\n")), 0644)
}

// EnsureWorldStructure cleans up any legacy forbidden symlinks in the world folder
// to prevent Minecraft 26.3 ContentValidationException.
func EnsureWorldStructure(serverDir string) {
	worldDir := filepath.Join(serverDir, "world")
	if _, err := os.Stat(worldDir); err != nil {
		return
	}

	// Clean up any forbidden symlinks in worldDir to prevent ContentValidationException
	_ = filepath.Walk(worldDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			_ = os.Remove(path)
		}
		return nil
	})
}

// EnsureBlueMapConfig guarantees core download consent, live player position persistence, and vanilla map configurations.
func EnsureBlueMapConfig(serverDir string) {
	bmDir := filepath.Join(serverDir, "plugins", "BlueMap")
	_ = os.MkdirAll(bmDir, 0755)

	// 1. core.conf: accept-download: true
	coreConf := filepath.Join(bmDir, "core.conf")
	data, err := os.ReadFile(coreConf)
	if err == nil {
		content := string(data)
		re := regexp.MustCompile(`(?m)^\s*accept-download\s*:\s*false\s*$`)
		if re.MatchString(content) {
			newContent := re.ReplaceAllString(content, "accept-download: true")
			_ = os.WriteFile(coreConf, []byte(newContent), 0644)
		} else if !strings.Contains(content, "accept-download") {
			newContent := "accept-download: true\n" + content
			_ = os.WriteFile(coreConf, []byte(newContent), 0644)
		}
	} else {
		preseed := `# BlueMap core configuration pre-seeded by Sanctum Manager
accept-download: true
data: "bluemap"
metrics: true
`
		_ = os.WriteFile(coreConf, []byte(preseed), 0644)
	}

	// 2. plugin.conf: enable write-players-interval: 1 and write-markers-interval: 2 so web UI displays live players
	pluginConf := filepath.Join(bmDir, "plugin.conf")
	pData, err := os.ReadFile(pluginConf)
	if err == nil {
		pContent := string(pData)
		// Enable writing players to disk every 1 second
		pContent = regexp.MustCompile(`(?m)^#?\s*write-players-interval\s*:\s*\d+`).ReplaceAllString(pContent, "write-players-interval: 1")
		// Enable writing markers to disk every 2 seconds
		pContent = regexp.MustCompile(`(?m)^#?\s*write-markers-interval\s*:\s*\d+`).ReplaceAllString(pContent, "write-markers-interval: 2")
		// Ensure skin download and live markers
		pContent = regexp.MustCompile(`(?m)^#?\s*live-player-markers\s*:\s*(false|true)`).ReplaceAllString(pContent, "live-player-markers: true")
		pContent = regexp.MustCompile(`(?m)^#?\s*skin-download\s*:\s*(false|true)`).ReplaceAllString(pContent, "skin-download: true")
		_ = os.WriteFile(pluginConf, []byte(pContent), 0644)
	}

	// 3. Ensure maps/*.conf point to vanilla "world"
	mapsDir := filepath.Join(bmDir, "maps")
	entries, err := os.ReadDir(mapsDir)
	if err == nil {
		worldDimRe := regexp.MustCompile(`(?m)^\s*world\s*:\s*["']?[^"'\n\r]+["']?`)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".conf") {
				confPath := filepath.Join(mapsDir, e.Name())
				cData, err := os.ReadFile(confPath)
				if err == nil {
					cStr := string(cData)
					if worldDimRe.MatchString(cStr) {
						cStr = worldDimRe.ReplaceAllString(cStr, `world: "world"`)
						_ = os.WriteFile(confPath, []byte(cStr), 0644)
					}
				}
			}
		}
	}
}

