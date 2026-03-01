package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/quangkhaidam93/shync/internal/fileutil"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var removeCmd = &cobra.Command{
	Use:     "remove",
	Aliases: []string{"rm"},
	Short:   "Remove tracked files from config",
	Long:    "Select tracked files to stop syncing. This only removes them from\ntracking — local and remote files are not deleted.",
	Args:    cobra.NoArgs,
	RunE:    runRemove,
}

func init() {
	rootCmd.AddCommand(removeCmd)
}

type removeItem struct {
	Name      string
	Path      string
	AbsPath   string
	Index     int
	Selected  bool
}

func runRemove(cmd *cobra.Command, args []string) error {
	if len(cfg.Files) == 0 {
		fmt.Println("No tracked files.")
		return nil
	}

	// Build items from tracked files.
	items := make([]removeItem, len(cfg.Files))
	for i, f := range cfg.Files {
		items[i] = removeItem{
			Name:    f.RemoteName,
			Path:    f.LocalPath,
			AbsPath: func() string {
				abs, _ := filepath.Abs(fileutil.ExpandPath(f.LocalPath))
				return abs
			}(),
			Index: i,
		}
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
		fmt.Printf("  Remove tracked files  (space=select, enter=confirm, esc=quit)\r\n\r\n")
		for i, it := range items {
			prefix := "  "
			if i == cursor {
				prefix = "> "
			}
			check := "[ ]"
			if it.Selected {
				check = "[x]"
			}
			if i == cursor {
				fmt.Printf("  \033[1;31m%s%s %s (%s)\033[0m\r\n", prefix, check, it.Name, it.Path)
			} else if it.Selected {
				fmt.Printf("  \033[31m%s%s %s (%s)\033[0m\r\n", prefix, check, it.Name, it.Path)
			} else {
				fmt.Printf("  %s%s %s (%s)\r\n", prefix, check, it.Name, it.Path)
			}
		}

		count := 0
		for _, it := range items {
			if it.Selected {
				count++
			}
		}
		if count > 0 {
			fmt.Printf("\r\n  \033[31m%d file(s) to remove\033[0m\r\n", count)
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

			var toRemove []removeItem
			for _, it := range items {
				if it.Selected {
					toRemove = append(toRemove, it)
				}
			}
			if len(toRemove) == 0 {
				fmt.Println("No files selected.")
				return nil
			}

			// Confirm.
			prompt := promptui.Prompt{
				Label:     fmt.Sprintf("Remove %d file(s) from tracking", len(toRemove)),
				IsConfirm: true,
			}
			if _, err := prompt.Run(); err != nil {
				fmt.Println("Aborted.")
				return nil
			}

			// Remove in reverse index order to keep indices valid.
			for i := len(toRemove) - 1; i >= 0; i-- {
				idx := toRemove[i].Index
				cfg.Files = append(cfg.Files[:idx], cfg.Files[idx+1:]...)
			}

			if err := cfg.Save(); err != nil {
				return fmt.Errorf("saving config: %w", err)
			}

			fmt.Printf("Removed %d file(s) from tracking:\n", len(toRemove))
			for _, it := range toRemove {
				fmt.Printf("  %s\n", it.Path)
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
