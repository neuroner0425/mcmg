package mcservice

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PropertyEntry represents a line in server.properties (comment, empty line, or key-value pair).
type PropertyEntry struct {
	IsComment bool   `json:"is_comment"`
	Comment   string `json:"comment,omitempty"`
	Key       string `json:"key,omitempty"`
	Value     string `json:"value,omitempty"`
}

// PropertiesManager handles reading and writing server.properties files safely.
type PropertiesManager struct {
	filePath  string
	backupDir string
	mu        sync.RWMutex
}

// NewPropertiesManager initializes a manager for the specified properties file path and backup directory.
func NewPropertiesManager(filePath, backupDir string) *PropertiesManager {
	return &PropertiesManager{
		filePath:  filePath,
		backupDir: backupDir,
	}
}

// Read loads and parses the server.properties file preserving original order and comments.
func (pm *PropertiesManager) Read() ([]PropertyEntry, map[string]string, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	file, err := os.Open(pm.filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open properties file: %w", err)
	}
	defer file.Close()

	entries, keyMap, err := parseProperties(file)
	if err != nil {
		return nil, nil, err
	}

	return entries, keyMap, nil
}

// Get returns the value of a specific property key.
func (pm *PropertiesManager) Get(key string) (string, bool) {
	_, keyMap, err := pm.Read()
	if err != nil {
		return "", false
	}
	val, ok := keyMap[key]
	return val, ok
}

// Set updates and saves a specific property key.
func (pm *PropertiesManager) Set(key, val string) error {
	return pm.Save(map[string]string{key: val})
}

// parseProperties parses lines from an io.Reader.
func parseProperties(r io.Reader) ([]PropertyEntry, map[string]string, error) {
	var entries []PropertyEntry
	keyMap := make(map[string]string)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Handle comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			entries = append(entries, PropertyEntry{
				IsComment: true,
				Comment:   line,
			})
			continue
		}

		// Handle key=value
		idx := strings.Index(trimmed, "=")
		if idx == -1 {
			entries = append(entries, PropertyEntry{
				IsComment: true,
				Comment:   line,
			})
			continue
		}

		key := strings.TrimSpace(trimmed[:idx])
		val := strings.TrimSpace(trimmed[idx+1:])

		entries = append(entries, PropertyEntry{
			IsComment: false,
			Key:       key,
			Value:     val,
		})
		keyMap[key] = val
	}

	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("error reading properties file: %w", err)
	}

	return entries, keyMap, nil
}

// Save updates the server.properties file with the provided key-value updates.
// It creates a backup in backupDir before overwriting and writes atomically.
func (pm *PropertiesManager) Save(updates map[string]string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if updates == nil {
		return nil
	}

	// 1. Read existing file entries
	entries, _, err := func() ([]PropertyEntry, map[string]string, error) {
		file, err := os.Open(pm.filePath)
		if err != nil {
			return nil, nil, err
		}
		defer file.Close()
		return parseProperties(file)
	}()

	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read properties before save: %w", err)
	}

	// 2. Create backup in backupDir if existing file exists
	if !os.IsNotExist(err) {
		bDir := pm.backupDir
		if bDir == "" {
			bDir = filepath.Dir(pm.filePath)
		}
		_ = os.MkdirAll(bDir, 0755)

		backupFileName := fmt.Sprintf("server.properties.bak.%s", time.Now().Format("20060102150405"))
		backupPath := filepath.Join(bDir, backupFileName)
		if err := copyFile(pm.filePath, backupPath); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// 3. Merge updates into entries
	updatedKeys := make(map[string]bool)
	var newEntries []PropertyEntry

	for _, entry := range entries {
		if entry.IsComment {
			newEntries = append(newEntries, entry)
			continue
		}

		if newVal, exists := updates[entry.Key]; exists {
			newEntries = append(newEntries, PropertyEntry{
				IsComment: false,
				Key:       entry.Key,
				Value:     newVal,
			})
			updatedKeys[entry.Key] = true
		} else {
			newEntries = append(newEntries, entry)
			updatedKeys[entry.Key] = true
		}
	}

	// 4. Append any brand new keys
	for k, v := range updates {
		if !updatedKeys[k] {
			newEntries = append(newEntries, PropertyEntry{
				IsComment: false,
				Key:       k,
				Value:     v,
			})
		}
	}

	// 5. Write to temporary file in the target directory
	dir := filepath.Dir(pm.filePath)
	_ = os.MkdirAll(dir, 0755)

	tmpFile, err := os.CreateTemp(dir, "server_props_tmp_*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tmpName := tmpFile.Name()

	writer := bufio.NewWriter(tmpFile)
	for _, entry := range newEntries {
		if entry.IsComment {
			if _, err := writer.WriteString(entry.Comment + "\n"); err != nil {
				tmpFile.Close()
				_ = os.Remove(tmpName)
				return err
			}
		} else {
			line := fmt.Sprintf("%s=%s\n", entry.Key, entry.Value)
			if _, err := writer.WriteString(line); err != nil {
				tmpFile.Close()
				_ = os.Remove(tmpName)
				return err
			}
		}
	}

	if err := writer.Flush(); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}

	// 6. Rename atomic replace
	if err := os.Rename(tmpName, pm.filePath); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to replace properties file: %w", err)
	}

	return nil
}

// copyFile handles simple file copying for backups.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
