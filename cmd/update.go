package cmd

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const releaseURL = "https://api.github.com/repos/quangkhaidam93/shync/releases/latest"

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update shync to the latest version",
	RunE: func(cmd *cobra.Command, args []string) error {
		exe, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locating executable: %w", err)
		}

		// Suggest brew upgrade for Homebrew installs.
		lower := strings.ToLower(exe)
		if strings.Contains(lower, "/homebrew/") || strings.Contains(lower, "/cellar/") {
			fmt.Println("It looks like shync was installed via Homebrew.")
			fmt.Println("Run: brew upgrade shync")
			return nil
		}

		// Fetch latest release.
		resp, err := http.Get(releaseURL)
		if err != nil {
			return fmt.Errorf("fetching latest release: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			fmt.Println("No releases found yet.")
			return nil
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("GitHub API returned %s", resp.Status)
		}

		var release ghRelease
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			return fmt.Errorf("parsing release: %w", err)
		}

		latest := strings.TrimPrefix(release.TagName, "v")
		current := strings.TrimPrefix(Version, "v")
		if current == latest {
			fmt.Printf("Already up to date (%s).\n", Version)
			return nil
		}

		// Find the right asset.
		archiveName := fmt.Sprintf("shync_%s_%s_%s.tar.gz", latest, runtime.GOOS, runtime.GOARCH)
		var downloadURL string
		for _, a := range release.Assets {
			if a.Name == archiveName {
				downloadURL = a.BrowserDownloadURL
				break
			}
		}
		if downloadURL == "" {
			return fmt.Errorf("no asset found for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, release.TagName)
		}

		fmt.Printf("Updating %s -> %s ...\n", Version, release.TagName)

		// Download archive.
		dlResp, err := http.Get(downloadURL)
		if err != nil {
			return fmt.Errorf("downloading release: %w", err)
		}
		defer dlResp.Body.Close()
		if dlResp.StatusCode != http.StatusOK {
			return fmt.Errorf("download returned %s", dlResp.Status)
		}

		// Extract binary from tar.gz.
		bin, err := extractBinary(dlResp.Body)
		if err != nil {
			return fmt.Errorf("extracting binary: %w", err)
		}

		// Write to a temp file next to the current executable, then rename.
		tmpPath := exe + ".tmp"
		if err := os.WriteFile(tmpPath, bin, 0o755); err != nil {
			return fmt.Errorf("writing temp file: %w", err)
		}

		if err := os.Rename(tmpPath, exe); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("replacing executable: %w", err)
		}

		fmt.Printf("Updated to %s.\n", release.TagName)
		return nil
	},
}

// extractBinary reads a tar.gz stream and returns the contents of the "shync" binary.
func extractBinary(r io.Reader) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg && (hdr.Name == "shync" || strings.HasSuffix(hdr.Name, "/shync")) {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("shync binary not found in archive")
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
