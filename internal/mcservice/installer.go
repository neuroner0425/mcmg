package mcservice

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	purpurAPIBase   = "https://api.purpurmc.org/v2/purpur"
	streamChunkSize = 512 * 1024 // 512KB stream read buffer
)

// PurpurVersionResponse defines API schema for versions.
type PurpurVersionResponse struct {
	Versions []string `json:"versions"`
}

// PurpurBuildResponse defines API schema for builds of a specific version.
type PurpurBuildResponse struct {
	Builds struct {
		Latest string   `json:"latest"`
		All    []string `json:"all"`
	} `json:"builds"`
}

// Installer handles downloading Purpur jar files and ensuring EULA compliance.
type Installer struct {
	apiClient      *http.Client
	downloadClient *http.Client

	isBusy     atomic.Bool
	downloaded atomic.Int64
	totalBytes atomic.Int64
	bytesPerSec atomic.Int64

	statusTextMu sync.RWMutex
	statusText   string
}

// NewInstaller creates an optimized Installer instance.
func NewInstaller() *Installer {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ForceAttemptHTTP2 = true

	return &Installer{
		apiClient: &http.Client{
			Transport: tr,
			Timeout:   15 * time.Second,
		},
		downloadClient: &http.Client{
			Transport: tr,
			Timeout:   10 * time.Minute, // 넉넉한 10분 타임아웃
		},
	}
}

// GetStatus returns real-time download status metrics with lock-free atomic reads.
func (ins *Installer) GetStatus() (isBusy bool, progress int, statusText string) {
	busy := ins.isBusy.Load()
	down := ins.downloaded.Load()
	total := ins.totalBytes.Load()
	speed := ins.bytesPerSec.Load()

	ins.statusTextMu.RLock()
	st := ins.statusText
	ins.statusTextMu.RUnlock()

	pct := 0
	if total > 0 {
		pct = int((down * 100) / total)
		if pct > 100 {
			pct = 100
		}
	} else if !busy && down > 0 {
		pct = 100
	}

	if busy {
		speedMB := float64(speed) / (1024 * 1024)
		downMB := float64(down) / (1024 * 1024)
		if total > 0 {
			totalMB := float64(total) / (1024 * 1024)
			st = fmt.Sprintf("다운로드 중... (%d%%, %.1fMB / %.1fMB @ %.1f MB/s)", pct, downMB, totalMB, speedMB)
		} else {
			st = fmt.Sprintf("다운로드 중... (%.1fMB 수신 중 @ %.1f MB/s)", downMB, speedMB)
		}
	}

	return busy, pct, st
}

func (ins *Installer) setStatusText(txt string) {
	ins.statusTextMu.Lock()
	ins.statusText = txt
	ins.statusTextMu.Unlock()
}

// FetchVersions queries the Purpur API for all supported versions in descending order (newest first).
func (ins *Installer) FetchVersions() ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, purpurAPIBase, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PurpurDownloader/1.0 (Server Manager)")

	resp, err := ins.apiClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Purpur versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Purpur API returned HTTP status %d", resp.StatusCode)
	}

	var data PurpurVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse Purpur version JSON: %w", err)
	}

	// Reverse version list so that the newest versions (e.g. 26.3) appear first
	for i, j := 0, len(data.Versions)-1; i < j; i, j = i+1, j-1 {
		data.Versions[i], data.Versions[j] = data.Versions[j], data.Versions[i]
	}

	return data.Versions, nil
}

// FetchBuilds queries the available builds for a given Minecraft version.
func (ins *Installer) FetchBuilds(version string) ([]string, string, error) {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return nil, "", errors.New("version cannot be empty")
	}

	apiURL := fmt.Sprintf("%s/%s", purpurAPIBase, trimmed)
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "PurpurDownloader/1.0 (Server Manager)")

	resp, err := ins.apiClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch Purpur builds: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("Purpur API returned HTTP status %d for version %q", resp.StatusCode, trimmed)
	}

	var data PurpurBuildResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, "", fmt.Errorf("failed to parse Purpur build JSON: %w", err)
	}

	return data.Builds.All, data.Builds.Latest, nil
}

// DownloadPurpur downloads the jar file with high-throughput streaming and real-time speed tracking.
func (ins *Installer) DownloadPurpur(version, build, targetDir, jarName string) error {
	if !ins.isBusy.CompareAndSwap(false, true) {
		return errors.New("another download is already in progress")
	}
	defer ins.isBusy.Store(false)

	ins.downloaded.Store(0)
	ins.totalBytes.Store(0)
	ins.bytesPerSec.Store(0)
	ins.setStatusText("다운로드 서버 연결 중...")

	v := strings.TrimSpace(version)
	b := strings.TrimSpace(build)
	if b == "" {
		b = "latest"
	}

	downloadURL := fmt.Sprintf("%s/%s/%s/download", purpurAPIBase, v, b)

	req, err := http.NewRequest(http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create download request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")

	resp, err := ins.downloadClient.Do(req)
	if err != nil {
		ins.setStatusText("연결 실패: " + err.Error())
		return fmt.Errorf("download connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ins.setStatusText(fmt.Sprintf("HTTP %d 오류", resp.StatusCode))
		return fmt.Errorf("download failed with HTTP %d from %s", resp.StatusCode, downloadURL)
	}

	totalSize := resp.ContentLength
	ins.totalBytes.Store(totalSize)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}

	destPath := filepath.Join(targetDir, jarName)
	// Download into local OS temp directory first to avoid iCloud Drive real-time sync lock/slowdown
	tmpFile, err := os.CreateTemp("", "purpur_dl_*.jar")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	// High throughput buffered writer (256KB)
	writer := bufio.NewWriterSize(tmpFile, 256*1024)
	buf := make([]byte, streamChunkSize)

	// Background speed calculator (every 500ms)
	stopCalc := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		var lastBytes int64
		lastTime := time.Now()

		for {
			select {
			case <-ticker.C:
				currBytes := ins.downloaded.Load()
				now := time.Now()
				durationSec := now.Sub(lastTime).Seconds()
				if durationSec > 0 {
					speed := int64(float64(currBytes-lastBytes) / durationSec)
					ins.bytesPerSec.Store(speed)
				}
				lastBytes = currBytes
				lastTime = now
			case <-stopCalc:
				return
			}
		}
	}()

	var downloadErr error
	for {
		nr, er := resp.Body.Read(buf)
		if nr > 0 {
			nw, ew := writer.Write(buf[0:nr])
			if ew != nil {
				downloadErr = ew
				break
			}
			ins.downloaded.Add(int64(nw))
		}
		if er != nil {
			if errors.Is(er, io.EOF) {
				break
			}
			downloadErr = er
			break
		}
	}
	close(stopCalc)

	if err := writer.Flush(); err != nil && downloadErr == nil {
		downloadErr = err
	}
	_ = tmpFile.Close()

	if downloadErr != nil {
		ins.setStatusText("다운로드 오류: " + downloadErr.Error())
		return fmt.Errorf("stream failed: %w", downloadErr)
	}

	// Move or copy downloaded file to destPath in targetDir
	if err := os.Rename(tmpName, destPath); err != nil {
		// Cross-device link fallback (copy)
		inF, cErr := os.Open(tmpName)
		if cErr != nil {
			return fmt.Errorf("failed to open temp file for copy: %w", cErr)
		}
		defer inF.Close()

		outF, oErr := os.Create(destPath)
		if oErr != nil {
			return fmt.Errorf("failed to create destination jar: %w", oErr)
		}
		defer outF.Close()

		if _, cpErr := io.Copy(outF, inF); cpErr != nil {
			return fmt.Errorf("failed to copy jar to destination: %w", cpErr)
		}
	}

	// Ensure EULA
	if err := ins.EnsureEULA(targetDir); err != nil {
		return fmt.Errorf("failed to setup eula.txt: %w", err)
	}

	ins.downloaded.Store(totalSize)
	ins.setStatusText("다운로드 및 EULA 동의 완료")

	// Persist installed version info
	var finalSize int64 = totalSize
	if fi, err := os.Stat(destPath); err == nil {
		finalSize = fi.Size()
	}
	_ = ins.SaveInstalledVersion(targetDir, InstalledVersionInfo{
		Version:      v,
		Build:        b,
		JarName:      jarName,
		JarSize:      finalSize,
		DownloadedAt: time.Now(),
		Exists:       true,
	})

	return nil
}

// EnsureEULA creates or updates eula.txt to eula=true in targetDir.
func (ins *Installer) EnsureEULA(targetDir string) error {
	eulaPath := filepath.Join(targetDir, "eula.txt")
	content := "# By changing the setting below to TRUE you are indicating your agreement to our EULA (https://aka.ms/MinecraftEULA).\neula=true\n"
	return os.WriteFile(eulaPath, []byte(content), 0644)
}

// InstalledVersionInfo stores metadata about the currently installed Purpur server jar.
type InstalledVersionInfo struct {
	Version      string    `json:"version"`
	Build        string    `json:"build"`
	JarName      string    `json:"jar_name"`
	JarSize      int64     `json:"jar_size"`
	DownloadedAt time.Time `json:"downloaded_at"`
	Exists       bool      `json:"exists"`
}

// SaveInstalledVersion writes version metadata to installed_version.json in targetDir.
func (ins *Installer) SaveInstalledVersion(targetDir string, info InstalledVersionInfo) error {
	infoPath := filepath.Join(targetDir, "installed_version.json")
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(infoPath, data, 0644)
}

// GetInstalledVersion reads installed version info or checks jar file existence.
func (ins *Installer) GetInstalledVersion(targetDir, jarName string) InstalledVersionInfo {
	destPath := filepath.Join(targetDir, jarName)
	fi, err := os.Stat(destPath)
	exists := err == nil && !fi.IsDir()

	infoPath := filepath.Join(targetDir, "installed_version.json")
	data, err := os.ReadFile(infoPath)
	if err == nil {
		var info InstalledVersionInfo
		if err := json.Unmarshal(data, &info); err == nil {
			info.Exists = exists
			if exists && fi != nil {
				info.JarSize = fi.Size()
			}
			return info
		}
	}

	// Fallback if metadata file does not exist yet but jar is present
	if exists {
		return InstalledVersionInfo{
			Version:      "설치됨",
			Build:        "manual",
			JarName:      jarName,
			JarSize:      fi.Size(),
			DownloadedAt: fi.ModTime(),
			Exists:       true,
		}
	}

	return InstalledVersionInfo{
		Version: "",
		Build:   "",
		JarName: jarName,
		JarSize: 0,
		Exists:  false,
	}
}

