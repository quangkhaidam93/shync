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

var pushCmd = &cobra.Command{
	Use:   "push [file]...",
	Short: "Upload file(s) to remote storage",
	Long:  "Upload one or more local files to remote storage and track them in the config.\nWith no arguments, select from tracked files (space to multi-select, enter to submit).\nShows a diff preview when remote files already exist.",
	Args:  cobra.ArbitraryArgs,
	RunE:  runPush,
}

func init() {
	rootCmd.AddCommand(pushCmd)
}

func runPush(cmd *cobra.Command, args []string) error {
	var filesToPush []string

	if len(args) > 0 {
		filesToPush = args
	} else {
		// Interactive: let user pick from tracked files (multi-select)
		picked, err := pickTrackedFileMulti("Select file(s) to upload")
		if err != nil {
			return nil
		}
		for _, entry := range picked {
			filesToPush = append(filesToPush, entry.LocalPath)
		}
	}

	if len(filesToPush) == 0 {
		fmt.Println("No files selected.")
		return nil
	}

	backend, err := newBackend()
	if err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}
	defer backend.Close()

	var uploadErrors []string
	for _, localPath := range filesToPush {
		localPath = fileutil.ExpandPath(localPath)
		absPath, err := filepath.Abs(localPath)
		if err != nil {
			uploadErrors = append(uploadErrors, fmt.Sprintf("%s: resolving path: %v", localPath, err))
			continue
		}

		if !fileutil.FileExists(absPath) {
			uploadErrors = append(uploadErrors, fmt.Sprintf("%s: file not found", absPath))
			continue
		}

		// Use existing remote name if already tracked, otherwise resolve conflicts.
		displayPath := fileutil.ContractPath(absPath)
		var filename string
		if entry := cfg.FindFileByLocalPath(displayPath); entry != nil {
			filename = entry.RemoteName

			// Case 2: if other machines have added *-baseName variants on remote,
			// offer to rename this file for clarity.
			baseName := filepath.Base(absPath)
			if filename == baseName {
				parentPrefix := filepath.Base(filepath.Dir(absPath)) + "-" + baseName
				variants, listErr := findRemoteVariants(context.Background(), backend, baseName)
				if listErr == nil && len(variants) > 1 {
					prefixTaken := false
					for _, v := range variants {
						if v == parentPrefix {
							prefixTaken = true
							break
						}
					}
					if !prefixTaken && len(filesToPush) == 1 {
						fmt.Printf("\n  Other machines have added their own %q variants on remote.\n", baseName)
						renameSel := promptui.Select{
							Label: "Would you like to rename yours for clarity?",
							Items: []string{
								fmt.Sprintf("Rename remote %s → %s", baseName, parentPrefix),
								fmt.Sprintf("Keep as %s", baseName),
							},
						}
						renameIdx, _, renameErr := renameSel.Run()
						if renameErr == nil && renameIdx == 0 {
							if applyErr := applyRenames(backend, []renameAction{{
								oldRemote: baseName,
								newRemote: parentPrefix,
								entry:     entry,
							}}); applyErr == nil {
								filename = entry.RemoteName
							}
						}
					}
				}
			}
		} else {
			var renames []renameAction
			var resolveErr error
			filename, renames, resolveErr = resolveRemoteName(absPath, backend)
			if resolveErr != nil {
				uploadErrors = append(uploadErrors, fmt.Sprintf("%s: resolving remote name: %v", localPath, resolveErr))
				continue
			}
			if len(renames) > 0 {
				if err := applyRenames(backend, renames); err != nil {
					uploadErrors = append(uploadErrors, fmt.Sprintf("%s: renaming conflicting files: %v", localPath, err))
					continue
				}
			}
		}
		remotePath := cfg.RemoteDir + "/" + filename

		// If remote file exists and only one file being pushed, show diff and ask for confirmation.
		exists, err := backend.Exists(context.Background(), remotePath)
		if err != nil {
			uploadErrors = append(uploadErrors, fmt.Sprintf("%s: checking remote file: %v", filename, err))
			continue
		}
		if exists && len(filesToPush) == 1 {
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
				fmt.Printf("Remote file %s does not exist. This will be a new upload.\n", filename)
			}

			sel := promptui.Select{
				Label: "Overwrite remote file?",
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
			uploadErrors = append(uploadErrors, fmt.Sprintf("%s: opening file: %v", filename, err))
			continue
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
			uploadErrors = append(uploadErrors, fmt.Sprintf("%s: upload failed: %v", filename, uploadResult))
			continue
		}

		fmt.Printf("✓ Uploaded %s\n", filename)

		// Track in config
		if cfg.AddFile(displayPath, filename) {
			if err := cfg.Save(); err != nil {
				uploadErrors = append(uploadErrors, fmt.Sprintf("%s: saving config: %v", filename, err))
				continue
			}
			fmt.Printf("Tracked: %s -> %s\n", displayPath, filename)
		}
	}

	if len(uploadErrors) > 0 {
		fmt.Println("\nErrors encountered:")
		for _, errMsg := range uploadErrors {
			fmt.Printf("  ✗ %s\n", errMsg)
		}
		if len(uploadErrors) < len(filesToPush) {
			fmt.Println("Done (with errors).")
		}
		return fmt.Errorf("%d file(s) failed to upload", len(uploadErrors))
	}

	if len(filesToPush) > 1 {
		fmt.Printf("\n✓ All %d file(s) uploaded successfully.\n", len(filesToPush))
	} else {
		fmt.Println("Done.")
	}
	return nil
}

// uploadFile handles the upload + spinner for a single file. Used by both
// runPush and runAdd to share the same animation logic.
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
