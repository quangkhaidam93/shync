package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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

	filename := filepath.Base(absPath)
	remotePath := cfg.RemoteDir + "/" + filename

	// If remote file exists, show diff and ask for confirmation.
	exists, err := backend.Exists(context.Background(), remotePath)
	if err != nil {
		return fmt.Errorf("checking remote file: %w", err)
	}
	if exists {
		tmpFile, err := os.CreateTemp("", "shync-up-*")
		if err != nil {
			return fmt.Errorf("creating temp file: %w", err)
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if err := backend.Download(context.Background(), remotePath, tmpFile); err != nil {
			tmpFile.Close()
			return fmt.Errorf("downloading remote for diff: %w", err)
		}
		tmpFile.Close()

		remoteLines, err := readLines(tmpPath)
		if err != nil {
			return fmt.Errorf("reading remote file: %w", err)
		}
		localLines, err := readLines(absPath)
		if err != nil {
			return fmt.Errorf("reading local file: %w", err)
		}

		diffs := computeDiff(remoteLines, localLines)
		if isIdentical(diffs) {
			fmt.Println("Files are identical. Nothing to upload.")
			return nil
		}

		renderSideBySide(filename, "remote", diffs)

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\nUpload changes? [y/N]: ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
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
