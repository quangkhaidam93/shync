package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/quangkhaidam93/shync/internal/fileutil"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check sync status of all tracked files",
	Long:  "Compare local and remote file contents for all tracked files to see what has changed.",
	Args:  cobra.NoArgs,
	RunE:  runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

type checkResult struct {
	remoteName  string
	localExists bool
	remoteExists bool
	modified    bool
}

func runCheck(cmd *cobra.Command, args []string) error {
	if len(cfg.Files) == 0 {
		fmt.Println("No tracked files. Use 'shync add' or 'shync up <file>' to start tracking.")
		return nil
	}

	// Initialize backend before spinner so login/OTP prompts work correctly
	backend, err := newBackend()
	if err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}
	defer backend.Close()

	var results []checkResult

	err = spin("Checking files...", func() error {
		ctx := context.Background()
		for _, f := range cfg.Files {
			expanded := fileutil.ExpandPath(f.LocalPath)
			localExists := fileutil.FileExists(expanded)
			remotePath := cfg.RemoteDir + "/" + f.RemoteName

			remoteExists, err := backend.Exists(ctx, remotePath)
			if err != nil {
				remoteExists = false
			}

			cr := checkResult{
				remoteName:   f.RemoteName,
				localExists:  localExists,
				remoteExists: remoteExists,
			}

			if localExists && remoteExists {
				localData, err := os.ReadFile(expanded)
				if err != nil {
					return fmt.Errorf("reading local file %s: %w", f.RemoteName, err)
				}

				var remoteBuf bytes.Buffer
				if err := backend.Download(ctx, remotePath, &remoteBuf); err != nil {
					return fmt.Errorf("downloading %s: %w", f.RemoteName, err)
				}

				cr.modified = !bytes.Equal(localData, remoteBuf.Bytes())
			}

			results = append(results, cr)
		}
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Printf("Checking %d tracked files...\n\n", len(results))

	for _, r := range results {
		switch {
		case !r.localExists && !r.remoteExists:
			fmt.Printf("  %-14s ✗ missing\n", r.remoteName)
		case r.localExists && !r.remoteExists:
			fmt.Printf("  %-14s ✗ local only\n", r.remoteName)
		case !r.localExists && r.remoteExists:
			fmt.Printf("  %-14s ✗ remote only\n", r.remoteName)
		case r.modified:
			fmt.Printf("  %-14s ✗ modified\n", r.remoteName)
		default:
			fmt.Printf("  %-14s ✓ no changes\n", r.remoteName)
		}
	}

	return nil
}
