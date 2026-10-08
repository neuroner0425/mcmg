package mcservice

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestPurpurConfigManager_PreservesSliceTypes(t *testing.T) {
	tmpDir := t.TempDir()
	confPath := filepath.Join(tmpDir, "purpur.yml")
	bakDir := filepath.Join(tmpDir, "backups")

	// Sample initial purpur.yml with various YAML data structures
	initialYAML := `config-version: 49
settings:
  blast-resistance-overrides: {}
world-settings:
  default:
    gameplay-mechanics:
      item:
        immune:
          cactus: []
          explosion: []
        tools:
        - minecraft:iron_pickaxe
        - minecraft:diamond_pickaxe
      player:
        teleport-if-outside-border: false
`
	if err := os.WriteFile(confPath, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("Failed to write initial purpur.yml: %v", err)
	}

	mgr := &PurpurConfigManager{
		filePath:  confPath,
		backupDir: bakDir,
	}

	// Read initial
	_, flat, err := mgr.Read()
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Verify slice flattening
	if flat["world-settings.default.gameplay-mechanics.item.immune.cactus"] != "[]" {
		t.Errorf("Expected '[]', got '%s'", flat["world-settings.default.gameplay-mechanics.item.immune.cactus"])
	}

	// Perform a save that updates a boolean AND touches the slice with "[]"
	updates := map[string]string{
		"world-settings.default.gameplay-mechanics.player.teleport-if-outside-border": "true",
		"world-settings.default.gameplay-mechanics.item.immune.cactus":                 "[]",
	}

	if err := mgr.Save(updates); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read the raw saved YAML file
	savedBytes, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("Failed to read saved YAML: %v", err)
	}
	savedStr := string(savedBytes)

	// CRITICAL: Ensure 'cactus: []' did NOT become quoted 'cactus: \'[]\''
	if strings.Contains(savedStr, "cactus: '[]'") || strings.Contains(savedStr, "cactus: \"[]\"") {
		t.Errorf("Corrupted YAML: cactus was marshaled as quoted string:\n%s", savedStr)
	}

	// Parse back into generic interface{} to check exact Java-compatible types
	var checkRoot map[string]interface{}
	if err := yaml.Unmarshal(savedBytes, &checkRoot); err != nil {
		t.Fatalf("Failed to unmarshal saved YAML: %v", err)
	}

	// Drill down to cactus
	ws, _ := checkRoot["world-settings"].(map[string]interface{})
	def, _ := ws["default"].(map[string]interface{})
	gm, _ := def["gameplay-mechanics"].(map[string]interface{})
	item, _ := gm["item"].(map[string]interface{})
	immune, _ := item["immune"].(map[string]interface{})
	cactusVal := immune["cactus"]

	if _, isSlice := cactusVal.([]interface{}); !isSlice {
		t.Fatalf("CRITICAL: cactus is %T (%v), expected []interface{}", cactusVal, cactusVal)
	}
}

func TestParseSliceValue(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"[]", 0},
		{"[item1, item2]", 2},
		{"item1, item2, item3", 3},
		{"[minecraft:binding_curse minecraft:vanishing_curse]", 2},
	}

	for _, tc := range tests {
		res := parseSliceValue(tc.input)
		if len(res) != tc.expected {
			t.Errorf("parseSliceValue(%q) returned %d items, expected %d", tc.input, len(res), tc.expected)
		}
	}
}
