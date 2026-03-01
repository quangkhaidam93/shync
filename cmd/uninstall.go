package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var uninstallForce bool

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove shync binary and all configuration data",
	RunE: func(cmd *cobra.Command, args []string) error {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locating executable: %w", err)
		}
		exe, err = filepath.EvalSymlinks(exe)
		if err != nil {
			return fmt.Errorf("resolving executable path: %w", err)
		}

		// Suggest brew uninstall for Homebrew installs.
		lower := strings.ToLower(exe)
		if strings.Contains(lower, "/homebrew/") || strings.Contains(lower, "/cellar/") {
			fmt.Println("It looks like shync was installed via Homebrew.")
			fmt.Println("Run: brew uninstall shync")
			fmt.Println("")
			fmt.Println("To also remove configuration data:")
			fmt.Printf("  rm -rf %s\n", configDir())
			return nil
		}

		configDir := configDir()

		fmt.Println("This will remove:")
		fmt.Printf("  Binary:  %s\n", exe)
		fmt.Printf("  Config:  %s\n", configDir)

		if !uninstallForce {
			fmt.Print("\nContinue? [y/N] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Println("Aborted.")
				return nil
			}
		}

		// Remove config directory.
		if err := os.RemoveAll(configDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove %s: %v\n", configDir, err)
		} else {
			fmt.Printf("Removed %s\n", configDir)
		}

		// Remove binary.
		if err := os.Remove(exe); err != nil {
			return fmt.Errorf("could not remove binary %s: %w\nYou may need to run: sudo rm %s", exe, err, exe)
		}
		fmt.Printf("Removed %s\n", exe)

		fmt.Println("\nshync has been uninstalled.")
		return nil
	},
}

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "shync")
}

func init() {
	uninstallCmd.Flags().BoolVarP(&uninstallForce, "yes", "y", false, "skip confirmation prompt")
	rootCmd.AddCommand(uninstallCmd)
}
