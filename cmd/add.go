package cmd

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/quangkhaidam93/shync/internal/fileutil"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Pick files to track from supported file patterns",
	Long:  "Browse your filesystem for files matching supported_files patterns\nand add them to tracking. Use 'shync push' to upload tracked files.",
	Args:  cobra.NoArgs,
	RunE:  runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	selected, err := pickMultiFiles("Select files to track:", cfg.SupportedFiles)
	if err != nil {
		if err == errAborted {
			return nil
		}
		return err
	}

	backend, err := newBackend()
	if err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}
	defer backend.Close()

	var added []string
	for _, p := range selected {
		localPath := fileutil.ExpandPath(p)
		absPath, err := filepath.Abs(localPath)
		if err != nil {
			fmt.Printf("Skipping %s: %v\n", p, err)
			continue
		}

		filename, renames, err := resolveRemoteName(absPath, backend)
		if err != nil {
			fmt.Printf("Skipping %s: %v\n", p, err)
			continue
		}
		if len(renames) > 0 {
			if err := applyRenames(backend, renames); err != nil {
				fmt.Printf("Skipping %s: rename failed: %v\n", p, err)
				continue
			}
		}

		displayPath := fileutil.ContractPath(absPath)

		// Check remote status.
		remotePath := cfg.RemoteDir + "/" + filename
		remoteStatus := "not existed"
		exists, err := backend.Exists(context.Background(), remotePath)
		if err == nil && exists {
			remoteStatus = "existed"
		}

		cfg.AddFile(displayPath, filename)
		added = append(added, displayPath)

		fmt.Printf("  %s  local: existed  remote: %s\n", displayPath, remoteStatus)
	}

	if len(added) > 0 {
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("\nTracked %d file(s). Use 'shync push' to upload.\n", len(added))
	}

	return nil
}
