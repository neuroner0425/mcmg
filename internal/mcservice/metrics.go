package mcservice

import (
	"bytes"
	"fmt"
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

var tpsRegex = regexp.MustCompile(`(?:§[0-9a-fk-or])*([0-9]+\.[0-9]+)`)

type SystemMetrics struct {
	Timestamp       string  `json:"timestamp"`
	ServerRunning   bool    `json:"server_running"`
	ProcessPID      int     `json:"process_pid"`
	ProcessUptime   int64   `json:"process_uptime"`
	ProcessCPU      float64 `json:"process_cpu_percent"`
	ProcessMemoryMB float64 `json:"process_memory_mb"`
	DiskTotalGB     float64 `json:"disk_total_gb"`
	DiskFreeGB      float64 `json:"disk_free_gb"`
	DiskUsedGB      float64 `json:"disk_used_gb"`
	DiskUsedPct     float64 `json:"disk_used_pct"`
	TPS1m           float64 `json:"tps_1m"`
	TPS5m           float64 `json:"tps_5m"`
	TPS15m          float64 `json:"tps_15m"`
	OnlinePlayers   int     `json:"online_players"`
	MaxPlayers      int     `json:"max_players"`
	WorldTimeTicks  int     `json:"world_time_ticks"`
	WorldTimeString string  `json:"world_time_string"`
	WorldPhase      string  `json:"world_phase"`
	WorldWeather    string  `json:"world_weather"`
}

type MetricsService struct {
	serverDir      string
	procMgr        *ProcessManager
	rcon           RCONClient
	propMgr        *PropertiesManager
	historyMu      sync.RWMutex
	history        []SystemMetrics
	maxHist        int
	envMu          sync.RWMutex
	currentWeather string
	currentTime    int
}

func NewMetricsService(serverDir string, procMgr *ProcessManager, rcon RCONClient, propMgr *PropertiesManager) *MetricsService {
	ms := &MetricsService{
		serverDir:      serverDir,
		procMgr:        procMgr,
		rcon:           rcon,
		propMgr:        propMgr,
		history:        make([]SystemMetrics, 0, 60),
		maxHist:        60,
		currentWeather: "clear",
		currentTime:    6000,
	}
	ms.initWeatherFromLog()
	return ms
}

func (ms *MetricsService) CollectCurrentMetrics() SystemMetrics {
	now := time.Now().Format("15:04:05")
	procStatus := ms.procMgr.GetStatus()
	running := (procStatus.Status == StatusRunning)

	m := SystemMetrics{
		Timestamp:     now,
		ServerRunning: running,
		ProcessPID:    procStatus.PID,
		ProcessUptime: procStatus.Uptime,
		TPS1m:         20.0,
		TPS5m:         20.0,
		TPS15m:        20.0,
	}

	// 1. Process CPU and Memory
	if running && procStatus.PID > 0 {
		out, err := exec.Command("ps", "-o", "%cpu,rss", "-p", strconv.Itoa(procStatus.PID)).Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(lines) >= 2 {
				fields := strings.Fields(lines[1])
				if len(fields) >= 2 {
					if cpu, err := strconv.ParseFloat(fields[0], 64); err == nil {
						m.ProcessCPU = cpu
					}
					if rssKB, err := strconv.ParseFloat(fields[1], 64); err == nil {
						m.ProcessMemoryMB = rssKB / 1024.0
					}
				}
			}
		}
	}

	// 2. Disk Usage
	var stat syscall.Statfs_t
	if err := syscall.Statfs(ms.serverDir, &stat); err == nil {
		total := float64(stat.Blocks) * float64(stat.Bsize) / (1024 * 1024 * 1024)
		free := float64(stat.Bavail) * float64(stat.Bsize) / (1024 * 1024 * 1024)
		used := total - free
		m.DiskTotalGB = round(total, 2)
		m.DiskFreeGB = round(free, 2)
		m.DiskUsedGB = round(used, 2)
		if total > 0 {
			m.DiskUsedPct = round((used/total)*100.0, 1)
		}
	}

	// 3. RCON TPS and Players
	if running && ms.rcon != nil {
		tpsResp, err := ms.rcon.Execute("tps")
		if err == nil && tpsResp != "" {
			matches := tpsRegex.FindAllStringSubmatch(tpsResp, -1)
			if len(matches) >= 3 {
				if v, err := strconv.ParseFloat(matches[0][1], 64); err == nil {
					m.TPS1m = min(v, 20.0)
				}
				if v, err := strconv.ParseFloat(matches[1][1], 64); err == nil {
					m.TPS5m = min(v, 20.0)
				}
				if v, err := strconv.ParseFloat(matches[2][1], 64); err == nil {
					m.TPS15m = min(v, 20.0)
				}
			}
		}

		listResp, err := ms.rcon.Execute("list")
		if err == nil && listResp != "" {
			// Pattern: "There are X of a max of Y players online:"
			re := regexp.MustCompile(`(?:There are\s+)?(\d+)\s+(?:of a max of|/)\s*(\d+)`)
			sub := re.FindStringSubmatch(listResp)
			if len(sub) >= 3 {
				m.OnlinePlayers, _ = strconv.Atoi(sub[1])
				m.MaxPlayers, _ = strconv.Atoi(sub[2])
			}
		}
		// 4. World Time Query (Purpur/Paper 1.21+ uses "time query day", vanilla fallback "time query daytime")
		timeResp, err := ms.rcon.Execute("time query day")
		if err != nil || strings.Contains(timeResp, "Can't find") || strings.Contains(timeResp, "Unknown") {
			timeResp, err = ms.rcon.Execute("time query daytime")
		}
		if err == nil && timeResp != "" {
			// Matches "Timeline minecraft:day is at 8686 tick(s)" or "The time is 8686"
			re := regexp.MustCompile(`(?:is\s+at\s+|The time is\s+|Time is\s+)(\d+)`)
			if sub := re.FindStringSubmatch(timeResp); len(sub) >= 2 {
				if t, err := strconv.Atoi(sub[1]); err == nil {
					ms.envMu.Lock()
					ms.currentTime = t % 24000
					ms.envMu.Unlock()
				}
			} else {
				// Fallback tick match: any digits followed by tick
				reFallback := regexp.MustCompile(`(\d+)\s*tick`)
				if subFallback := reFallback.FindStringSubmatch(timeResp); len(subFallback) >= 2 {
					if t, err := strconv.Atoi(subFallback[1]); err == nil {
						ms.envMu.Lock()
						ms.currentTime = t % 24000
						ms.envMu.Unlock()
					}
				}
			}
		}
	}

	if m.MaxPlayers == 0 && ms.propMgr != nil {
		if maxStr, ok := ms.propMgr.Get("max-players"); ok {
			m.MaxPlayers, _ = strconv.Atoi(maxStr)
		}
	}

	// World Environment Telemetry
	ms.envMu.RLock()
	curTime := ms.currentTime
	curWeather := ms.currentWeather
	ms.envMu.RUnlock()

	m.WorldTimeTicks = curTime
	m.WorldTimeString, m.WorldPhase = formatMinecraftTime(curTime)
	m.WorldWeather = curWeather

	// Record in history
	ms.historyMu.Lock()
	if len(ms.history) >= ms.maxHist {
		ms.history = ms.history[1:]
	}
	ms.history = append(ms.history, m)
	ms.historyMu.Unlock()

	return m
}

func (ms *MetricsService) GetHistory() []SystemMetrics {
	ms.historyMu.RLock()
	defer ms.historyMu.RUnlock()

	res := make([]SystemMetrics, len(ms.history))
	copy(res, ms.history)
	return res
}

func (ms *MetricsService) SetWorldTime(ticks int) (string, error) {
	ticks = ticks % 24000
	if ticks < 0 {
		ticks += 24000
	}
	ms.envMu.Lock()
	ms.currentTime = ticks
	ms.envMu.Unlock()

	if ms.rcon != nil {
		cmd := fmt.Sprintf("time set %d", ticks)
		resp, err := ms.rcon.Execute(cmd)
		if err != nil {
			return "", err
		}
		return resp, nil
	}
	return "", fmt.Errorf("RCON connection unavailable")
}

func (ms *MetricsService) SetWorldWeather(weather string) (string, error) {
	weather = strings.ToLower(strings.TrimSpace(weather))
	if weather != "clear" && weather != "rain" && weather != "thunder" {
		weather = "clear"
	}
	ms.envMu.Lock()
	ms.currentWeather = weather
	ms.envMu.Unlock()

	if ms.rcon != nil {
		cmd := fmt.Sprintf("weather %s", weather)
		resp, err := ms.rcon.Execute(cmd)
		if err != nil {
			return "", err
		}
		return resp, nil
	}
	return "", fmt.Errorf("RCON connection unavailable")
}

func (ms *MetricsService) GetWorldEnvironment() (ticks int, timeStr string, phase string, weather string) {
	ms.envMu.RLock()
	ticks = ms.currentTime
	weather = ms.currentWeather
	ms.envMu.RUnlock()

	timeStr, phase = formatMinecraftTime(ticks)
	return
}

// IngestLogLine parses live server stdout lines for in-game or console weather/time changes
func (ms *MetricsService) IngestLogLine(line string) {
	lower := strings.ToLower(line)
	if strings.Contains(lower, "set the weather to rain and thunder") || strings.Contains(lower, "set the weather to thunder") {
		ms.envMu.Lock()
		ms.currentWeather = "thunder"
		ms.envMu.Unlock()
	} else if strings.Contains(lower, "set the weather to rain") {
		ms.envMu.Lock()
		ms.currentWeather = "rain"
		ms.envMu.Unlock()
	} else if strings.Contains(lower, "set the weather to clear") {
		ms.envMu.Lock()
		ms.currentWeather = "clear"
		ms.envMu.Unlock()
	}

	// In-game or RCON /time set announcement
	reTime := regexp.MustCompile(`(?:Set\s+(?:the\s+time|minecraft:overworld)\s+to\s+)(\d+)`)
	if sub := reTime.FindStringSubmatch(line); len(sub) >= 2 {
		if t, err := strconv.Atoi(sub[1]); err == nil {
			ms.envMu.Lock()
			ms.currentTime = t % 24000
			ms.envMu.Unlock()
		}
	}
}

// initWeatherFromLog inspects the end of latest.log to recover last known weather state on startup
func (ms *MetricsService) initWeatherFromLog() {
	logPath := filepath.Join(ms.serverDir, "logs", "latest.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	start := 0
	if len(lines) > 500 {
		start = len(lines) - 500
	}
	for i := len(lines) - 1; i >= start; i-- {
		l := strings.ToLower(lines[i])
		if strings.Contains(l, "set the weather to rain and thunder") || strings.Contains(l, "set the weather to thunder") {
			ms.currentWeather = "thunder"
			break
		} else if strings.Contains(l, "set the weather to rain") {
			ms.currentWeather = "rain"
			break
		} else if strings.Contains(l, "set the weather to clear") {
			ms.currentWeather = "clear"
			break
		}
	}
}

func formatMinecraftTime(ticks int) (timeStr string, phase string) {
	ticks = ticks % 24000
	if ticks < 0 {
		ticks += 24000
	}
	hours := (ticks/1000 + 6) % 24
	mins := (ticks % 1000) * 60 / 1000
	timeStr = fmt.Sprintf("%02d:%02d", hours, mins)

	if ticks >= 23000 || ticks < 1000 {
		phase = "일출"
	} else if ticks < 6000 {
		phase = "아침/낮"
	} else if ticks < 7000 {
		phase = "정오"
	} else if ticks < 12000 {
		phase = "오후"
	} else if ticks < 13500 {
		phase = "일몰"
	} else if ticks < 17500 {
		phase = "밤"
	} else if ticks < 18500 {
		phase = "자정"
	} else {
		phase = "새벽"
	}
	return
}

func round(val float64, precision int) float64 {
	p := 1.0
	for i := 0; i < precision; i++ {
		p *= 10.0
	}
	return float64(int(val*p+0.5)) / p
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func cleanAnsi(b []byte) string {
	var buf bytes.Buffer
	inEscape := false
	for _, c := range b {
		if c == 0x1b {
			inEscape = true
			continue
		}
		if inEscape {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				inEscape = false
			}
			continue
		}
		buf.WriteByte(c)
	}
	return buf.String()
}
