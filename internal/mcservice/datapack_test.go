package mcservice

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDatapackManager_ApplyExperiments(t *testing.T) {
	tempDir := t.TempDir()
	worldDir := filepath.Join(tempDir, "world")
	_ = os.MkdirAll(worldDir, 0755)

	// Copy real level.dat if exists
	realLevelDat := "../../server/world/level.dat"
	destLevelDat := filepath.Join(worldDir, "level.dat")
	if data, err := os.ReadFile(realLevelDat); err == nil {
		_ = os.WriteFile(destLevelDat, data, 0644)
	}

	// Write mock server.properties
	propsPath := filepath.Join(tempDir, "server.properties")
	_ = os.WriteFile(propsPath, []byte("initial-enabled-packs=vanilla\ninitial-disabled-packs=\n"), 0644)

	dm := NewDatapackManager(tempDir)
	experiments := []string{"trade_rebalance", "minecart_improvements", "redstone_experiments"}

	if err := dm.ApplyExperiments(experiments, nil); err != nil {
		t.Fatalf("ApplyExperiments failed: %v", err)
	}

	// Verify status
	status, err := dm.GetStatus(nil)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	t.Logf("Enabled packs: %v", status.EnabledPacks)
	t.Logf("Disabled packs: %v", status.DisabledPacks)

	enabledMap := make(map[string]bool)
	for _, p := range status.EnabledPacks {
		enabledMap[p] = true
	}

	for _, exp := range experiments {
		if !enabledMap[exp] {
			t.Errorf("Expected %s to be enabled", exp)
		}
	}
}

func TestApplyLiveExperiments(t *testing.T) {
	dm := NewDatapackManager("../../server")
	allExps := []string{"trade_rebalance", "minecart_improvements", "redstone_experiments"}
	if err := dm.ApplyExperiments(allExps, nil); err != nil {
		t.Fatalf("Live apply failed: %v", err)
	}
	t.Log("Successfully applied all experiments without bundle to server directory!")
}
