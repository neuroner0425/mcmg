package mcservice

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteLevelDat(t *testing.T) {
	levelDatPath := "../../server/world/level.dat"
	if _, err := os.Stat(levelDatPath); err != nil {
		t.Skip("level.dat not found, skipping")
	}

	root, err := ReadLevelDat(levelDatPath)
	if err != nil {
		t.Fatalf("ReadLevelDat failed: %v", err)
	}

	if root.Type != TagCompound {
		t.Fatalf("Expected root to be TagCompound, got %d", root.Type)
	}

	dataTag := root.FindChild("Data")
	if dataTag == nil {
		t.Fatal("Data tag not found in root")
	}

	dataPacks := dataTag.FindChild("DataPacks")
	if dataPacks == nil {
		t.Fatal("DataPacks tag not found in Data")
	}

	enabled := dataPacks.FindChild("Enabled")
	if enabled == nil {
		t.Fatal("Enabled list not found in DataPacks")
	}
	t.Logf("Enabled count: %d", len(enabled.ListItems))
	for _, item := range enabled.ListItems {
		t.Logf(" - %s", item.Value.(string))
	}

	disabled := dataPacks.FindChild("Disabled")
	if disabled != nil {
		t.Logf("Disabled count: %d", len(disabled.ListItems))
		for _, item := range disabled.ListItems {
			t.Logf(" - %s", item.Value.(string))
		}
	}

	// Round-trip test
	tmpFile := filepath.Join(t.TempDir(), "level.dat")
	if err := WriteLevelDat(tmpFile, root); err != nil {
		t.Fatalf("WriteLevelDat failed: %v", err)
	}

	root2, err := ReadLevelDat(tmpFile)
	if err != nil {
		t.Fatalf("ReadLevelDat on written file failed: %v", err)
	}

	dataPacks2 := root2.FindChild("Data").FindChild("DataPacks")
	enabled2 := dataPacks2.FindChild("Enabled")
	if len(enabled2.ListItems) != len(enabled.ListItems) {
		t.Errorf("Roundtrip mismatch on Enabled items count")
	}
}
