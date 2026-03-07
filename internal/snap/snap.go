package snap

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Snap represents a saved shell command snippet.
type Snap struct {
	Name string `json:"name"`
	Cmd  string `json:"cmd"`
}

// Parse decodes JSONL-encoded snap data (one JSON object per line).
// Empty lines are silently skipped.
func Parse(data []byte) ([]Snap, error) {
	var snaps []Snap
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var s Snap
		if err := json.Unmarshal(line, &s); err != nil {
			return nil, fmt.Errorf("parsing snap entry: %w", err)
		}
		snaps = append(snaps, s)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading snaps: %w", err)
	}
	return snaps, nil
}

// Format encodes a slice of Snaps to JSONL (one JSON object per line).
func Format(snaps []Snap) ([]byte, error) {
	var buf bytes.Buffer
	for _, s := range snaps {
		line, err := json.Marshal(s)
		if err != nil {
			return nil, fmt.Errorf("encoding snap %q: %w", s.Name, err)
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// LoadLocal reads and parses snaps from a local JSONL file.
// Returns an empty slice (not an error) if the file does not exist.
func LoadLocal(path string) ([]Snap, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading local snaps: %w", err)
	}
	return Parse(data)
}

// SaveLocal writes snaps to a local JSONL file, creating it if necessary.
func SaveLocal(path string, snaps []Snap) error {
	data, err := Format(snaps)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// filepath_dir is a thin wrapper so we don't need to import path/filepath
// at package level just for one call — the caller handles directory creation.
func filepath_dir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return "."
}
