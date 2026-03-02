package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/manifoldco/promptui"
	"github.com/quangkhaidam93/shync/internal/config"
	"github.com/quangkhaidam93/shync/internal/storage"
)

type renameAction struct {
	oldRemote string
	newRemote string
	entry     *config.FileEntry
}

// resolveRemoteName determines the remote name for a file being added/uploaded.
// If the plain basename conflicts with an existing tracked file or remote file,
// it prefixes with the parent directory name. If the existing tracked file uses
// the plain basename, a rename action is returned so it can be updated too.
func resolveRemoteName(absPath string, backend storage.Backend) (string, []renameAction, error) {
	baseName := filepath.Base(absPath)
	parentPrefix := filepath.Base(filepath.Dir(absPath)) + "-" + baseName

	ctx := context.Background()

	// Check if baseName conflicts with a locally tracked file.
	localConflict := cfg.FindFileByRemoteName(baseName)
	// Ignore if the conflict is the same file being re-added.
	if localConflict != nil {
		localConflictAbs, _ := filepath.Abs(localConflict.LocalPath)
		// LocalPath may use ~ so expand it first.
		expanded := expandIfNeeded(localConflict.LocalPath)
		localConflictAbs, _ = filepath.Abs(expanded)
		if localConflictAbs == absPath {
			localConflict = nil
		}
	}

	// Check if baseName exists on remote (from another device or previous upload).
	remoteConflict := false
	if localConflict == nil {
		exists, err := backend.Exists(ctx, cfg.RemoteDir+"/"+baseName)
		if err == nil && exists {
			// Only a conflict if this file isn't already tracked with this remote name.
			remoteConflict = true
		}
	}

	// No conflict — use plain basename.
	if localConflict == nil && !remoteConflict {
		return baseName, nil, nil
	}

	// Conflict detected — try parentPrefix.
	// Check parentPrefix against local tracked files.
	if existing := cfg.FindFileByRemoteName(parentPrefix); existing != nil {
		// parentPrefix also conflicts with a tracked file — prompt user.
		name, err := promptCustomName(baseName)
		if err != nil {
			return "", nil, err
		}
		return name, nil, nil
	}
	// Check parentPrefix against remote.
	if exists, err := backend.Exists(ctx, cfg.RemoteDir+"/"+parentPrefix); err == nil && exists {
		// Check if the remote file is tracked locally (could be from init on another device).
		if cfg.FindFileByRemoteName(parentPrefix) == nil {
			// parentPrefix exists on remote but not tracked locally — prompt user.
			name, err := promptCustomName(baseName)
			if err != nil {
				return "", nil, err
			}
			return name, nil, nil
		}
	}

	// parentPrefix is available.
	var renames []renameAction

	// If an existing tracked file uses baseName, it needs to be renamed.
	if localConflict != nil {
		existingAbs, _ := filepath.Abs(expandIfNeeded(localConflict.LocalPath))
		existingParentPrefix := filepath.Base(filepath.Dir(existingAbs)) + "-" + baseName
		renames = append(renames, renameAction{
			oldRemote: baseName,
			newRemote: existingParentPrefix,
			entry:     localConflict,
		})
	}

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
