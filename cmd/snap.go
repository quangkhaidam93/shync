package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/quangkhaidam93/shync/internal/snap"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const snapsRemoteFile = "snaps.jsonl"

var snapCmd = &cobra.Command{
	Use:   "snap",
	Short: "Manage saved command snippets",
	Long:  "Store and retrieve frequently used shell commands, synced across all your devices.",
	RunE:  runSnapList,
}

var snapListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved snaps; search by regex and paste selected to shell prompt",
	Args:  cobra.NoArgs,
	RunE:  runSnapList,
}

var snapAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Save a new command snippet",
	Args:  cobra.NoArgs,
	RunE:  runSnapAdd,
}

var snapRemoveCmd = &cobra.Command{
	Use:     "remove",
	Aliases: []string{"rm"},
	Short:   "Remove saved snaps",
	Args:    cobra.NoArgs,
	RunE:    runSnapRemove,
}

var snapSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync local snaps with remote backup",
	Long:  "Merges local snaps with the remote backup file. Entries present in either source are kept; on name conflicts the local version wins. The merged result is saved both locally and to remote.",
	Args:  cobra.NoArgs,
	RunE:  runSnapSync,
}

func init() {
	snapCmd.AddCommand(snapListCmd, snapAddCmd, snapRemoveCmd, snapSyncCmd)
	rootCmd.AddCommand(snapCmd)
}

// localSnapsPath returns the path to the local snaps file.
func localSnapsPath() string {
	return filepath.Join(cfg.Dir(), snapsRemoteFile)
}

// remoteSnapsPath returns the remote path for the snaps backup file.
func remoteSnapsPath() string {
	return cfg.RemoteDir + "/" + snapsRemoteFile
}

// loadLocalSnaps reads snaps from the local file (fast, no network).
func loadLocalSnaps() ([]snap.Snap, error) {
	snaps, err := snap.LoadLocal(localSnapsPath())
	if err != nil {
		return nil, fmt.Errorf("loading local snaps: %w", err)
	}
	return snaps, nil
}

// saveSnaps writes snaps to the local file and uploads to remote as backup.
func saveSnaps(snaps []snap.Snap) error {
	if err := snap.SaveLocal(localSnapsPath(), snaps); err != nil {
		return fmt.Errorf("saving local snaps: %w", err)
	}
	if err := uploadSnapsToRemote(snaps); err != nil {
		return fmt.Errorf("backing up to remote: %w", err)
	}
	return nil
}

// uploadSnapsToRemote serializes snaps and uploads them to the active backend.
func uploadSnapsToRemote(snaps []snap.Snap) error {
	data, err := snap.Format(snaps)
	if err != nil {
		return fmt.Errorf("formatting snaps: %w", err)
	}
	// Capture data in a local copy for the goroutine closure.
	payload := data
	return runWithSpinner("Backing up snaps to remote...", func() error {
		backend, err := newBackend()
		if err != nil {
			return fmt.Errorf("initializing backend: %w", err)
		}
		defer backend.Close()
		return backend.Upload(context.Background(), remoteSnapsPath(), bytes.NewReader(payload), snapsRemoteFile)
	})
}

// downloadRemoteSnaps fetches snaps from the remote backup.
// Returns nil (not an error) if the remote file does not exist yet.
func downloadRemoteSnaps() ([]snap.Snap, error) {
	backend, err := newBackend()
	if err != nil {
		return nil, fmt.Errorf("initializing backend: %w", err)
	}
	defer backend.Close()

	exists, err := backend.Exists(context.Background(), remoteSnapsPath())
	if err != nil {
		return nil, fmt.Errorf("checking remote: %w", err)
	}
	if !exists {
		return nil, nil
	}

	var buf bytes.Buffer
	if err := backend.Download(context.Background(), remoteSnapsPath(), &buf); err != nil {
		return nil, fmt.Errorf("downloading remote snaps: %w", err)
	}
	return snap.Parse(buf.Bytes())
}

// mergeSnaps merges local and remote snap lists. All entries from both sources
// are included; on duplicate names the local entry wins.
func mergeSnaps(local, remote []snap.Snap) ([]snap.Snap, int, int) {
	seen := make(map[string]bool, len(local))
	merged := make([]snap.Snap, len(local))
	copy(merged, local)
	for _, s := range local {
		seen[strings.ToLower(s.Name)] = true
	}

	added := 0
	for _, s := range remote {
		if !seen[strings.ToLower(s.Name)] {
			merged = append(merged, s)
			added++
		}
	}
	// Count local entries not in remote (only in local).
	remoteNames := make(map[string]bool, len(remote))
	for _, s := range remote {
		remoteNames[strings.ToLower(s.Name)] = true
	}
	onlyLocal := 0
	for _, s := range local {
		if !remoteNames[strings.ToLower(s.Name)] {
			onlyLocal++
		}
	}
	return merged, added, onlyLocal
}

// --- list ---

func runSnapList(_ *cobra.Command, _ []string) error {
	snaps, err := loadLocalSnaps()
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		fmt.Println("No snaps saved yet. Use 'shync snap add' to save your first command.")
		return nil
	}

	items := make([]string, len(snaps))
	for i, s := range snaps {
		items[i] = fmt.Sprintf("%-20s  %s", s.Name, s.Cmd)
	}

	sel := promptui.Select{
		Label:             "Select a snap  (type to search with regex, enter to paste to prompt)",
		Items:             items,
		Size:              15,
		StartInSearchMode: true,
		Searcher: func(input string, index int) bool {
			if input == "" {
				return true
			}
			re, err := regexp.Compile("(?i)" + input)
			if err != nil {
				// Invalid regex — fall back to plain case-insensitive substring match.
				return strings.Contains(strings.ToLower(items[index]), strings.ToLower(input))
			}
			return re.MatchString(items[index])
		},
	}
	idx, _, err := sel.Run()
	if err != nil {
		return nil
	}

	pasteToShellPrompt(snaps[idx].Cmd)
	return nil
}

// --- add ---

func runSnapAdd(_ *cobra.Command, _ []string) error {
	existing, err := loadLocalSnaps()
	if err != nil {
		return err
	}

	taken := make(map[string]bool, len(existing))
	for _, s := range existing {
		taken[strings.ToLower(s.Name)] = true
	}

	// --- step 1: resolve command string ---
	cmdStr, err := resolveCommandInput()
	if err != nil {
		return nil // user cancelled
	}

	// --- step 2: name ---
	namePrompt := promptui.Prompt{
		Label: "Name",
		Validate: func(input string) error {
			input = strings.TrimSpace(input)
			if input == "" {
				return fmt.Errorf("name cannot be empty")
			}
			if taken[strings.ToLower(input)] {
				return fmt.Errorf("a snap named %q already exists", input)
			}
			return nil
		},
	}
	name, err := namePrompt.Run()
	if err != nil {
		return nil
	}
	name = strings.TrimSpace(name)

	updated := append(existing, snap.Snap{Name: name, Cmd: cmdStr})
	if err := saveSnaps(updated); err != nil {
		return err
	}
	fmt.Printf("Snap %q saved.\n", name)
	return nil
}

// resolveCommandInput lets the user either type a command or pick from recent
// shell history. Returns the command string, or an error if cancelled.
func resolveCommandInput() (string, error) {
	history := snap.RecentHistory(10)

	if len(history) > 0 {
		sourceSel := promptui.Select{
			Label: "Command source",
			Items: []string{"Type manually", "Pick from recent history"},
		}
		_, choice, err := sourceSel.Run()
		if err != nil {
			return "", err
		}

		if choice == "Pick from recent history" {
			histSel := promptui.Select{
				Label: "Select a recent command",
				Items: history,
				Size:  10,
			}
			_, picked, err := histSel.Run()
			if err != nil {
				return "", err
			}

			// Let the user confirm or edit the picked command.
			editPrompt := promptui.Prompt{
				Label:   "Command",
				Default: picked,
				Validate: func(input string) error {
					if strings.TrimSpace(input) == "" {
						return fmt.Errorf("command cannot be empty")
					}
					return nil
				},
			}
			cmd, err := editPrompt.Run()
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(cmd), nil
		}
	}

	cmdPrompt := promptui.Prompt{
		Label: "Command",
		Validate: func(input string) error {
			if strings.TrimSpace(input) == "" {
				return fmt.Errorf("command cannot be empty")
			}
			return nil
		},
	}
	cmd, err := cmdPrompt.Run()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(cmd), nil
}

// --- remove ---

type snapRemoveItem struct {
	snap.Snap
	Index    int
	Selected bool
}

func runSnapRemove(_ *cobra.Command, _ []string) error {
	snaps, err := loadLocalSnaps()
	if err != nil {
		return err
	}
	if len(snaps) == 0 {
		fmt.Println("No snaps to remove.")
		return nil
	}

	items := make([]snapRemoveItem, len(snaps))
	for i, s := range snaps {
		items[i] = snapRemoveItem{Snap: s, Index: i}
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("enabling raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	cursor := 0
	buf := make([]byte, 3)

	render := func() {
		fmt.Print("\033[H\033[J")
		fmt.Printf("  Remove snaps  (space=select, enter=confirm, esc=quit)\r\n\r\n")
		for i, it := range items {
			prefix := "  "
			if i == cursor {
				prefix = "> "
			}
			check := "[ ]"
			if it.Selected {
				check = "[x]"
			}
			label := fmt.Sprintf("%s (%s)", it.Name, it.Cmd)
			if i == cursor {
				fmt.Printf("  \033[1;31m%s%s %s\033[0m\r\n", prefix, check, label)
			} else if it.Selected {
				fmt.Printf("  \033[31m%s%s %s\033[0m\r\n", prefix, check, label)
			} else {
				fmt.Printf("  %s%s %s\r\n", prefix, check, label)
			}
		}
		count := 0
		for _, it := range items {
			if it.Selected {
				count++
			}
		}
		if count > 0 {
			fmt.Printf("\r\n  \033[31m%d snap(s) to remove\033[0m\r\n", count)
		}
	}

	render()
	for {
		n, readErr := os.Stdin.Read(buf)
		if readErr != nil {
			return nil
		}

		switch {
		case n == 3 && buf[0] == 0x1b && buf[1] == '[':
			switch buf[2] {
			case 'A':
				if cursor > 0 {
					cursor--
				}
			case 'B':
				if cursor < len(items)-1 {
					cursor++
				}
			}

		case n == 1 && buf[0] == ' ':
			items[cursor].Selected = !items[cursor].Selected

		case n == 1 && (buf[0] == '\r' || buf[0] == '\n'):
			fmt.Print("\033[H\033[J")
			term.Restore(fd, oldState)

			var toRemove []snapRemoveItem
			for _, it := range items {
				if it.Selected {
					toRemove = append(toRemove, it)
				}
			}
			if len(toRemove) == 0 {
				fmt.Println("No snaps selected.")
				return nil
			}

			prompt := promptui.Prompt{
				Label:     fmt.Sprintf("Remove %d snap(s)", len(toRemove)),
				IsConfirm: true,
			}
			if _, err := prompt.Run(); err != nil {
				fmt.Println("Aborted.")
				return nil
			}

			removeIdx := make(map[int]bool, len(toRemove))
			for _, it := range toRemove {
				removeIdx[it.Index] = true
			}

			updated := snaps[:0]
			for i, s := range snaps {
				if !removeIdx[i] {
					updated = append(updated, s)
				}
			}

			if err := saveSnaps(updated); err != nil {
				return err
			}
			fmt.Printf("Removed %d snap(s):\n", len(toRemove))
			for _, it := range toRemove {
				fmt.Printf("  %s: %s\n", it.Name, it.Cmd)
			}
			return nil

		case n == 1 && (buf[0] == 0x1b || buf[0] == 0x03):
			fmt.Print("\033[H\033[J")
			term.Restore(fd, oldState)
			return nil
		}

		render()
	}
}

// --- sync ---

func runSnapSync(_ *cobra.Command, _ []string) error {
	var remote []snap.Snap
	if err := runWithSpinner("Fetching remote snaps...", func() error {
		var err error
		remote, err = downloadRemoteSnaps()
		return err
	}); err != nil {
		return err
	}

	local, err := loadLocalSnaps()
	if err != nil {
		return err
	}

	merged, fromRemote, onlyLocal := mergeSnaps(local, remote)

	if fromRemote == 0 && onlyLocal == 0 && len(local) == len(remote) {
		fmt.Println("Already in sync. Nothing to do.")
		return nil
	}

	if err := snap.SaveLocal(localSnapsPath(), merged); err != nil {
		return fmt.Errorf("saving merged snaps locally: %w", err)
	}
	if err := uploadSnapsToRemote(merged); err != nil {
		return fmt.Errorf("uploading merged snaps: %w", err)
	}

	fmt.Printf("Sync complete. Total: %d snap(s)", len(merged))
	if fromRemote > 0 {
		fmt.Printf(", +%d pulled from remote", fromRemote)
	}
	if onlyLocal > 0 {
		fmt.Printf(", +%d pushed to remote", onlyLocal)
	}
	fmt.Println(".")
	return nil
}

