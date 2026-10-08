package mcservice

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindSquaremapURL(t *testing.T) {
	// Test MC 26.3
	url26, file26, err26 := FindSquaremapURL("26.3")
	if err26 != nil {
		t.Fatalf("FindSquaremapURL(26.3) failed: %v", err26)
	}
	if !strings.Contains(url26, "squaremap") {
		t.Errorf("Expected squaremap in URL, got: %s", url26)
	}
	if file26 == "" {
		t.Errorf("Expected valid filename, got empty")
	}

	// Test MC 1.21.4
	url21, _, err21 := FindSquaremapURL("1.21.4")
	if err21 != nil {
		t.Fatalf("FindSquaremapURL(1.21.4) failed: %v", err21)
	}
	if !strings.Contains(url21, "squaremap") {
		t.Errorf("Expected squaremap in URL, got: %s", url21)
	}
}

func TestEnsureSquaremapConfig(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sm_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	if err := EnsureSquaremapConfig(tempDir, 8100); err != nil {
		t.Fatalf("EnsureSquaremapConfig failed: %v", err)
	}

	cfgPath := filepath.Join(tempDir, "plugins", "squaremap", "config.yml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("config.yml not found: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "port: 8100") {
		t.Errorf("Expected port 8100 in config.yml, got:\n%s", content)
	}
	if !strings.Contains(content, "enabled: true") {
		t.Errorf("Expected enabled: true in config.yml, got:\n%s", content)
	}
}

func TestAutoInstallSquaremapLive(t *testing.T) {
	pm := NewPluginManager("../../server")
	file, err := pm.AutoInstallSquaremap("")
	if err != nil {
		t.Fatalf("AutoInstallSquaremap failed: %v", err)
	}
	t.Logf("Installed squaremap jar: %s", file)
}

