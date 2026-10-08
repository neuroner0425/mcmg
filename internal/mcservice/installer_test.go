package mcservice

import (
	"testing"
)

func TestInstaller_APIs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	ins := NewInstaller()

	// 1. Fetch Versions
	versions, err := ins.FetchVersions()
	if err != nil {
		t.Fatalf("FetchVersions() error = %v", err)
	}
	if len(versions) == 0 {
		t.Fatal("expected at least one Purpur version")
	}

	// 2. Fetch Builds for latest version
	latestVersion := versions[len(versions)-1]
	builds, latestBuild, err := ins.FetchBuilds(latestVersion)
	if err != nil {
		t.Fatalf("FetchBuilds(%s) error = %v", latestVersion, err)
	}
	if len(builds) == 0 || latestBuild == "" {
		t.Fatalf("expected non-empty builds for %s", latestVersion)
	}

	// 3. Skip heavy 70MB download during standard testing to avoid timeouts
	t.Logf("Purpur %s has %d builds (latest: %s)", latestVersion, len(builds), latestBuild)
}
