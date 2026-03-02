package cmd

import (
	"bufio"
	"fmt"
	"os"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var backends = []string{"google_drive", "synology", "gist"}

var backendCmd = &cobra.Command{
	Use:   "backend",
	Short: "Change the active storage backend",
	Long:  "Select which storage backend to use for syncing files.\nLaunches the setup wizard when switching to a new backend.",
	Args:  cobra.NoArgs,
	RunE:  runBackend,
}

func init() {
	rootCmd.AddCommand(backendCmd)
}

func runBackend(cmd *cobra.Command, args []string) error {
	fmt.Printf("Current backend: %s\n\n", cfg.ActiveBackend)

	// Find current index for cursor start position.
	cursorPos := 0
	for i, b := range backends {
		if b == cfg.ActiveBackend {
			cursorPos = i
			break
		}
	}

	sel := promptui.Select{
		Label:     "Select backend",
		Items:     backends,
		CursorPos: cursorPos,
		Size:      len(backends),
	}

	_, chosen, err := sel.Run()
	if err != nil {
		return nil
	}

	if chosen == cfg.ActiveBackend {
		fmt.Printf("Already using %s.\n", chosen)
		return nil
	}

	cfg.ActiveBackend = chosen

	// Launch setup wizard for the new backend.
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\nConfigure %s:\n", chosen)

	switch chosen {
	case "google_drive":
		if err := setupGoogleDrive(reader, cfg); err != nil {
			return err
		}
	case "synology":
		if err := setupSynology(reader, cfg); err != nil {
			return err
		}
	case "gist":
		if err := setupGist(reader, cfg); err != nil {
			return err
		}
	}

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\nSwitched to %s.\n", chosen)
	return nil
}
