package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/quangkhaidam93/shync/internal/fileutil"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all tracked files with remote status",
	Args:  cobra.NoArgs,
	RunE:  runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

type fileStatus struct {
	LocalPath    string
	RemoteName   string
	LocalExists  bool
	RemoteExists bool
}

func runList(cmd *cobra.Command, args []string) error {
	if len(cfg.Files) == 0 {
		fmt.Println("No tracked files. Use 'shync add' or 'shync up <file>' to start tracking.")
		return nil
	}

	// Fetch remote status in background with spinner.
	type result struct {
		statuses []fileStatus
		err      error
	}
	ch := make(chan result, 1)
	go func() {
		backend, err := newBackend()
		if err != nil {
			ch <- result{err: err}
			return
		}
		defer backend.Close()
		var statuses []fileStatus
		for _, f := range cfg.Files {
			localExists := fileutil.FileExists(fileutil.ExpandPath(f.LocalPath))
			remotePath := cfg.RemoteDir + "/" + f.RemoteName
			remoteExists := false
			exists, err := backend.Exists(context.Background(), remotePath)
			if err == nil && exists {
				remoteExists = true
			}
			statuses = append(statuses, fileStatus{
				LocalPath:    f.LocalPath,
				RemoteName:   f.RemoteName,
				LocalExists:  localExists,
				RemoteExists: remoteExists,
			})
		}
		ch <- result{statuses: statuses}
	}()

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	start := time.Now()
	minDisplay := 500 * time.Millisecond
	i := 0
	var res result

	for {
		select {
		case res = <-ch:
			ch = nil
		default:
		}
		if ch == nil && time.Since(start) >= minDisplay {
			break
		}
		fmt.Printf("\r%s Fetching remote status...", frames[i%len(frames)])
		i++
		time.Sleep(80 * time.Millisecond)
	}
	fmt.Printf("\r\033[K")

	if res.err != nil {
		return fmt.Errorf("initializing backend: %w", res.err)
	}

	fmt.Printf("Tracked files (%d):\n\n", len(res.statuses))
	for i, s := range res.statuses {
		local := statusLabel(s.LocalExists)
		remote := statusLabel(s.RemoteExists)
		fmt.Printf("  %d. %s -> %s  (local: %s, remote: %s)\n", i, s.LocalPath, s.RemoteName, local, remote)
	}
	return nil
}

func statusLabel(exists bool) string {
	if exists {
		return "existed"
	}
	return "not existed"
}
