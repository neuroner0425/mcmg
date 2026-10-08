package mcservice

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type WorldBackupInfo struct {
	FileName    string `json:"file_name"`
	SizeMB      float64 `json:"size_mb"`
	CreatedAt   string `json:"created_at"`
	Description string `json:"description"`
}

type BackupManager struct {
	mu        sync.Mutex
	serverDir string
	backupDir string
	rcon      RCONClient
	procMgr   *ProcessManager
}

func NewBackupManager(serverDir, backupDir string, rcon RCONClient, procMgr *ProcessManager) *BackupManager {
	_ = os.MkdirAll(backupDir, 0755)
	return &BackupManager{
		serverDir: serverDir,
		backupDir: backupDir,
		rcon:      rcon,
		procMgr:   procMgr,
	}
}

// ListBackups returns all world tar.gz backups sorted by newest first.
func (bm *BackupManager) ListBackups() ([]WorldBackupInfo, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	entries, err := os.ReadDir(bm.backupDir)
	if err != nil {
		return nil, err
	}

	var list []WorldBackupInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "world_") || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		sizeMB := float64(info.Size()) / (1024 * 1024)
		list = append(list, WorldBackupInfo{
			FileName:  entry.Name(),
			SizeMB:    round(sizeMB, 2),
			CreatedAt: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt > list[j].CreatedAt
	})

	return list, nil
}

// CreateBackup pauses world saving via RCON, archives world folders, and resumes saving.
func (bm *BackupManager) CreateBackup(desc string) (WorldBackupInfo, error) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	_ = os.MkdirAll(bm.backupDir, 0755)

	// Flush and pause autosave if RCON is reachable
	if bm.rcon != nil {
		_, _ = bm.rcon.Execute("save-off")
		_, _ = bm.rcon.Execute("save-all flush")
		time.Sleep(1 * time.Second)
		defer func() {
			if bm.rcon != nil {
				_, _ = bm.rcon.Execute("save-on")
			}
		}()
	}

	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("world_%s.tar.gz", timestamp)
	targetPath := filepath.Join(bm.backupDir, filename)

	tarFile, err := os.Create(targetPath)
	if err != nil {
		return WorldBackupInfo{}, fmt.Errorf("failed to create backup file: %w", err)
	}
	defer tarFile.Close()

	gw := gzip.NewWriter(tarFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Folders to back up
	worldFolders := []string{"world", "world_nether", "world_the_end"}
	hasAnyWorld := false

	for _, folder := range worldFolders {
		folderPath := filepath.Join(bm.serverDir, folder)
		if fi, err := os.Stat(folderPath); err == nil && fi.IsDir() {
			hasAnyWorld = true
			if err := addDirToTar(tw, folderPath, folder); err != nil {
				_ = os.Remove(targetPath)
				return WorldBackupInfo{}, fmt.Errorf("failed to archive %s: %w", folder, err)
			}
		}
	}

	if !hasAnyWorld {
		_ = os.Remove(targetPath)
		return WorldBackupInfo{}, fmt.Errorf("no world folders found in %s", bm.serverDir)
	}

	_ = tw.Close()
	_ = gw.Close()
	_ = tarFile.Close()

	fi, err := os.Stat(targetPath)
	if err != nil {
		return WorldBackupInfo{}, err
	}

	return WorldBackupInfo{
		FileName:    filename,
		SizeMB:      round(float64(fi.Size())/(1024*1024), 2),
		CreatedAt:   time.Now().Format("2006-01-02 15:04:05"),
		Description: desc,
	}, nil
}

// DeleteBackup deletes a backup archive.
func (bm *BackupManager) DeleteBackup(filename string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	cleanName := filepath.Base(filename)
	if !strings.HasPrefix(cleanName, "world_") || !strings.HasSuffix(cleanName, ".tar.gz") {
		return fmt.Errorf("invalid backup filename")
	}

	target := filepath.Join(bm.backupDir, cleanName)
	return os.Remove(target)
}

func addDirToTar(tw *tar.Writer, srcDir, tarBase string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}

		tarPath := filepath.Join(tarBase, rel)
		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(tarPath)

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tw, file)
		return err
	})
}
