package mcservice

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// UpdateInfo holds details about the system version and pending updates.
type UpdateInfo struct {
	HasUpdate      bool     `json:"has_update"`
	CurrentCommit  string   `json:"current_commit"`
	CurrentMessage string   `json:"current_message"`
	LatestCommit   string   `json:"latest_commit"`
	LatestMessage  string   `json:"latest_message"`
	PendingCommits []string `json:"pending_commits"`
	Branch         string   `json:"branch"`
	CheckedAt      string   `json:"checked_at"`
}

// UpdateProgress represents the status during update application.
type UpdateProgress struct {
	Step    string `json:"step"`
	Status  string `json:"status"` // "in_progress", "success", "error"
	Message string `json:"message"`
}

// Updater handles git checking, pulling, binary rebuilding, and zero-downtime self-restarting.
type Updater struct {
	repoDir    string
	mu         sync.Mutex
	isUpdating bool
	lastInfo   *UpdateInfo
}

// NewUpdater creates a new system updater.
func NewUpdater(repoDir string) *Updater {
	if repoDir == "" {
		repoDir = "."
	}
	return &Updater{
		repoDir: repoDir,
	}
}

// CheckUpdate checks the remote git repository for available updates without modifying files.
func (u *Updater) CheckUpdate(ctx context.Context) (*UpdateInfo, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	// 1. Get current branch
	branchOut, err := exec.CommandContext(ctx, "git", "-C", u.repoDir, "rev-parse", "--abbrev-ref", "HEAD").Output()
	branch := "main"
	if err == nil {
		branch = strings.TrimSpace(string(branchOut))
	}

	// 2. Get local HEAD commit hash & message
	headHashOut, err := exec.CommandContext(ctx, "git", "-C", u.repoDir, "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get local git commit: %w", err)
	}
	headHash := strings.TrimSpace(string(headHashOut))

	headMsgOut, _ := exec.CommandContext(ctx, "git", "-C", u.repoDir, "log", "-1", "--format=%s", "HEAD").Output()
	headMsg := strings.TrimSpace(string(headMsgOut))

	info := &UpdateInfo{
		CurrentCommit:  headHash,
		CurrentMessage: headMsg,
		LatestCommit:   headHash,
		LatestMessage:  headMsg,
		Branch:         branch,
		CheckedAt:      time.Now().Format("2006-01-02 15:04:05"),
		HasUpdate:      false,
		PendingCommits: []string{},
	}

	// 3. Fetch from remote (with 8-second timeout)
	fetchCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	_ = exec.CommandContext(fetchCtx, "git", "-C", u.repoDir, "fetch", "origin", branch).Run()

	// 4. Resolve upstream reference (try @{u} first, then origin/<branch>)
	upstreamRef := "@{u}"
	remoteHashOut, err := exec.CommandContext(ctx, "git", "-C", u.repoDir, "rev-parse", "--short", upstreamRef).Output()
	if err != nil {
		upstreamRef = fmt.Sprintf("origin/%s", branch)
		remoteHashOut, err = exec.CommandContext(ctx, "git", "-C", u.repoDir, "rev-parse", "--short", upstreamRef).Output()
	}

	if err == nil {
		remoteHash := strings.TrimSpace(string(remoteHashOut))
		if remoteHash != "" && remoteHash != headHash {
			// Find commits between HEAD and upstream
			logRange := fmt.Sprintf("HEAD..%s", upstreamRef)
			logOut, err := exec.CommandContext(ctx, "git", "-C", u.repoDir, "log", "--oneline", logRange).Output()
			if err == nil {
				rawLines := strings.Split(strings.TrimSpace(string(logOut)), "\n")
				var commits []string
				for _, line := range rawLines {
					trimmed := strings.TrimSpace(line)
					if trimmed != "" {
						commits = append(commits, trimmed)
					}
				}
				if len(commits) > 0 {
					info.HasUpdate = true
					info.LatestCommit = remoteHash
					remoteMsgOut, _ := exec.CommandContext(ctx, "git", "-C", u.repoDir, "log", "-1", "--format=%s", upstreamRef).Output()
					info.LatestMessage = strings.TrimSpace(string(remoteMsgOut))
					info.PendingCommits = commits
				}
			}
		}
	}

	u.lastInfo = info
	return info, nil
}

// ApplyUpdate pulls changes, compiles a new binary, swaps it, and restarts the manager without stopping Minecraft.
func (u *Updater) ApplyUpdate(ctx context.Context) error {
	u.mu.Lock()
	if u.isUpdating {
		u.mu.Unlock()
		return errors.New("update is already in progress")
	}
	u.isUpdating = true
	u.mu.Unlock()

	defer func() {
		u.mu.Lock()
		u.isUpdating = false
		u.mu.Unlock()
	}()

	log.Println("[UPDATER] Initiating self-update: Step 1/3 git pull...")

	// 1. git pull
	pullCmd := exec.CommandContext(ctx, "git", "-C", u.repoDir, "pull")
	pullOut, err := pullCmd.CombinedOutput()
	if err != nil {
		log.Printf("[UPDATER] git pull failed: %v, output: %s", err, string(pullOut))
		return fmt.Errorf("git pull failed: %s (%w)", strings.TrimSpace(string(pullOut)), err)
	}

	log.Println("[UPDATER] Step 2/3 building new binary (go build)...")

	// 2. Build new binary: go build -o mc_manager_new cmd/server/main.go
	buildTarget := filepath.Join(u.repoDir, "mc_manager_new")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", buildTarget, "cmd/server/main.go")
	buildCmd.Dir = u.repoDir
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(buildTarget)
		log.Printf("[UPDATER] go build failed: %v, output: %s", err, string(buildOut))
		return fmt.Errorf("build failed: %s (%w)", strings.TrimSpace(string(buildOut)), err)
	}

	log.Println("[UPDATER] Step 3/3 swapping binary and triggering zero-downtime restart...")

	// 3. Locate current executable
	currentExe, err := os.Executable()
	if err != nil {
		_ = os.Remove(buildTarget)
		return fmt.Errorf("cannot find current executable path: %w", err)
	}

	// Backup current binary and atomic replace
	backupExe := currentExe + ".old"
	_ = os.Remove(backupExe)
	if err := os.Rename(currentExe, backupExe); err != nil {
		_ = os.Remove(buildTarget)
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	if err := os.Rename(buildTarget, currentExe); err != nil {
		// Rollback
		_ = os.Rename(backupExe, currentExe)
		return fmt.Errorf("failed to place new binary: %w", err)
	}
	_ = os.Chmod(currentExe, 0755)

	// 4. Trigger seamless restart in background after small grace period
	go func() {
		time.Sleep(1 * time.Second)
		log.Println("[UPDATER] Launching new server binary...")

		cmd := exec.Command(currentExe, os.Args[1:]...)
		cmd.Dir = u.repoDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = os.Environ()

		if err := cmd.Start(); err != nil {
			log.Printf("[UPDATER] Failed to spawn new manager process: %v", err)
			return
		}

		log.Printf("[UPDATER] New manager process launched (PID: %d). Shutting down old manager cleanly...", cmd.Process.Pid)
		// Clean exit of current process
		os.Exit(0)
	}()

	return nil
}
