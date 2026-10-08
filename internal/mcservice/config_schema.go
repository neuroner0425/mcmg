package mcservice

import (
	"fmt"
	"strconv"
	"strings"
)

// SettingType defines the input UI type.
type SettingType string

const (
	TypeBool   SettingType = "bool"
	TypeNumber SettingType = "number"
	TypeSelect SettingType = "select"
	TypeString SettingType = "string"
)

// SettingSchema defines metadata for a configurable server option.
type SettingSchema struct {
	Source      string      `json:"source"`      // "properties" or "purpur"
	Key         string      `json:"key"`         // e.g. "difficulty" or "mobs.creeper.ridable"
	Type        SettingType `json:"type"`        // bool, number, select, string
	LabelKR     string      `json:"label_kr"`    // 한국어 명칭
	Description string      `json:"description"` // 상세 한국어 설명
	Category    string      `json:"category"`    // 카테고리
	Options     []string    `json:"options,omitempty"`
	CurrentVal  string      `json:"current_val"`
	DefaultVal  string      `json:"default_val,omitempty"`
}

// BuildFullConfigSchema merges the predefined registry with all discovered keys from server.properties and purpur.yml,
// guaranteeing that 100% of all settings are present without any missing items.
func BuildFullConfigSchema(propMap map[string]string, purpurMap map[string]string) []SettingSchema {
	var result []SettingSchema
	seenProps := make(map[string]bool)
	seenPurpur := make(map[string]bool)

	// 1. Load known schema with rich Korean descriptions and defaults
	for _, s := range ConfigSchemaRegistry {
		clone := s
		if def, ok := StandardDefaults[s.Key]; ok {
			clone.DefaultVal = def
		} else if s.Type == TypeBool {
			clone.DefaultVal = "false"
		}

		if s.Source == "properties" {
			seenProps[s.Key] = true
			if val, ok := propMap[s.Key]; ok {
				clone.CurrentVal = val
			} else if clone.DefaultVal != "" {
				clone.CurrentVal = clone.DefaultVal
			}
		} else if s.Source == "purpur" {
			// Purpur keys might be prefixed with "world-settings.default." in yaml
			seenPurpur[s.Key] = true
			if val, ok := purpurMap[s.Key]; ok {
				clone.CurrentVal = val
			} else if val, ok := purpurMap["world-settings.default."+s.Key]; ok {
				clone.CurrentVal = val
			} else if clone.DefaultVal != "" {
				clone.CurrentVal = clone.DefaultVal
			}
		}
		result = append(result, clone)
	}

	// 2. Discover remaining keys in server.properties
	for k, v := range propMap {
		if !seenProps[k] {
			typ := inferSettingType(v)
			defVal := v
			if def, ok := StandardDefaults[k]; ok {
				defVal = def
			}
			result = append(result, SettingSchema{
				Source:      "properties",
				Key:         k,
				Type:        typ,
				LabelKR:     formatAutoKeyLabel(k),
				Description: fmt.Sprintf("server.properties의 %s 항목입니다.", k),
				Category:    "기타 서버 설정",
				CurrentVal:  v,
				DefaultVal:  defVal,
			})
			seenProps[k] = true
		}
	}

	// 3. Discover remaining keys in purpur.yml
	for k, v := range purpurMap {
		shortKey := strings.TrimPrefix(k, "world-settings.default.")
		if !seenPurpur[shortKey] && !seenPurpur[k] {
			typ := inferSettingType(v)
			cat := categorizePurpurKey(shortKey)
			defVal := v
			if def, ok := StandardDefaults[shortKey]; ok {
				defVal = def
			} else if def, ok := StandardDefaults[k]; ok {
				defVal = def
			}
			result = append(result, SettingSchema{
				Source:      "purpur",
				Key:         shortKey,
				Type:        typ,
				LabelKR:     formatAutoKeyLabel(shortKey),
				Description: fmt.Sprintf("Purpur의 %s 항목입니다.", shortKey),
				Category:    cat,
				CurrentVal:  v,
				DefaultVal:  defVal,
			})
			seenPurpur[shortKey] = true
		}
	}

	return result
}

func inferSettingType(val string) SettingType {
	lower := strings.ToLower(strings.TrimSpace(val))
	if lower == "true" || lower == "false" {
		return TypeBool
	}
	if _, err := strconv.ParseFloat(val, 64); err == nil && !strings.Contains(val, ":") {
		return TypeNumber
	}
	return TypeString
}

func formatAutoKeyLabel(key string) string {
	parts := strings.Split(key, ".")
	last := parts[len(parts)-1]
	words := strings.Split(strings.ReplaceAll(last, "-", " "), "_")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func categorizePurpurKey(key string) string {
	if strings.HasPrefix(key, "mobs.") {
		return "Purpur 몹 상세 설정"
	}
	if strings.HasPrefix(key, "gameplay-mechanics.") {
		return "Purpur 게임플레이"
	}
	if strings.HasPrefix(key, "blocks.") {
		return "Purpur 블록 설정"
	}
	if strings.HasPrefix(key, "tools.") {
		return "Purpur 도구 설정"
	}
	if strings.HasPrefix(key, "settings.network.") {
		return "Purpur 네트워크"
	}
	if strings.HasPrefix(key, "settings.messages.") {
		return "Purpur 메시지"
	}
	return "Purpur 기타 설정"
}

