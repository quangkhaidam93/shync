package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/quangkhaidam93/shync/internal/config"
	"github.com/quangkhaidam93/shync/internal/fileutil"
	"github.com/quangkhaidam93/shync/internal/storage"
	"golang.org/x/term"
)

// errAborted is returned when the user exits the picker. Callers should
// treat it as a silent no-op rather than printing an error.
var errAborted = errors.New("")

type pickItem struct {
	RemoteName string
	LocalPath  string
	IsExit     bool
}

// pickTrackedFile displays tracked files and prompts the user to choose one
// using arrow-key navigation. Includes an "Exit" option to abort.
func pickTrackedFile(prompt string) (*config.FileEntry, error) {
	if len(cfg.Files) == 0 {
		return nil, fmt.Errorf("no tracked files — use 'shync push <file>' to start tracking")
	}

	items := make([]pickItem, len(cfg.Files)+1)
	for i, f := range cfg.Files {
		items[i] = pickItem{RemoteName: f.RemoteName, LocalPath: f.LocalPath}
	}
	items[len(items)-1] = pickItem{RemoteName: "Exit", IsExit: true}

	templates := &promptui.SelectTemplates{
		Label: "{{ . }}",
		Active: `{{ if .IsExit }}` +
			"\U0000276F {{ .RemoteName | red }}" +
			`{{ else }}` +
			"\U0000276F {{ .RemoteName | cyan }} ({{ .LocalPath }})" +
			`{{ end }}`,
		Inactive: `{{ if .IsExit }}` +
			"  {{ .RemoteName }}" +
			`{{ else }}` +
			"  {{ .RemoteName }} ({{ .LocalPath }})" +
			`{{ end }}`,
		Selected: `{{ if .IsExit }}` +
			"Aborted." +
			`{{ else }}` +
			"\U00002713 {{ .RemoteName | cyan }}" +
			`{{ end }}`,
	}

	sel := promptui.Select{
		Label:     prompt,
		Items:     items,
		Templates: templates,
		Size:      10,
	}

	idx, _, err := sel.Run()
	if err != nil {
		return nil, errAborted
	}

	if items[idx].IsExit {
		return nil, errAborted
	}

	return &cfg.Files[idx], nil
}

type remotePickItem struct {
	Name   string
	Size   string
	IsExit bool
}

// pickRemoteFile shows a spinner while fetchFn runs in the background
// (covering both backend init and file listing), then presents an
// arrow-key picker with the results.
func pickRemoteFile(prompt string, fetchFn func() ([]storage.FileMetadata, error)) (string, error) {
	type listResult struct {
		files []storage.FileMetadata
		err   error
	}
	ch := make(chan listResult, 1)
	go func() {
		f, e := fetchFn()
		ch <- listResult{f, e}
	}()

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	start := time.Now()
	minDisplay := 500 * time.Millisecond
	i := 0
	var res listResult

	// Spin until fetch completes AND minimum display time has passed
	for {
		select {
		case res = <-ch:
			ch = nil // stop reading, keep spinning until minDisplay
		default:
		}
		if ch == nil && time.Since(start) >= minDisplay {
			break
		}
		fmt.Printf("\r%s Fetching remote files...", frames[i%len(frames)])
		i++
		time.Sleep(80 * time.Millisecond)
	}
	fmt.Printf("\r\033[K")

	if res.err != nil {
		return "", fmt.Errorf("listing remote files: %w", res.err)
	}
	if len(res.files) == 0 {
		return "", fmt.Errorf("no files found in remote directory %s", cfg.RemoteDir)
	}
	return showRemotePicker(prompt, res.files)
}

func showRemotePicker(prompt string, files []storage.FileMetadata) (string, error) {
	items := make([]remotePickItem, len(files)+1)
	for i, f := range files {
		items[i] = remotePickItem{Name: f.Name, Size: formatSize(f.Size)}
	}
	items[len(items)-1] = remotePickItem{Name: "Exit", IsExit: true}

	templates := &promptui.SelectTemplates{
		Label: "{{ . }}",
		Active: `{{ if .IsExit }}` +
			"\U0000276F {{ .Name | red }}" +
			`{{ else }}` +
			"\U0000276F {{ .Name | cyan }} ({{ .Size }})" +
			`{{ end }}`,
		Inactive: `{{ if .IsExit }}` +
			"  {{ .Name }}" +
			`{{ else }}` +
			"  {{ .Name }} ({{ .Size }})" +
			`{{ end }}`,
		Selected: `{{ if .IsExit }}` +
			"" +
			`{{ else }}` +
			"\U00002713 {{ .Name | cyan }}" +
			`{{ end }}`,
	}

	sel := promptui.Select{
		Label:     prompt,
		Items:     items,
		Templates: templates,
		Size:      10,
	}

	idx, _, err := sel.Run()
	if err != nil {
		return "", errAborted
	}

	if items[idx].IsExit {
		return "", errAborted
	}

	return files[idx].Name, nil
}

// finderItem represents an entry in the file finder.
type finderItem struct {
	Name        string
	AbsPath     string
	IsDir       bool
	IsParent    bool // ../
	IsSupported bool // matches a supported_files pattern
	IsTracked   bool // already tracked, shown but not selectable
	Selected    bool
}

// readDir returns finder items for the given directory.
func readDir(dir string, patterns map[string]bool, tracked map[string]bool) []finderItem {
	var items []finderItem

	// Parent directory entry (unless at root).
	if dir != "/" {
		items = append(items, finderItem{
			Name:     "../",
			AbsPath:  filepath.Dir(dir),
			IsDir:    true,
			IsParent: true,
		})
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return items
	}

	// Show files matching supported_files patterns. Tracked files are
	// displayed but not selectable.
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !patterns[e.Name()] {
			continue
		}
		abs := filepath.Join(dir, e.Name())
		items = append(items, finderItem{
			Name:        e.Name(),
			AbsPath:     abs,
			IsSupported: true,
			IsTracked:   tracked[abs],
		})
	}
	for _, e := range entries {
		if e.IsDir() {
			items = append(items, finderItem{
				Name:    e.Name() + "/",
				AbsPath: filepath.Join(dir, e.Name()),
				IsDir:   true,
			})
		}
	}

	return items
}

const finderPageSize = 20

// pickMultiFiles shows an interactive file browser starting at $HOME.
// Supported files are highlighted. Space toggles selection on files,
// Enter on a directory navigates into it, Enter submits selected files,
// Esc/Ctrl-C aborts.
func pickMultiFiles(prompt string, patterns []string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home dir: %w", err)
	}

	patternSet := make(map[string]bool, len(patterns))
	for _, p := range patterns {
		patternSet[p] = true
	}

	tracked := make(map[string]bool)
	for _, f := range cfg.Files {
		expanded := fileutil.ExpandPath(f.LocalPath)
		abs, _ := filepath.Abs(expanded)
		tracked[abs] = true
	}

	// Global selection map: abs path -> true.
	selected := make(map[string]bool)

	cwd := home
	items := readDir(cwd, patternSet, tracked)
	cursor := 0

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return nil, fmt.Errorf("enabling raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	buf := make([]byte, 3)

	render := func() {
		fmt.Print("\033[H\033[J")
		displayDir := fileutil.ContractPath(cwd)
		fmt.Printf("  %s\r\n", displayDir)
		fmt.Printf("  %s  (space=select, enter=open/submit, esc=quit)\r\n\r\n", prompt)

		// Viewport: show finderPageSize items around cursor.
		start := 0
		end := len(items)
		if len(items) > finderPageSize {
			start = cursor - finderPageSize/2
			if start < 0 {
				start = 0
			}
			end = start + finderPageSize
			if end > len(items) {
				end = len(items)
				start = end - finderPageSize
			}
		}

		for i := start; i < end; i++ {
			it := items[i]
			prefix := "  "
			if i == cursor {
				prefix = "> "
			}

			switch {
			case it.IsDir:
				if i == cursor {
					fmt.Printf("  \033[1;34m%s%s\033[0m\r\n", prefix, it.Name)
				} else {
					fmt.Printf("  \033[34m%s%s\033[0m\r\n", prefix, it.Name)
				}
			case it.IsTracked:
				fmt.Printf("  \033[2m%s    %s (tracked)\033[0m\r\n", prefix, it.Name)
			case it.IsSupported:
				check := "[ ]"
				if selected[it.AbsPath] {
					check = "[x]"
				}
				if i == cursor {
					fmt.Printf("  \033[1;36m%s%s %s\033[0m\r\n", prefix, check, it.Name)
				} else {
					fmt.Printf("  \033[36m%s%s %s\033[0m\r\n", prefix, check, it.Name)
				}
			default:
				check := "[ ]"
				if selected[it.AbsPath] {
					check = "[x]"
				}
				if i == cursor {
					fmt.Printf("  \033[1m%s%s %s\033[0m\r\n", prefix, check, it.Name)
				} else {
					fmt.Printf("  %s%s %s\r\n", prefix, check, it.Name)
				}
			}
		}

		if len(items) > finderPageSize {
			fmt.Printf("\r\n  \033[2m(%d/%d)\033[0m\r\n", cursor+1, len(items))
		}

		count := len(selected)
		if count > 0 {
			fmt.Printf("\r\n  \033[32m%d file(s) selected\033[0m\r\n", count)
		}
	}

	render()
	for {
		n, readErr := os.Stdin.Read(buf)
		if readErr != nil {
			return nil, errAborted
		}

		switch {
		// Arrow keys.
		case n == 3 && buf[0] == 0x1b && buf[1] == '[':
			switch buf[2] {
			case 'A': // Up
				if cursor > 0 {
					cursor--
				}
			case 'B': // Down
				if cursor < len(items)-1 {
					cursor++
				}
			}

		// Space — toggle selection (untracked files only).
		case n == 1 && buf[0] == ' ':
			it := items[cursor]
			if !it.IsDir && !it.IsTracked {
				if selected[it.AbsPath] {
					delete(selected, it.AbsPath)
				} else {
					selected[it.AbsPath] = true
				}
			}

		// Enter — navigate into directory, or submit selection.
		case n == 1 && (buf[0] == '\r' || buf[0] == '\n'):
			it := items[cursor]
			if it.IsDir {
				cwd = it.AbsPath
				items = readDir(cwd, patternSet, tracked)
				cursor = 0
			} else if !it.IsTracked {
				// Submit if there are selections.
				if len(selected) == 0 {
					// Single-select: pick current file.
					selected[it.AbsPath] = true
				}
				fmt.Print("\033[H\033[J")
				term.Restore(fd, oldState)
				var result []string
				for p := range selected {
					result = append(result, fileutil.ContractPath(p))
				}
				return result, nil
			}

		// Backspace — go to parent directory.
		case n == 1 && (buf[0] == 0x7f || buf[0] == 0x08):
			if cwd != "/" {
				cwd = filepath.Dir(cwd)
				items = readDir(cwd, patternSet, tracked)
				cursor = 0
			}

		// Esc, Ctrl-C — abort.
		case n == 1 && (buf[0] == 0x1b || buf[0] == 0x03):
			fmt.Print("\033[H\033[J")
			term.Restore(fd, oldState)
			return nil, errAborted
		}

		// Sync selection state into current items for rendering.
		for i := range items {
			if !items[i].IsDir {
				items[i].Selected = selected[items[i].AbsPath]
			}
		}

		render()
	}
}

func formatSize(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
