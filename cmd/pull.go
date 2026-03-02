package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/quangkhaidam93/shync/internal/backup"
	"github.com/quangkhaidam93/shync/internal/fileutil"
	"github.com/quangkhaidam93/shync/internal/storage"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var pullCmd = &cobra.Command{
	Use:   "pull [file]",
	Short: "Download a file from remote storage",
	Long:  "Download a tracked file from remote storage. Shows a side-by-side diff preview and creates a backup of the existing local file first.\nWith no arguments, pick from tracked files.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runPull,
}

func init() {
	rootCmd.AddCommand(pullCmd)
}

func runPull(cmd *cobra.Command, args []string) error {
	var remoteName string
	if len(args) == 1 {
		remoteName = args[0]
	} else {
		picked, err := pickRemoteFile("Download which file?", func() ([]storage.FileMetadata, error) {
			b, err := newBackend()
			if err != nil {
				return nil, err
			}
			defer b.Close()
			return b.List(context.Background(), cfg.RemoteDir)
		})
		if err != nil {
			return nil
		}
		remoteName = picked
	}

	backend, err := newBackend()
	if err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}
	defer backend.Close()

	entry := cfg.FindFileByRemoteName(remoteName)
	if entry == nil {
		return fmt.Errorf("file not tracked: %s\nRun 'shync list' to see tracked files", remoteName)
	}

	localPath := fileutil.ExpandPath(entry.LocalPath)
	remotePath := cfg.RemoteDir + "/" + remoteName

	exists, err := backend.Exists(context.Background(), remotePath)
	if err != nil {
		return fmt.Errorf("checking remote file: %w", err)
	}
	if !exists {
		return fmt.Errorf("file not found on remote: %s", remoteName)
	}

	// Download to temp file first
	tmpFile, err := os.CreateTemp("", "shync-down-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	fmt.Printf("Downloading %s from %s (%s)...\n", entry.RemoteName, remotePath, backend.Name())
	if err := backend.Download(context.Background(), remotePath, tmpFile); err != nil {
		tmpFile.Close()
		return fmt.Errorf("download failed: %w", err)
	}
	tmpFile.Close()

	// If local file exists, show diff and ask for confirmation
	if fileutil.FileExists(localPath) {
		localLines, err := readLines(localPath)
		if err != nil {
			return fmt.Errorf("reading local file: %w", err)
		}
		remoteLines, err := readLines(tmpPath)
		if err != nil {
			return fmt.Errorf("reading remote file: %w", err)
		}

		diffs := computeDiff(localLines, remoteLines)
		if isIdentical(diffs) {
			fmt.Println("Files are identical. Nothing to do.")
			return nil
		}

		renderSideBySide(remoteName, "local", diffs)

		sel := promptui.Select{
			Label: "Apply changes?",
			Items: []string{"Yes", "No"},
		}
		_, choice, err := sel.Run()
		if err != nil || choice == "No" {
			fmt.Println("Aborted.")
			return nil
		}

		// Backup existing file
		backupPath, err := backup.Create(localPath)
		if err != nil {
			return fmt.Errorf("creating backup: %w", err)
		}
		if backupPath != "" {
			fmt.Printf("Backed up %s -> %s\n", localPath, backupPath)
		}
	}

	if err := copyFile(tmpPath, localPath); err != nil {
		return fmt.Errorf("writing local file: %w", err)
	}

	fmt.Println("Done.")
	return nil
}

// --- diff types ---

type diffOp int

const (
	opEqual  diffOp = iota // line is the same in both files
	opDelete               // line only in left (local)
	opInsert               // line only in right (remote)
	opChange               // line differs between left and right
)

type diffLine struct {
	op    diffOp
	left  string
	right string
}

// --- LCS-based line diff ---

func computeDiff(left, right []string) []diffLine {
	m, n := len(left), len(right)

	// Build LCS table
	dp := make([][]int, m+1)
	for i := range dp {
		dp[i] = make([]int, n+1)
	}
	for i := 1; i <= m; i++ {
		for j := 1; j <= n; j++ {
			if left[i-1] == right[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	// Backtrack to get raw edits (reversed)
	var raw []diffLine
	i, j := m, n
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && left[i-1] == right[j-1] {
			raw = append(raw, diffLine{opEqual, left[i-1], right[j-1]})
			i--
			j--
		} else if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			raw = append(raw, diffLine{opInsert, "", right[j-1]})
			j--
		} else {
			raw = append(raw, diffLine{opDelete, left[i-1], ""})
			i--
		}
	}

	// Reverse
	for l, r := 0, len(raw)-1; l < r; l, r = l+1, r-1 {
		raw[l], raw[r] = raw[r], raw[l]
	}

	// Merge adjacent delete+insert runs into paired change lines
	return mergeChanges(raw)
}

func mergeChanges(raw []diffLine) []diffLine {
	var result []diffLine
	i := 0
	for i < len(raw) {
		if raw[i].op != opDelete {
			result = append(result, raw[i])
			i++
			continue
		}

		// Collect consecutive deletes
		var dels []string
		for i < len(raw) && raw[i].op == opDelete {
			dels = append(dels, raw[i].left)
			i++
		}
		// Collect consecutive inserts that follow
		var ins []string
		for i < len(raw) && raw[i].op == opInsert {
			ins = append(ins, raw[i].right)
			i++
		}

		// Pair them side by side
		pairs := max(len(dels), len(ins))
		for j := 0; j < pairs; j++ {
			d := diffLine{}
			if j < len(dels) {
				d.left = dels[j]
			}
			if j < len(ins) {
				d.right = ins[j]
			}
			switch {
			case d.left != "" && d.right != "":
				d.op = opChange
			case d.left != "":
				d.op = opDelete
			default:
				d.op = opInsert
			}
			result = append(result, d)
		}
	}
	return result
}

func isIdentical(diffs []diffLine) bool {
	for _, d := range diffs {
		if d.op != opEqual {
			return false
		}
	}
	return true
}

// --- side-by-side rendering ---

const (
	colorRed   = "\033[31m"
	colorGreen = "\033[32m"
	colorCyan  = "\033[36m"
	colorDim   = "\033[2m"
	colorBold  = "\033[1m"
	colorReset = "\033[0m"
)

func renderSideBySide(filename, source string, diffs []diffLine) {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 40 {
		width = 120
	}

	// Layout:  "NNNN │ content ││ NNNN │ content"
	//           4 + 3 + content  3   4 + 3 + content
	// sideWidth = (width - 3) / 2   (3 for middle " │ ")
	sideWidth := (width - 3) / 2
	numWidth := 4
	contentWidth := sideWidth - numWidth - 3 // 3 for " │ "
	if contentWidth < 10 {
		contentWidth = 10
		sideWidth = contentWidth + numWidth + 3
	}

	divider := strings.Repeat("─", sideWidth)

	// Header
	fmt.Printf("\n%sDiff for %s (%s) (current -> incoming)%s\n\n",
		colorBold, filename, source, colorReset)
	fmt.Printf("%s%s%-*s%s │ %s%-*s%s\n",
		colorBold, colorRed, sideWidth, " -- current", colorReset,
		colorBold+colorGreen, sideWidth, "++ incoming", colorReset)
	fmt.Printf("%s┼%s\n", divider, divider)

	leftNum, rightNum := 0, 0
	for _, d := range diffs {
		switch d.op {
		case opEqual:
			leftNum++
			rightNum++
			l := formatSide(leftNum, "   ", d.left, numWidth, contentWidth)
			r := formatSide(rightNum, "   ", d.right, numWidth, contentWidth)
			fmt.Printf("%s%s%s │ %s%s%s\n",
				colorDim, l, colorReset,
				colorDim, r, colorReset)

		case opChange:
			leftNum++
			rightNum++
			l := formatSide(leftNum, "-- ", d.left, numWidth, contentWidth)
			r := formatSide(rightNum, "++ ", d.right, numWidth, contentWidth)
			fmt.Printf("%s%s%s │ %s%s%s\n",
				colorRed, l, colorReset,
				colorGreen, r, colorReset)

		case opDelete:
			leftNum++
			l := formatSide(leftNum, "-- ", d.left, numWidth, contentWidth)
			r := padSide(sideWidth)
			fmt.Printf("%s%s%s │ %s\n",
				colorRed, l, colorReset, r)

		case opInsert:
			rightNum++
			l := padSide(sideWidth)
			r := formatSide(rightNum, "++ ", d.right, numWidth, contentWidth)
			fmt.Printf("%s │ %s%s%s\n",
				l, colorGreen, r, colorReset)
		}
	}
}

func formatSide(num int, marker, text string, numWidth, contentWidth int) string {
	text = truncate(text, contentWidth-3) // 3 for marker prefix
	return fmt.Sprintf("%*d │ %s%-*s", numWidth, num, marker, contentWidth-3, text)
}

func padSide(width int) string {
	return strings.Repeat(" ", width)
}

func truncate(s string, maxWidth int) string {
	// Use rune count for display width (good enough for ASCII config files)
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return string(runes[:maxWidth])
	}
	return string(runes[:maxWidth-1]) + "…"
}

// --- file helpers ---

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := string(data)
	if s == "" {
		return nil, nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n"), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
