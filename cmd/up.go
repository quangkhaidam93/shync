package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/manifoldco/promptui"
	"github.com/quangkhaidam93/shync/internal/fileutil"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up [file]",
	Short: "Upload a file to remote storage",
	Long:  "Upload a local file to remote storage and track it in the config.\nWith no arguments, pick from tracked files.\nShows a diff preview when the remote file already exists.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runUp,
}

func init() {
	rootCmd.AddCommand(upCmd)
}

func runUp(cmd *cobra.Command, args []string) error {
	var localPath string
	if len(args) == 1 {
		localPath = fileutil.ExpandPath(args[0])
	} else {
		picked, err := pickTrackedFile("Upload which file?")
		if err != nil {
			return nil
		}
		localPath = fileutil.ExpandPath(picked.LocalPath)
	}

	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	if !fileutil.FileExists(absPath) {
		return fmt.Errorf("file not found: %s", absPath)
	}

	backend, err := newBackend()
	if err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}
	defer backend.Close()

	filename := filepath.Base(absPath)
	remotePath := cfg.RemoteDir + "/" + filename

	// If remote file exists, show diff and ask for confirmation.
	exists, err := backend.Exists(context.Background(), remotePath)
	if err != nil {
		return fmt.Errorf("checking remote file: %w", err)
	}
	if exists {
		showedDiff := false

		tmpFile, err := os.CreateTemp("", "shync-up-*")
		if err == nil {
			tmpPath := tmpFile.Name()
			defer os.Remove(tmpPath)

			if dlErr := backend.Download(context.Background(), remotePath, tmpFile); dlErr == nil {
				tmpFile.Close()
				remoteLines, _ := readLines(tmpPath)
				localLines, _ := readLines(absPath)

				diffs := computeDiff(remoteLines, localLines)
				if isIdentical(diffs) {
					fmt.Println("Files are identical. Nothing to upload.")
					return nil
				}

				renderSideBySide(filename, "remote", diffs)
				showedDiff = true
			} else {
				tmpFile.Close()
			}
		}

		if !showedDiff {
			fmt.Printf("Remote file %s already exists (diff unavailable).\n", filename)
		}

		sel := promptui.Select{
			Label: "Upload changes?",
			Items: []string{"Yes", "No"},
		}
		_, choice, err := sel.Run()
		if err != nil || choice == "No" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	f, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	// Upload with spinner animation.
	uploadErr := make(chan error, 1)
	go func() {
		uploadErr <- backend.Upload(context.Background(), remotePath, f, filename)
	}()

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	start := time.Now()
	minDisplay := 500 * time.Millisecond
	i := 0
	var uploadResult error
	done := false

	for {
		select {
		case uploadResult = <-uploadErr:
			done = true
		default:
		}
		if done && time.Since(start) >= minDisplay {
			break
		}
		fmt.Printf("\r%s Uploading %s...", frames[i%len(frames)], filename)
		i++
		time.Sleep(80 * time.Millisecond)
	}
	fmt.Printf("\r\033[K")

	if uploadResult != nil {
		return fmt.Errorf("upload failed: %w", uploadResult)
	}

	fmt.Printf("\u2713 Uploaded %s\n", filename)

	// Track in config
	displayPath := fileutil.ContractPath(absPath)
	if cfg.AddFile(displayPath, filename) {
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}
		fmt.Printf("Tracked: %s -> %s\n", displayPath, filename)
	}

	fmt.Println("Done.")
	return nil
}

// uploadFile handles the upload + spinner for a single file. Used by both
// runUp and runAdd to share the same animation logic.
func uploadFile(absPath string) error {
	backend, err := newBackend()
	if err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}
	defer backend.Close()

	filename := filepath.Base(absPath)
	remotePath := cfg.RemoteDir + "/" + filename

	f, err := os.Open(absPath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	uploadErr := make(chan error, 1)
	go func() {
		uploadErr <- backend.Upload(context.Background(), remotePath, io.Reader(f), filename)
	}()

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	start := time.Now()
	minDisplay := 500 * time.Millisecond
	i := 0
	var uploadResult error
	done := false

	for {
		select {
		case uploadResult = <-uploadErr:
			done = true
		default:
		}
		if done && time.Since(start) >= minDisplay {
			break
		}
		fmt.Printf("\r%s Uploading %s...", frames[i%len(frames)], filename)
		i++
		time.Sleep(80 * time.Millisecond)
	}
	fmt.Printf("\r\033[K")

	if uploadResult != nil {
		return fmt.Errorf("upload failed: %w", uploadResult)
	}

	fmt.Printf("\u2713 Uploaded %s\n", filename)
	return nil
}
