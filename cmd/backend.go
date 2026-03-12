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
	Short: "Manage storage backends",
	Long:  "Select which storage backend to use for syncing files.\nLaunches the setup wizard when switching to a new backend.",
	Args:  cobra.NoArgs,
	RunE:  runBackend,
}

var backendListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured backends (active and backups)",
	Args:  cobra.NoArgs,
	RunE:  runBackendList,
}

var backendSwitchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Switch active backend, moving the old one to backup",
	Long:  "Promotes a configured backend to active and demotes the current active backend to a backup.",
	Args:  cobra.NoArgs,
	RunE:  runBackendSwitch,
}

func init() {
	backendCmd.AddCommand(backendListCmd, backendSwitchCmd)
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

// --- list ---

func runBackendList(_ *cobra.Command, _ []string) error {
	fmt.Println("Storage backends:")
	fmt.Println()

	activeStatus := "configured"
	if !cfg.IsBackendConfigured(cfg.ActiveBackend) {
		activeStatus = "credentials missing"
	}
	fmt.Printf("  ● %-14s  %s  (active)\n", cfg.ActiveBackend, activeStatus)

	if len(cfg.BackupBackends) == 0 {
		fmt.Println("\nNo backup backends. Use 'shync backup add' to add one.")
		return nil
	}

	fmt.Println()
	for _, name := range cfg.BackupBackends {
		status := "configured"
		if !cfg.IsBackendConfigured(name) {
			status = "credentials missing"
		}
		fmt.Printf("  ○ %-14s  %s  (backup)\n", name, status)
	}
	return nil
}

// --- switch ---

func runBackendSwitch(_ *cobra.Command, _ []string) error {
	// Offer every backend that has credentials, except the current active one.
	var candidates []string
	for _, b := range backends {
		if b == cfg.ActiveBackend {
			continue
		}
		if cfg.IsBackendConfigured(b) {
			candidates = append(candidates, b)
		}
	}
	if len(candidates) == 0 {
		fmt.Println("No other configured backends to switch to.")
		fmt.Println("Use 'shync backup add' to configure a new backend first.")
		return nil
	}

	sel := promptui.Select{
		Label: fmt.Sprintf("Switch active backend (currently: %s)", cfg.ActiveBackend),
		Items: candidates,
	}
	_, chosen, err := sel.Run()
	if err != nil {
		return nil // user cancelled
	}

	old := cfg.ActiveBackend
	cfg.ActiveBackend = chosen

	// Demote old active to backup (if not already listed).
	cfg.AddBackupBackend(old)

	// Remove new active from backup list (it's no longer a backup).
	cfg.RemoveBackupBackend(chosen)

	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("✓ Active backend: %s → %s\n", old, chosen)
	fmt.Printf("  %s added to backup backends.\n", old)
	return nil
}
