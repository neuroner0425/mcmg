package mcservice

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPluginManager(t *testing.T) {
	tmpDir := t.TempDir()
	pm := NewPluginManager(tmpDir)

	// 1. Initial list should be empty
	list, err := pm.ListPlugins()
	if err != nil {
		t.Fatalf("ListPlugins failed: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected 0 plugins, got %d", len(list))
	}

	// 2. Upload dummy plugin
	dummyContent := []byte("PK\x03\x04dummy jar content")
	if err := pm.SaveUploadedPlugin("BlueMap.jar", bytes.NewReader(dummyContent)); err != nil {
		t.Fatalf("SaveUploadedPlugin failed: %v", err)
	}

	// 3. Verify in list
	list, err = pm.ListPlugins()
	if err != nil {
		t.Fatalf("ListPlugins failed: %v", err)
	}
	if len(list) != 1 || list[0].Name != "BlueMap.jar" {
		t.Errorf("expected BlueMap.jar in list, got %+v", list)
	}

	// 4. Test path traversal attempt
	err = pm.SaveUploadedPlugin("../evil.jar", bytes.NewReader(dummyContent))
	if err != nil {
		// Should sanitize or error
	}
	// Verify it stayed within plugins/
	if _, err := os.Stat(filepath.Join(tmpDir, "evil.jar")); !os.IsNotExist(err) {
		t.Errorf("path traversal vulnerability: evil.jar created outside plugins")
	}

	// 5. Delete plugin
	if err := pm.DeletePlugin("BlueMap.jar"); err != nil {
		t.Fatalf("DeletePlugin failed: %v", err)
	}

	list, _ = pm.ListPlugins()
	if len(list) != 0 {
		t.Errorf("expected 0 plugins after delete, got %d", len(list))
	}
}
