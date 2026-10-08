package mcservice

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// PurpurConfigManager handles reading and writing purpur.yml.
type PurpurConfigManager struct {
	filePath  string
	backupDir string
	mu        sync.RWMutex
}

// NewPurpurConfigManager initializes a new PurpurConfigManager.
func NewPurpurConfigManager(serverDir, backupDir string) *PurpurConfigManager {
	return &PurpurConfigManager{
		filePath:  filepath.Join(serverDir, "purpur.yml"),
		backupDir: backupDir,
	}
}

// Read loads purpur.yml and returns nested map and flat map.
func (pcm *PurpurConfigManager) Read() (map[string]interface{}, map[string]string, error) {
	pcm.mu.RLock()
	defer pcm.mu.RUnlock()

	data, err := os.ReadFile(pcm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]interface{}), make(map[string]string), nil
		}
		return nil, nil, fmt.Errorf("failed to read purpur.yml: %w", err)
	}

	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, nil, fmt.Errorf("failed to parse purpur.yml: %w", err)
	}

	flatMap := make(map[string]string)
	flattenYAML("", root, flatMap)

	return root, flatMap, nil
}

func flattenYAML(prefix string, node interface{}, result map[string]string) {
	switch v := node.(type) {
	case map[string]interface{}:
		for k, val := range v {
			newKey := k
			if prefix != "" {
				newKey = prefix + "." + k
			}
			flattenYAML(newKey, val, result)
		}
	case []interface{}:
		if len(v) == 0 {
			result[prefix] = "[]"
		} else {
			var sb strings.Builder
			sb.WriteString("[")
			for i, item := range v {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(fmt.Sprintf("%v", item))
			}
			sb.WriteString("]")
			result[prefix] = sb.String()
		}
	default:
		result[prefix] = fmt.Sprintf("%v", v)
	}
}

// Save updates specific dot-notated keys in purpur.yml safely while preserving existing YAML types (lists/maps).
func (pcm *PurpurConfigManager) Save(updates map[string]string) error {
	pcm.mu.Lock()
	defer pcm.mu.Unlock()

	if updates == nil || len(updates) == 0 {
		return nil
	}

	data, err := os.ReadFile(pcm.filePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read purpur.yml before save: %w", err)
	}

	var root map[string]interface{}
	if len(data) > 0 {
		if err := yaml.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("failed to parse existing purpur.yml: %w", err)
		}
	}
	if root == nil {
		root = make(map[string]interface{})
	}

	// Backup
	if len(data) > 0 {
		_ = os.MkdirAll(pcm.backupDir, 0755)
		bakPath := filepath.Join(pcm.backupDir, fmt.Sprintf("purpur.yml.bak.%s", time.Now().Format("20060102150405")))
		_ = os.WriteFile(bakPath, data, 0644)
	}

	// Apply updates
	for dotKey, strVal := range updates {
		// If key doesn't start with settings. and doesn't start with world-settings.,
		// check if it's a world-setting by default in purpur.yml
		targetKey := dotKey
		if !strings.HasPrefix(targetKey, "settings.") && !strings.HasPrefix(targetKey, "world-settings.") && !strings.HasPrefix(targetKey, "verbose") {
			targetKey = "world-settings.default." + targetKey
		}
		parts := strings.Split(targetKey, ".")
		setNestedValue(root, parts, strVal)
	}

	outBytes, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("failed to marshal purpur.yml: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(pcm.filePath), "purpur_tmp_*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write(outBytes); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}

	return os.Rename(tmpName, pcm.filePath)
}

func setNestedValue(m map[string]interface{}, keys []string, strVal string) {
	if len(keys) == 0 {
		return
	}
	if len(keys) == 1 {
		k := keys[0]
		existing, ok := m[k]
		if ok {
			// If existing value is a YAML list/slice, preserve slice type!
			if _, isSlice := existing.([]interface{}); isSlice {
				m[k] = parseSliceValue(strVal)
				return
			}
			// If existing value is an empty or populated map, preserve map
			if _, isMap := existing.(map[string]interface{}); isMap {
				trimmed := strings.TrimSpace(strVal)
				if trimmed == "{}" || trimmed == "" {
					m[k] = make(map[string]interface{})
					return
				}
			}
		}
		m[k] = parseTypedValue(strVal)
		return
	}

	sub, ok := m[keys[0]].(map[string]interface{})
	if !ok || sub == nil {
		sub = make(map[string]interface{})
		m[keys[0]] = sub
	}
	setNestedValue(sub, keys[1:], strVal)
}

func parseSliceValue(s string) []interface{} {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || trimmed == "[]" {
		return []interface{}{}
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	if trimmed == "" {
		return []interface{}{}
	}

	var parts []string
	if strings.Contains(trimmed, ",") {
		parts = strings.Split(trimmed, ",")
	} else {
		parts = strings.Fields(trimmed)
	}

	res := make([]interface{}, 0, len(parts))
	for _, p := range parts {
		clean := strings.TrimSpace(p)
		clean = strings.Trim(clean, "\"'")
		if clean != "" {
			res = append(res, parseTypedValue(clean))
		}
	}
	return res
}

func parseTypedValue(s string) interface{} {
	trimmed := strings.TrimSpace(s)
	if trimmed == "[]" {
		return []interface{}{}
	}
	if trimmed == "{}" {
		return make(map[string]interface{})
	}
	lower := strings.ToLower(trimmed)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}
	if num, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return num
	}
	if flt, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return flt
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		return parseSliceValue(trimmed)
	}
	return trimmed
}
