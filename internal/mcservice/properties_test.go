package mcservice

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPropertiesManager_ReadAndSave(t *testing.T) {
	tmpDir := t.TempDir()
	propPath := filepath.Join(tmpDir, "server.properties")

	initialContent := `# Minecraft server properties
# Mon Oct 07 2026
difficulty=easy
gamemode=survival
max-players=10
pvp=true
`
	if err := os.WriteFile(propPath, []byte(initialContent), 0644); err != nil {
		t.Fatalf("failed to setup initial properties file: %v", err)
	}

	backupDir := filepath.Join(tmpDir, "backups")
	pm := NewPropertiesManager(propPath, backupDir)

	// Test Read
	entries, keyMap, err := pm.Read()
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	if keyMap["difficulty"] != "easy" {
		t.Errorf("expected difficulty=easy, got %s", keyMap["difficulty"])
	}
	if keyMap["max-players"] != "10" {
		t.Errorf("expected max-players=10, got %s", keyMap["max-players"])
	}

	// Test Save (Update difficulty, max-players, add new-key)
	updates := map[string]string{
		"difficulty":  "hard",
		"max-players": "25",
		"view-distance": "16",
	}

	if err := pm.Save(updates); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	// Read again and verify
	entriesAfter, keyMapAfter, err := pm.Read()
	if err != nil {
		t.Fatalf("Read() after save failed: %v", err)
	}

	if keyMapAfter["difficulty"] != "hard" {
		t.Errorf("expected difficulty=hard, got %s", keyMapAfter["difficulty"])
	}
	if keyMapAfter["max-players"] != "25" {
		t.Errorf("expected max-players=25, got %s", keyMapAfter["max-players"])
	}
	if keyMapAfter["view-distance"] != "16" {
		t.Errorf("expected view-distance=16, got %s", keyMapAfter["view-distance"])
	}
	if keyMapAfter["gamemode"] != "survival" {
		t.Errorf("expected gamemode=survival preserved, got %s", keyMapAfter["gamemode"])
	}

	// Check if comments were preserved
	foundComment := false
	for _, entry := range entriesAfter {
		if entry.IsComment && entry.Comment == "# Minecraft server properties" {
			foundComment = true
			break
		}
	}
	if !foundComment {
		t.Errorf("comments were not preserved")
	}

	_ = entries
}
