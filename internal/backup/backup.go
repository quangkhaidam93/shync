package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Dir returns the backup directory path.
func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "shync", "backups")
}

// Create creates a timestamped backup of the given file.
// Returns the backup path or empty string if the source doesn't exist.
func Create(srcPath string) (string, error) {
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		return "", nil
	}

	backupDir := Dir()
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", fmt.Errorf("creating backup directory: %w", err)
	}

	filename := filepath.Base(srcPath)
	timestamp := time.Now().Format("20060102_15:04:05")
	backupName := fmt.Sprintf("%s.bk_%s", filename, timestamp)
	backupPath := filepath.Join(backupDir, backupName)

	src, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("opening source: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		return "", fmt.Errorf("creating backup file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("copying to backup: %w", err)
	}

	return backupPath, nil
}

// Clean removes backup files older than the given expiry duration.
// Returns the list of deleted filenames.
func Clean(expiry time.Duration) ([]string, error) {
	dir := Dir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading backup directory: %w", err)
	}

	var removed []string
	now := time.Now()

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ts, ok := parseBackupTimestamp(name)
		if !ok {
			continue
		}
		if now.Sub(ts) > expiry {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				return removed, fmt.Errorf("removing %s: %w", name, err)
			}
			removed = append(removed, name)
		}
	}
	return removed, nil
}

// parseBackupTimestamp extracts the timestamp from a backup filename
// with the format "<name>.bk_YYYYMMDD_HH:MM:SS".
func parseBackupTimestamp(name string) (time.Time, bool) {
	idx := strings.Index(name, ".bk_")
	if idx < 0 {
		return time.Time{}, false
	}
	tsStr := name[idx+len(".bk_"):]
	t, err := time.Parse("20060102_15:04:05", tsStr)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
