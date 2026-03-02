package cmd

import (
	"fmt"

	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
)

var supportedCmd = &cobra.Command{
	Use:   "supported",
	Short: "Manage supported file patterns",
	Long:  "Add or remove filename patterns that 'shync add' will look for\nwhen browsing your filesystem.",
}

var supportedAddCmd = &cobra.Command{
	Use:   "add <pattern>",
	Short: "Add a supported file pattern",
	Long:  "Add a filename pattern (e.g., .vimrc, config.toml) to the list of\nfiles that 'shync add' will recognize when browsing.",
	Args:  cobra.ExactArgs(1),
	RunE:  runSupportedAdd,
}

var supportedRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove a supported file pattern",
	Long:  "Pick a pattern from the current list and remove it.",
	Args:  cobra.NoArgs,
	RunE:  runSupportedRemove,
}

var supportedListCmd = &cobra.Command{
	Use:   "list",
	Short: "List supported file patterns",
	Args:  cobra.NoArgs,
	RunE:  runSupportedList,
}

func init() {
	supportedCmd.AddCommand(supportedAddCmd)
	supportedCmd.AddCommand(supportedRemoveCmd)
	supportedCmd.AddCommand(supportedListCmd)
	rootCmd.AddCommand(supportedCmd)
}

func runSupportedAdd(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	for _, p := range cfg.SupportedFiles {
		if p == pattern {
			fmt.Printf("Pattern %q is already in the list.\n", pattern)
			return nil
		}
	}

	cfg.SupportedFiles = append(cfg.SupportedFiles, pattern)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Added %q to supported files.\n", pattern)
	return nil
}

func runSupportedRemove(cmd *cobra.Command, args []string) error {
	if len(cfg.SupportedFiles) == 0 {
		fmt.Println("No supported file patterns configured.")
		return nil
	}

	sel := promptui.Select{
		Label: "Remove which pattern?",
		Items: cfg.SupportedFiles,
	}
	idx, _, err := sel.Run()
	if err != nil {
		return nil
	}

	removed := cfg.SupportedFiles[idx]
	cfg.SupportedFiles = append(cfg.SupportedFiles[:idx], cfg.SupportedFiles[idx+1:]...)
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Removed %q from supported files.\n", removed)
	return nil
}

func runSupportedList(cmd *cobra.Command, args []string) error {
	if len(cfg.SupportedFiles) == 0 {
		fmt.Println("No supported file patterns configured.")
		return nil
	}
	for _, p := range cfg.SupportedFiles {
		fmt.Printf("  %s\n", p)
	}
	return nil
}
