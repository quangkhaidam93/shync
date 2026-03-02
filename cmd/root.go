package cmd

import (
	"fmt"
	"os"

	"github.com/quangkhaidam93/shync/internal/config"
	"github.com/quangkhaidam93/shync/internal/storage"
	"github.com/quangkhaidam93/shync/internal/storage/gist"
	"github.com/quangkhaidam93/shync/internal/storage/googledrive"
	"github.com/quangkhaidam93/shync/internal/storage/synology"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "shync",
	Short: "Sync config files across devices",
	Long:  "shync uploads and downloads config files (.zshrc, .vimrc, etc.) to remote storage, enabling sharing settings between devices.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		switch cmd.Name() {
		case "init", "version", "update", "uninstall":
			return nil
		}
		var err error
		cfg, err = config.Load(cfgFile)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No config found. Run 'shync init' first.")
				os.Exit(1)
			}
			return fmt.Errorf("loading config: %w", err)
		}
		return nil
	},
}

func Execute() {
	rootCmd.Version = Version
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default ~/.config/shync/config.toml)")
}

func newBackend() (storage.Backend, error) {
	return newBackendWith(cfg)
}

func newBackendWith(c *config.Config) (storage.Backend, error) {
	switch c.ActiveBackend {
	case "google_drive":
		return googledrive.New(c)
	case "synology":
		return synology.New(c)
	case "gist":
		return gist.New(c)
	default:
		return nil, fmt.Errorf("unknown backend: %s", c.ActiveBackend)
	}
}
