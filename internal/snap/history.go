package snap

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// RecentHistory returns up to n recent unique shell commands from the user's
// history file. It checks $HISTFILE first, then falls back to ~/.zsh_history
// and ~/.bash_history. Both zsh extended format (": ts:0;cmd") and plain
// lines (bash) are handled.
func RecentHistory(n int) []string {
	path := historyFilePath()
	if path == "" {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Zsh extended_history format: ": <timestamp>:<elapsed>;<command>"
		if strings.HasPrefix(line, ": ") {
			if idx := strings.Index(line, ";"); idx != -1 {
				line = strings.TrimSpace(line[idx+1:])
			}
		}
		if line != "" {
			lines = append(lines, line)
		}
	}

	// Deduplicate while preserving order, keeping the most recent occurrence.
	seen := make(map[string]bool)
	unique := make([]string, 0, n)
	for i := len(lines) - 1; i >= 0 && len(unique) < n; i-- {
		cmd := lines[i]
		if !seen[cmd] {
			seen[cmd] = true
			unique = append(unique, cmd)
		}
	}
	return unique
}

func historyFilePath() string {
	if h := os.Getenv("HISTFILE"); h != "" {
		return h
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	// Prefer zsh history, fall back to bash.
	for _, name := range []string{".zsh_history", ".bash_history"} {
		p := filepath.Join(home, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
