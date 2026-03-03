package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/quangkhaidam93/shync/internal/config"
	"github.com/quangkhaidam93/shync/internal/storage"
)

type renameAction struct {
	oldRemote string
	newRemote string
	entry     *config.FileEntry
}

// findRemoteVariants returns all remote file names equal to baseName or ending
// with "-baseName" (parent-prefixed variants created by conflict resolution).
func findRemoteVariants(ctx context.Context, backend storage.Backend, baseName string) ([]string, error) {
	files, err := backend.List(ctx, cfg.RemoteDir)
	if err != nil {
		return nil, err
	}
	suffix := "-" + baseName
	var variants []string
	for _, f := range files {
		if f.Name == baseName || strings.HasSuffix(f.Name, suffix) {
			variants = append(variants, f.Name)
		}
	}
	return variants, nil
}

// askSameFile downloads a remote variant, shows a diff against the local file,
// and asks whether the local file is the same config file as the remote variant.
// noLabel is the text shown for the "no" choice.
// Returns the remote name to use if user says "yes", or "" if "no".
func askSameFile(localPath, variantName, noLabel string, backend storage.Backend, ctx context.Context) (string, error) {
	var tmpPath string
	err := spin(fmt.Sprintf("Fetching %s for comparison...", variantName), func() error {
		tmpFile, innerErr := os.CreateTemp("", "shync-cmp-*")
		if innerErr != nil {
			return fmt.Errorf("creating temp file: %w", innerErr)
		}
		tmpPath = tmpFile.Name()
		remotePath := cfg.RemoteDir + "/" + variantName
		if dlErr := backend.Download(ctx, remotePath, tmpFile); dlErr != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			tmpPath = ""
			return fmt.Errorf("downloading remote %s: %w", variantName, dlErr)
		}
		tmpFile.Close()
		return nil
	})
	if err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)

	localLines, err := readLines(localPath)
	if err != nil {
		return "", fmt.Errorf("reading local file: %w", err)
	}
	remoteLines, err := readLines(tmpPath)
	if err != nil {
		return "", fmt.Errorf("reading remote file: %w", err)
	}

	diffs := computeDiff(localLines, remoteLines)
	if isIdentical(diffs) {
		fmt.Printf("\n  Local file is identical to remote %s\n", variantName)
	} else {
		renderDiffHunks(diffs, variantName)
	}

	sel := promptui.Select{
		Label: fmt.Sprintf("Is your local file the same config as remote %q?", variantName),
		Items: []string{
			fmt.Sprintf("Yes, same file as remote %s", variantName),
			noLabel,
		},
	}
	idx, _, selErr := sel.Run()
	if selErr != nil {
		return "", fmt.Errorf("prompt cancelled: %w", selErr)
	}
	if idx == 0 {
		return variantName, nil
	}
	return "", nil
}

// resolveRemoteName determines the remote name for a file being added/uploaded.
// When the plain basename has remote variants (from other machines), it shows
// diffs and asks the user if their file is the same as each variant. If yes,
// that remote name is reused. If no to all, the parent-prefixed name is assigned.
// For local conflicts (same machine, different tracked path), it auto-renames.
func resolveRemoteName(absPath string, backend storage.Backend) (string, []renameAction, error) {
	baseName := filepath.Base(absPath)
	parentPrefix := filepath.Base(filepath.Dir(absPath)) + "-" + baseName

	ctx := context.Background()

	// Check if baseName conflicts with a locally tracked file.
	localConflict := cfg.FindFileByRemoteName(baseName)
	// Ignore if the conflict is the same file being re-added.
	if localConflict != nil {
		expanded := expandIfNeeded(localConflict.LocalPath)
		localConflictAbs, _ := filepath.Abs(expanded)
		if localConflictAbs == absPath {
			localConflict = nil
		}
	}

	// No local conflict — look for remote variants and ask the user.
	if localConflict == nil {
		variants, err := findRemoteVariants(ctx, backend, baseName)
		if err != nil {
			return "", nil, fmt.Errorf("listing remote files: %w", err)
		}
		if len(variants) == 0 {
			return baseName, nil, nil
		}

		// Check whether parentPrefix is already among the remote variants.
		parentPrefixTaken := false
		for _, v := range variants {
			if v == parentPrefix {
				parentPrefixTaken = true
				break
			}
		}

		for i, variant := range variants {
			isLast := i == len(variants)-1
			var noLabel string
			switch {
			case isLast && !parentPrefixTaken:
				noLabel = fmt.Sprintf("No, create %s on remote", parentPrefix)
			case isLast && parentPrefixTaken:
				noLabel = "No, none of these are the same file"
			default:
				noLabel = "No, check next"
			}

			chosen, askErr := askSameFile(absPath, variant, noLabel, backend, ctx)
			if askErr != nil {
				return "", nil, askErr
			}
			if chosen != "" {
				return chosen, nil, nil
			}
		}

		// User said "no" to every variant.
		if parentPrefixTaken {
			// parentPrefix is also taken — prompt for a custom name.
			name, err := promptCustomName(baseName)
			if err != nil {
				return "", nil, err
			}
			return name, nil, nil
		}
		return parentPrefix, nil, nil
	}

	// Local conflict detected — try parentPrefix.
	// Check parentPrefix against local tracked files.
	if existing := cfg.FindFileByRemoteName(parentPrefix); existing != nil {
		name, err := promptCustomName(baseName)
		if err != nil {
			return "", nil, err
		}
		return name, nil, nil
	}
	// Check parentPrefix against remote.
	if exists, err := backend.Exists(ctx, cfg.RemoteDir+"/"+parentPrefix); err == nil && exists {
		if cfg.FindFileByRemoteName(parentPrefix) == nil {
			name, err := promptCustomName(baseName)
			if err != nil {
				return "", nil, err
			}
			return name, nil, nil
		}
	}

	// parentPrefix is available — rename the conflicting local entry.
	existingAbs, _ := filepath.Abs(expandIfNeeded(localConflict.LocalPath))
	existingParentPrefix := filepath.Base(filepath.Dir(existingAbs)) + "-" + baseName
	renames := []renameAction{{
		oldRemote: baseName,
		newRemote: existingParentPrefix,
		entry:     localConflict,
	}}

	return parentPrefix, renames, nil
}

// applyRenames downloads, re-uploads, and deletes remote files to rename them,
// then updates the config entries.
func applyRenames(backend storage.Backend, renames []renameAction) error {
	ctx := context.Background()

	for _, r := range renames {
		oldPath := cfg.RemoteDir + "/" + r.oldRemote
		newPath := cfg.RemoteDir + "/" + r.newRemote

		fmt.Printf("  Renaming remote %s → %s\n", r.oldRemote, r.newRemote)

		err := spin(fmt.Sprintf("Renaming %s...", r.oldRemote), func() error {
			// Download to temp file.
			tmpFile, err := os.CreateTemp("", "shync-rename-*")
			if err != nil {
				return fmt.Errorf("creating temp file: %w", err)
			}
			tmpPath := tmpFile.Name()
			defer os.Remove(tmpPath)

			if err := backend.Download(ctx, oldPath, tmpFile); err != nil {
				tmpFile.Close()
				return fmt.Errorf("downloading %s: %w", r.oldRemote, err)
			}
			tmpFile.Close()

			// Re-open for upload.
			src, err := os.Open(tmpPath)
			if err != nil {
				return fmt.Errorf("opening temp file: %w", err)
			}
			defer src.Close()

			if err := backend.Upload(ctx, newPath, src, r.newRemote); err != nil {
				return fmt.Errorf("uploading %s: %w", r.newRemote, err)
			}

			// Delete old remote file.
			if err := backend.Delete(ctx, oldPath); err != nil {
				return fmt.Errorf("deleting %s: %w", r.oldRemote, err)
			}

			return nil
		})
		if err != nil {
			return err
		}

		// Update config entry.
		r.entry.RemoteName = r.newRemote
	}

	if len(renames) > 0 {
		if err := cfg.Save(); err != nil {
			return fmt.Errorf("saving config after rename: %w", err)
		}
	}

	return nil
}

// promptCustomName asks the user for a custom remote name when both the
// plain basename and parent-prefixed name conflict.
func promptCustomName(baseName string) (string, error) {
	prompt := promptui.Prompt{
		Label:   fmt.Sprintf("Both '%s' and its prefixed variant conflict. Enter a custom remote name", baseName),
		Default: baseName,
		Validate: func(input string) error {
			if input == "" {
				return fmt.Errorf("name cannot be empty")
			}
			if filepath.Base(input) != input {
				return fmt.Errorf("name must not contain path separators")
			}
			return nil
		},
	}
	result, err := prompt.Run()
	if err != nil {
		return "", fmt.Errorf("prompt cancelled: %w", err)
	}
	return result, nil
}

// expandIfNeeded expands ~ in a path if present.
func expandIfNeeded(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}
