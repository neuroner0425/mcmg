package mcservice

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ModrinthVersion represents a version entry returned from Modrinth API.
type ModrinthVersion struct {
	VersionNumber string   `json:"version_number"`
	GameVersions  []string `json:"game_versions"`
	Loaders       []string `json:"loaders"`
	Files         []struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		Primary  bool   `json:"primary"`
	} `json:"files"`
}

// DetectServerVersion attempts to read installed version or server logs to identify Minecraft version.
func DetectServerVersion(serverDir string) string {
	// 1. Try reading installed_version.json
	infoPath := filepath.Join(serverDir, "installed_version.json")
	if data, err := os.ReadFile(infoPath); err == nil {
		var info struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(data, &info); err == nil && info.Version != "" && info.Version != "설치됨" {
			return strings.TrimSpace(info.Version)
		}
	}

	// 2. Try scanning latest.log
	logPath := filepath.Join(serverDir, "logs", "latest.log")
	if f, err := os.Open(logPath); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		reVer := regexp.MustCompile(`(?:Starting minecraft server version|Loading Purpur)\s+([0-9.]+)`)
		lineCount := 0
		for scanner.Scan() && lineCount < 100 {
			line := scanner.Text()
			lineCount++
			if m := reVer.FindStringSubmatch(line); len(m) > 1 {
				return m[1]
			}
		}
	}

	// 3. Fallback default
	return "26.3"
}

// extractPrimaryFile retrieves primary or first file URL and filename.
func extractPrimaryFile(v ModrinthVersion) (string, string) {
	if len(v.Files) == 0 {
		return "", ""
	}
	for _, f := range v.Files {
		if f.Primary && f.URL != "" {
			return f.URL, f.Filename
		}
	}
	return v.Files[0].URL, v.Files[0].Filename
}

// FindSquaremapURL queries Modrinth API to find the most compatible squaremap build for mcVersion.
func FindSquaremapURL(mcVersion string) (string, string, error) {
	if mcVersion == "" {
		mcVersion = "26.3"
	}

	client := &http.Client{Timeout: 15 * time.Second}
	loadersQuery := url.QueryEscape(`["paper"]`)

	// 1. Query by exact game_version filter
	exactQuery := url.QueryEscape(fmt.Sprintf(`["%s"]`, mcVersion))
	apiURL := fmt.Sprintf("https://api.modrinth.com/v2/project/squaremap/version?game_versions=%s&loaders=%s", exactQuery, loadersQuery)
	if req, err := http.NewRequest(http.MethodGet, apiURL, nil); err == nil {
		req.Header.Set("User-Agent", "MinecraftServerManager/1.0 (local-manager)")
		if resp, err := client.Do(req); err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var list []ModrinthVersion
			if err := json.NewDecoder(resp.Body).Decode(&list); err == nil && len(list) > 0 {
				if fileURL, name := extractPrimaryFile(list[0]); fileURL != "" {
					return fileURL, name, nil
				}
			}
		}
	}

	// 2. Query all paper versions and find closest matching semver
	allURL := "https://api.modrinth.com/v2/project/squaremap/version?loaders=" + loadersQuery
	if req, err := http.NewRequest(http.MethodGet, allURL, nil); err == nil {
		req.Header.Set("User-Agent", "MinecraftServerManager/1.0 (local-manager)")
		if resp, err := client.Do(req); err == nil && resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var list []ModrinthVersion
			if err := json.NewDecoder(resp.Body).Decode(&list); err == nil && len(list) > 0 {
				// Exact match inside version array
				for _, v := range list {
					for _, gv := range v.GameVersions {
						if gv == mcVersion || strings.HasPrefix(mcVersion, gv) || strings.HasPrefix(gv, mcVersion) {
							if fileURL, name := extractPrimaryFile(v); fileURL != "" {
								return fileURL, name, nil
							}
						}
					}
				}

				// Prefix match (e.g. 1.21.x -> 1.21)
				majorPrefix := mcVersion
				if parts := strings.Split(mcVersion, "."); len(parts) >= 2 {
					majorPrefix = parts[0] + "." + parts[1]
				}
				for _, v := range list {
					for _, gv := range v.GameVersions {
						if strings.HasPrefix(gv, majorPrefix) {
							if fileURL, name := extractPrimaryFile(v); fileURL != "" {
								return fileURL, name, nil
							}
						}
					}
				}

				// Fallback to latest available paper version
				if fileURL, name := extractPrimaryFile(list[0]); fileURL != "" {
					return fileURL, name, nil
				}
			}
		}
	}

	// 3. Static CDN Fallback
	staticMap := map[string]string{
		"26.3":    "https://cdn.modrinth.com/data/PFb7ZqK6/versions/Bztlrcv0/squaremap-paper-mc26.3-1.4.0.jar",
		"26.2":    "https://cdn.modrinth.com/data/PFb7ZqK6/versions/ejPk2ZiR/squaremap-paper-mc26.2-1.3.15.jar",
		"26.1.2":  "https://cdn.modrinth.com/data/PFb7ZqK6/versions/mK9vPyoY/squaremap-paper-mc26.1.2-1.3.13.2.jar",
		"1.21.11": "https://cdn.modrinth.com/data/PFb7ZqK6/versions/GItyEkou/squaremap-paper-mc1.21.11-1.3.12.jar",
		"1.21.4":  "https://cdn.modrinth.com/data/PFb7ZqK6/versions/DB47ULQI/squaremap-paper-mc1.21.4-1.3.4.jar",
		"1.21.1":  "https://cdn.modrinth.com/data/PFb7ZqK6/versions/qLpGqU5n/squaremap-paper-mc1.21.1-1.3.3.jar",
	}

	if u, ok := staticMap[mcVersion]; ok {
		return u, "squaremap.jar", nil
	}

	return staticMap["26.3"], "squaremap.jar", nil
}

// EnsureSquaremapConfig ensures internal-webserver port (8100) and bind settings in squaremap's config.yml
// using the robust yaml.v3 parser to eliminate syntax/indentation errors.
func EnsureSquaremapConfig(serverDir string, port int) error {
	if port <= 0 {
		port = 8100
	}
	smDir := filepath.Join(serverDir, "plugins", "squaremap")
	if err := os.MkdirAll(smDir, 0755); err != nil {
		return err
	}

	configPath := filepath.Join(smDir, "config.yml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Create clean default config
		template := fmt.Sprintf(`# squaremap configuration auto-generated by Sanctum Manager
settings:
  internal-webserver:
    enabled: true
    bind: 0.0.0.0
    port: %d
`, port)
		return os.WriteFile(configPath, []byte(template), 0644)
	}

	var rawMap map[string]interface{}
	if err := yaml.Unmarshal(data, &rawMap); err != nil || rawMap == nil {
		// Existing file had syntax error; recover with clean template
		template := fmt.Sprintf(`# squaremap configuration auto-generated by Sanctum Manager
settings:
  internal-webserver:
    enabled: true
    bind: 0.0.0.0
    port: %d
`, port)
		return os.WriteFile(configPath, []byte(template), 0644)
	}

	settings, ok := rawMap["settings"].(map[string]interface{})
	if !ok || settings == nil {
		settings = make(map[string]interface{})
		rawMap["settings"] = settings
	}
	webserver, ok := settings["internal-webserver"].(map[string]interface{})
	if !ok || webserver == nil {
		webserver = make(map[string]interface{})
		settings["internal-webserver"] = webserver
	}
	webserver["enabled"] = true
	webserver["port"] = port
	if _, hasBind := webserver["bind"]; !hasBind {
		webserver["bind"] = "0.0.0.0"
	}

	out, err := yaml.Marshal(rawMap)
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, out, 0644)
}

// AutoInstallSquaremap discovers compatible squaremap version and downloads/configures it.
func (pm *PluginManager) AutoInstallSquaremap(mcVersion string) (string, error) {
	if mcVersion == "" {
		mcVersion = DetectServerVersion(pm.serverDir)
	}

	downloadURL, _, err := FindSquaremapURL(mcVersion)
	if err != nil {
		return "", fmt.Errorf("failed to discover squaremap for version %s: %w", mcVersion, err)
	}

	// Disable or delete conflicting BlueMap jar if present
	_ = os.Remove(filepath.Join(pm.pluginsDir(), "BlueMap.jar"))
	_ = os.Remove(filepath.Join(pm.pluginsDir(), "BlueMap.jar.disabled"))

	targetJar := "squaremap.jar"
	if err := pm.DownloadPluginFromURL(downloadURL, targetJar); err != nil {
		return "", fmt.Errorf("failed to download squaremap jar: %w", err)
	}

	// Ensure port 8100 configuration
	_ = EnsureSquaremapConfig(pm.serverDir, 8100)

	return targetJar, nil
}
