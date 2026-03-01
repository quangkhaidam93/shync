package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage shync configuration",
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config value",
	Long:  "Get a config value using dot notation (e.g., synology.host)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		val, err := cfg.Get(args[0])
		if err != nil {
			return err
		}
		fmt.Println(val)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a config value",
	Long:  "Set a config value using dot notation (e.g., shync config set synology.host 192.168.1.100)",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cfg.Set(args[0], args[1]); err != nil {
			return err
		}
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("%s = %s\n", args[0], args[1])
		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all config values",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		kvs := cfg.ListAll()
		for _, kv := range kvs {
			fmt.Printf("%s = %s\n", kv.Key, kv.Value)
		}
		return nil
	},
}

var configRemoveCmd = &cobra.Command{
	Use:   "remove <key>",
	Short: "Remove a config key (reset to zero value)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cfg.Remove(args[0]); err != nil {
			return err
		}
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("Removed %s\n", args[0])
		return nil
	},
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configRemoveCmd)
	rootCmd.AddCommand(configCmd)
}
