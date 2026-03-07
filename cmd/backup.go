package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var allBackends = []string{"google_drive", "synology", "gist"}

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Manage backup storage backends",
	Long:  "Configure secondary backends to mirror your remote files, and sync all files from the active backend to every configured backup.",
}

var backupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured backup backends",
	Args:  cobra.NoArgs,
	RunE:  runBackupList,
}

var backupAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a backup backend",
	Args:  cobra.NoArgs,
	RunE:  runBackupAdd,
}

var backupRemoveCmd = &cobra.Command{
	Use:     "remove",
	Aliases: []string{"rm"},
	Short:   "Remove a backup backend",
	Args:    cobra.NoArgs,
	RunE:    runBackupRemove,
}

var backupSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync all remote files from active backend to all backup backends",
	Args:  cobra.NoArgs,
	RunE:  runBackupSync,
}

func init() {
	backupCmd.AddCommand(backupListCmd, backupAddCmd, backupRemoveCmd, backupSyncCmd)
	rootCmd.AddCommand(backupCmd)
}

// --- list ---

func runBackupList(_ *cobra.Command, _ []string) error {
	if len(cfg.BackupBackends) == 0 {
		fmt.Println("No backup backends configured. Use 'shync backup add' to add one.")
		return nil
	}
	fmt.Printf("Backup backends (%d):\n\n", len(cfg.BackupBackends))
	for i, name := range cfg.BackupBackends {
		status := "credentials missing"
		if cfg.IsBackendConfigured(name) {
			status = "configured"
		}
		fmt.Printf("  %d. %-14s  %s\n", i+1, name, status)
	}
	return nil
}

// --- add ---

func runBackupAdd(_ *cobra.Command, _ []string) error {
	// Build list of backends that can still be added.
	var available []string
	for _, b := range allBackends {
		if b == cfg.ActiveBackend {
			continue
		}
		if cfg.HasBackupBackend(b) {
			continue
		}
		available = append(available, b)
	}
	if len(available) == 0 {
		fmt.Println("All available backends are already configured (active or backup).")
		return nil
	}

	sel := promptui.Select{
		Label: "Select backend to add as backup",
		Items: available,
	}
	_, chosen, err := sel.Run()
	if err != nil {
		return nil
	}

	reader := bufio.NewReader(os.Stdin)

	// Offer to reuse existing credentials if already configured.
	if cfg.IsBackendConfigured(chosen) {
		reuse := promptui.Select{
			Label: fmt.Sprintf("Credentials for %s are already configured. Use them?", chosen),
			Items: []string{"Yes, use existing credentials", "No, reconfigure"},
		}
		_, choice, err := reuse.Run()
		if err != nil {
			return nil
		}
		if choice == "Yes, use existing credentials" {
			return saveBackupBackend(chosen)
		}
	}

	// Run the appropriate credential setup wizard.
	switch chosen {
	case "google_drive":
		if err := setupGoogleDrive(reader, cfg); err != nil {
			return fmt.Errorf("Google Drive setup: %w", err)
		}
	case "synology":
		if err := setupSynology(reader, cfg); err != nil {
			return fmt.Errorf("Synology setup: %w", err)
		}
	case "gist":
		if err := setupGist(reader, cfg); err != nil {
			return fmt.Errorf("Gist setup: %w", err)
		}
	}

	// Test connection.
	fmt.Printf("\nVerifying connection to %s...\n", chosen)
	b, err := newBackendByName(chosen)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	b.Close()
	fmt.Println("Connection successful.")

	return saveBackupBackend(chosen)
}

func saveBackupBackend(name string) error {
	cfg.AddBackupBackend(name)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Added %s as a backup backend.\n", name)
	return nil
}

// --- remove ---

func runBackupRemove(_ *cobra.Command, _ []string) error {
	if len(cfg.BackupBackends) == 0 {
		fmt.Println("No backup backends to remove.")
		return nil
	}

	sel := promptui.Select{
		Label: "Select backup backend to remove",
		Items: cfg.BackupBackends,
	}
	_, chosen, err := sel.Run()
	if err != nil {
		return nil
	}

	confirm := promptui.Prompt{
		Label:     fmt.Sprintf("Remove %s from backup backends", chosen),
		IsConfirm: true,
	}
	if _, err := confirm.Run(); err != nil {
		fmt.Println("Aborted.")
		return nil
	}

	cfg.RemoveBackupBackend(chosen)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Removed %s from backup backends.\n", chosen)
	return nil
}

// --- sync ---

func runBackupSync(_ *cobra.Command, _ []string) error {
	if len(cfg.BackupBackends) == 0 {
		fmt.Println("No backup backends configured. Use 'shync backup add' first.")
		return nil
	}

	// List all files on the active backend.
	var files []string
	if err := runWithSpinner(fmt.Sprintf("Fetching file list from %s...", cfg.ActiveBackend), func() error {
		active, err := newBackend()
		if err != nil {
			return fmt.Errorf("initializing active backend: %w", err)
		}
		defer active.Close()
		meta, err := active.List(context.Background(), cfg.RemoteDir)
		if err != nil {
			return fmt.Errorf("listing remote files: %w", err)
		}
		for _, m := range meta {
			files = append(files, m.Name)
		}
		return nil
	}); err != nil {
		return err
	}

	if len(files) == 0 {
		fmt.Println("No files found on the active backend.")
		return nil
	}
	fmt.Printf("Found %d file(s) on %s.\n\n", len(files), cfg.ActiveBackend)

	totalErrors := 0
	for _, backupName := range cfg.BackupBackends {
		fmt.Printf("Syncing to %s:\n", backupName)
		synced, errs := syncToBackend(backupName, files)
		if errs > 0 {
			fmt.Printf("  %d/%d file(s) synced (%d error(s))\n\n", synced, len(files), errs)
		} else {
			fmt.Printf("  ✓ All %d file(s) backed up.\n\n", synced)
		}
		totalErrors += errs
	}

	if totalErrors > 0 {
		return fmt.Errorf("sync completed with %d error(s)", totalErrors)
	}
	return nil
}

// syncToBackend copies every named file from the active backend to dst.
// Returns the number of files successfully synced and the error count.
func syncToBackend(dst string, files []string) (synced, errCount int) {
	active, err := newBackend()
	if err != nil {
		fmt.Printf("  Error initializing active backend: %v\n", err)
		return 0, len(files)
	}
	defer active.Close()

	backup, err := newBackendByName(dst)
	if err != nil {
		fmt.Printf("  Error initializing %s: %v\n", dst, err)
		return 0, len(files)
	}
	defer backup.Close()

	total := len(files)
	for i, name := range files {
		remotePath := cfg.RemoteDir + "/" + name
		label := fmt.Sprintf("Syncing %s → %s (%d/%d)...", name, dst, i+1, total)

		var syncErr error
		_ = runWithSpinner(label, func() error {
			var buf bytes.Buffer
			if err := active.Download(context.Background(), remotePath, &buf); err != nil {
				syncErr = fmt.Errorf("download: %w", err)
				return nil // don't surface via spinner; handle below
			}
			data := buf.Bytes()
			if err := backup.Upload(context.Background(), remotePath, bytes.NewReader(data), name); err != nil {
				syncErr = fmt.Errorf("upload: %w", err)
			}
			return nil
		})

		if syncErr != nil {
			fmt.Printf("  ✗ %s: %v\n", name, syncErr)
			errCount++
		} else {
			fmt.Printf("  ✓ %s\n", name)
			synced++
		}
	}
	return
}
