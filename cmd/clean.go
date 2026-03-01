package cmd

import (
	"fmt"

	"github.com/quangkhaidam93/shync/internal/backup"
	"github.com/quangkhaidam93/shync/internal/config"
	"github.com/spf13/cobra"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Remove stale backups",
	Long:  "Delete backup files older than the configured backup_expiry (default 3mo).",
	RunE:  runClean,
}

func init() {
	rootCmd.AddCommand(cleanCmd)
}

func runClean(cmd *cobra.Command, args []string) error {
	expiryStr := cfg.BackupExpiry
	if expiryStr == "" {
		expiryStr = "3mo"
	}

	expiry, err := config.ParseExpiry(expiryStr)
	if err != nil {
		return fmt.Errorf("invalid backup_expiry %q: %w", expiryStr, err)
	}

	removed, err := backup.Clean(expiry)
	if err != nil {
		return fmt.Errorf("cleaning backups: %w", err)
	}

	if len(removed) == 0 {
		fmt.Println("No stale backups found.")
		return nil
	}

	for _, name := range removed {
		fmt.Printf("  removed %s\n", name)
	}
	fmt.Printf("Cleaned %d backup(s).\n", len(removed))
	return nil
}
