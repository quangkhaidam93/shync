package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/quangkhaidam93/shync/internal/fileutil"
	"github.com/spf13/cobra"
)

var viewRemote bool

var viewCmd = &cobra.Command{
	Use:   "view [file]",
	Short: "View a tracked file in read-only vim",
	Long:  "Open a tracked file in vim read-only mode.\nWith --remote, fetches and opens the remote version.\nWith no arguments, pick from tracked files and choose local or remote.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runView,
}

func init() {
	viewCmd.Flags().BoolVarP(&viewRemote, "remote", "r", false, "view the remote version instead of local")
	rootCmd.AddCommand(viewCmd)
}

func runView(cmd *cobra.Command, args []string) error {
	var remoteName string
	showRemote := viewRemote

	if len(args) == 1 {
		remoteName = args[0]
	} else {
		entry, err := pickTrackedFile("View which file?")
		if err != nil {
			return nil
		}
		remoteName = entry.RemoteName

		if !cmd.Flags().Changed("remote") {
			sel := promptui.Select{
				Label: "Which version?",
				Items: []string{"Local", "Remote"},
			}
			idx, _, err := sel.Run()
			if err != nil {
				return nil
			}
			showRemote = idx == 1
		}
	}

	entry := cfg.FindFileByRemoteName(remoteName)
	if entry == nil {
		return fmt.Errorf("file not tracked: %s\nRun 'shync list' to see tracked files", remoteName)
	}

	if showRemote {
		return viewRemoteFile(remoteName)
	}

	return viewLocalFile(entry.LocalPath, remoteName)
}

func viewLocalFile(localPath, remoteName string) error {
	expanded := fileutil.ExpandPath(localPath)
	if !fileutil.FileExists(expanded) {
		return fmt.Errorf("local file not found: %s", expanded)
	}

	spinResult := spin(fmt.Sprintf("Opening %s...", remoteName), func() error {
		return nil // local file already exists, just show spinner briefly
	})
	if spinResult != nil {
		return spinResult
	}

	return openReadOnly(expanded)
}

func viewRemoteFile(remoteName string) error {
	// Initialize backend before spinner so login/OTP prompts work correctly
	backend, err := newBackend()
	if err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}
	defer backend.Close()

	remotePath := cfg.RemoteDir + "/" + remoteName

	exists, err := backend.Exists(context.Background(), remotePath)
	if err != nil {
		return fmt.Errorf("checking remote file: %w", err)
	}
	if !exists {
		return fmt.Errorf("file not found on remote: %s", remoteName)
	}

	var tmpPath string

	err = spin(fmt.Sprintf("Downloading %s...", remoteName), func() error {
		tmpFile, err := os.CreateTemp("", "shync-view-*")
		if err != nil {
			return fmt.Errorf("creating temp file: %w", err)
		}
		tmpPath = tmpFile.Name()

		if err := backend.Download(context.Background(), remotePath, tmpFile); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("download failed: %w", err)
		}
		tmpFile.Close()
		return nil
	})
	if err != nil {
		return err
	}

	defer os.Remove(tmpPath)

	return openReadOnly(tmpPath)
}

// openReadOnly opens a file in vim read-only mode.
func openReadOnly(path string) error {
	c := exec.Command("vim", "-RM", path)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}

// spin runs fn in a background goroutine while showing a spinner.
// The spinner displays for at least 500ms to avoid flicker.
func spin(message string, fn func() error) error {
	ch := make(chan error, 1)
	go func() {
		ch <- fn()
	}()

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	start := time.Now()
	minDisplay := 500 * time.Millisecond
	i := 0
	var result error
	done := false

	for {
		select {
		case result = <-ch:
			ch = nil
			done = true
		default:
		}
		if done && time.Since(start) >= minDisplay {
			break
		}
		fmt.Printf("\r%s %s", frames[i%len(frames)], message)
		i++
		time.Sleep(80 * time.Millisecond)
	}
	fmt.Printf("\r\033[K") // clear spinner line

	return result
}
