package mcservice

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PluginInfo represents metadata for an installed plugin jar file.
type PluginInfo struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"mod_time"`
	SizeBytes string    `json:"size_formatted"`
	Enabled   bool      `json:"enabled"`
}

// PluginManager manages files inside the plugins/ directory.
type PluginManager struct {
	serverDir string
}

// NewPluginManager initializes a PluginManager for the server directory.
func NewPluginManager(serverDir string) *PluginManager {
	return &PluginManager{
		serverDir: serverDir,
	}
}

// pluginsDir returns the absolute path to the plugins directory.
func (pm *PluginManager) pluginsDir() string {
	return filepath.Join(pm.serverDir, "plugins")
}

// ensurePluginsDir ensures the plugins/ directory exists.
func (pm *PluginManager) ensurePluginsDir() error {
	return os.MkdirAll(pm.pluginsDir(), 0755)
}

// sanitizeFilename prevents directory traversal attacks and ensures .jar extension.
func sanitizeFilename(name string) (string, error) {
	if strings.Contains(name, "..") || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", errors.New("directory traversal characters not allowed")
	}
	base := filepath.Base(filepath.Clean(name))
	if base == "." || base == "" {
		return "", errors.New("invalid plugin file name")
	}
	if !strings.HasSuffix(strings.ToLower(base), ".jar") && !strings.HasSuffix(strings.ToLower(base), ".jar.disabled") {
		base += ".jar"
	}
	return base, nil
}

// ListPlugins returns a list of installed plugin jar files (both enabled and disabled).
func (pm *PluginManager) ListPlugins() ([]PluginInfo, error) {
	if err := pm.ensurePluginsDir(); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(pm.pluginsDir())
	if err != nil {
		return nil, fmt.Errorf("failed to read plugins directory: %w", err)
	}

	var plugins []PluginInfo
	for _, entry := range entries {
		name := entry.Name()
		lower := strings.ToLower(name)
		if entry.IsDir() {
			continue
		}
		isEnabled := strings.HasSuffix(lower, ".jar")
		isDisabled := strings.HasSuffix(lower, ".jar.disabled")
		if !isEnabled && !isDisabled {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		plugins = append(plugins, PluginInfo{
			Name:      name,
			Size:      info.Size(),
			ModTime:   info.ModTime(),
			SizeBytes: formatBytes(info.Size()),
			Enabled:   isEnabled,
		})
	}

	return plugins, nil
}

// TogglePlugin toggles a plugin between enabled (.jar) and disabled (.jar.disabled).
func (pm *PluginManager) TogglePlugin(fileName string) (string, error) {
	cleanName := filepath.Base(fileName)
	if strings.Contains(cleanName, "..") || strings.Contains(cleanName, "/") || strings.Contains(cleanName, "\\") {
		return "", errors.New("invalid plugin file name")
	}

	dir := pm.pluginsDir()
	curPath := filepath.Join(dir, cleanName)
	if _, err := os.Stat(curPath); err != nil {
		return "", fmt.Errorf("plugin file %s not found", cleanName)
	}

	var newName string
	if strings.HasSuffix(cleanName, ".jar.disabled") {
		newName = strings.TrimSuffix(cleanName, ".disabled")
	} else if strings.HasSuffix(cleanName, ".jar") {
		newName = cleanName + ".disabled"
	} else {
		return "", errors.New("file is not a valid plugin jar")
	}

	newPath := filepath.Join(dir, newName)
	if err := os.Rename(curPath, newPath); err != nil {
		return "", fmt.Errorf("failed to toggle plugin: %w", err)
	}

	return newName, nil
}

// SaveUploadedPlugin saves a plugin uploaded via multipart form.
func (pm *PluginManager) SaveUploadedPlugin(fileName string, r io.Reader) error {
	cleanName, err := sanitizeFilename(fileName)
	if err != nil {
		return err
	}

	if err := pm.ensurePluginsDir(); err != nil {
		return err
	}

	destPath := filepath.Join(pm.pluginsDir(), cleanName)
	dst, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create plugin file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, r); err != nil {
		return fmt.Errorf("failed to save plugin file: %w", err)
	}

	if strings.Contains(strings.ToLower(cleanName), "bluemap") {
		EnsureBlueMapConfig(pm.serverDir)
	}

	return nil
}

// DownloadPluginFromURL fetches a remote jar file and saves it into the plugins directory.
func (pm *PluginManager) DownloadPluginFromURL(rawURL, fileName string) error {
	cleanURL := strings.TrimSpace(rawURL)
	if cleanURL == "" {
		return errors.New("download URL cannot be empty")
	}

	// Infer filename from URL if not specified
	name := strings.TrimSpace(fileName)
	if name == "" {
		parts := strings.Split(cleanURL, "/")
		name = parts[len(parts)-1]
		if idx := strings.Index(name, "?"); idx != -1 {
			name = name[:idx]
		}
	}

	cleanName, err := sanitizeFilename(name)
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Get(cleanURL)
	if err != nil {
		return fmt.Errorf("failed to download plugin: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("remote server returned HTTP %d", resp.StatusCode)
	}

	return pm.SaveUploadedPlugin(cleanName, resp.Body)
}

// AutoInstallBlueMap delegates to AutoInstallSquaremap as squaremap is now the standard map engine.
func (pm *PluginManager) AutoInstallBlueMap() (string, error) {
	return pm.AutoInstallSquaremap("")
}

// DeletePlugin removes a plugin file safely.
func (pm *PluginManager) DeletePlugin(fileName string) error {
	cleanName, err := sanitizeFilename(fileName)
	if err != nil {
		return err
	}

	target := filepath.Join(pm.pluginsDir(), cleanName)
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return errors.New("plugin not found")
		}
		return fmt.Errorf("failed to delete plugin: %w", err)
	}

	return nil
}

// Helper: formatBytes converts byte count to human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// Helper: fileExists checks file existence.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
