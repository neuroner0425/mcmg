package mcservice

import (
	"context"
	"testing"
	"time"
)

func TestUpdater_CheckUpdate(t *testing.T) {
	updater := NewUpdater(".")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := updater.CheckUpdate(ctx)
	if err != nil {
		t.Fatalf("CheckUpdate failed: %v", err)
	}

	if info.CurrentCommit == "" {
		t.Errorf("expected non-empty CurrentCommit")
	}

	if info.Branch == "" {
		t.Errorf("expected non-empty Branch")
	}
}
