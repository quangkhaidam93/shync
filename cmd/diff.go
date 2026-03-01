package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/quangkhaidam93/shync/internal/fileutil"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var diffCmd = &cobra.Command{
	Use:   "diff [file]",
	Short: "Show differences between local and remote versions",
	Long:  "Compare a tracked file's local and remote contents side by side.\nWith no arguments, pick from tracked files.\nOnly shows changed areas with surrounding context.",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDiff,
}

func init() {
	rootCmd.AddCommand(diffCmd)
}

const diffContextLines = 3

func runDiff(cmd *cobra.Command, args []string) error {
	var remoteName, localPath string

	if len(args) == 1 {
		remoteName = args[0]
		entry := cfg.FindFileByRemoteName(remoteName)
		if entry == nil {
			return fmt.Errorf("file not tracked: %s\nRun 'shync list' to see tracked files", remoteName)
		}
		localPath = entry.LocalPath
	} else {
		entry, err := pickTrackedFile("Diff which file?")
		if err != nil {
			return nil
		}
		remoteName = entry.RemoteName
		localPath = entry.LocalPath
	}

	expanded := fileutil.ExpandPath(localPath)
	if !fileutil.FileExists(expanded) {
		return fmt.Errorf("local file not found: %s", expanded)
	}

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
		tmpFile, err := os.CreateTemp("", "shync-diff-*")
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

	localLines, err := readLines(expanded)
	if err != nil {
		return fmt.Errorf("reading local file: %w", err)
	}
	remoteLines, err := readLines(tmpPath)
	if err != nil {
		return fmt.Errorf("reading remote file: %w", err)
	}

	diffs := computeDiff(localLines, remoteLines)
	if isIdentical(diffs) {
		fmt.Printf("  %s: no differences\n", remoteName)
		return nil
	}

	renderDiffHunks(diffs, remoteName)
	return nil
}

// hunkRange defines a contiguous region of the diff to display.
type hunkRange struct {
	start, end int
}

// groupDiffHunks groups changed lines into hunk ranges with surrounding context.
func groupDiffHunks(diffs []diffLine) []hunkRange {
	var changed []int
	for i, d := range diffs {
		if d.op != opEqual {
			changed = append(changed, i)
		}
	}
	if len(changed) == 0 {
		return nil
	}

	clamp := func(v, lo, hi int) int {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}

	var hunks []hunkRange
	start := clamp(changed[0]-diffContextLines, 0, len(diffs))
	end := clamp(changed[0]+diffContextLines+1, 0, len(diffs))

	for _, idx := range changed[1:] {
		ns := clamp(idx-diffContextLines, 0, len(diffs))
		ne := clamp(idx+diffContextLines+1, 0, len(diffs))

		if ns <= end {
			end = ne
		} else {
			hunks = append(hunks, hunkRange{start, end})
			start = ns
			end = ne
		}
	}
	hunks = append(hunks, hunkRange{start, end})
	return hunks
}

const colorBlue = "\033[34m"

// renderDiffHunks renders only the changed areas as separate side-by-side blocks.
// Layout: local (left) │ remote (right)
func renderDiffHunks(diffs []diffLine, name string) {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 40 {
		width = 80
	}

	numWidth := 4 // width for line number column
	// Layout: "  " + num + " " + content + " │ " + num + " " + content
	// Total gutter = 2 + numWidth + 1 + 3 + numWidth + 1 = 2*numWidth + 7
	contentWidth := (width - 2*numWidth - 7) / 2
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Pre-compute left/right line numbers for each diff entry.
	// In computeDiff: left = local, right = remote.
	leftNums := make([]int, len(diffs))
	rightNums := make([]int, len(diffs))
	ln, rn := 0, 0
	for i, d := range diffs {
		switch d.op {
		case opEqual, opChange:
			ln++
			rn++
		case opDelete:
			ln++
		case opInsert:
			rn++
		}
		leftNums[i] = ln
		rightNums[i] = rn
	}

	hunks := groupDiffHunks(diffs)

	fmt.Println()
	for _, h := range hunks {
		// Find the first changed line in this hunk for the header line number.
		hunkLine := leftNums[h.start]
		for i := h.start; i < h.end; i++ {
			if diffs[i].op != opEqual {
				hunkLine = leftNums[i]
				break
			}
		}

		header := fmt.Sprintf("── %s:%d ", name, hunkLine)
		pad := width - len(header)
		if pad > 0 {
			header += strings.Repeat("─", pad)
		}
		fmt.Printf("%s%s%s\n", colorDim, header, colorReset)

		// Column labels: local left, remote right
		sideWidth := numWidth + 1 + contentWidth
		fmt.Printf("  %s%-*s%s %s│%s %s%-*s%s\n",
			colorCyan, sideWidth, "local", colorReset,
			colorDim, colorReset,
			colorBlue, sideWidth, "remote", colorReset,
		)

		for i := h.start; i < h.end; i++ {
			d := diffs[i]
			// d.left = local, d.right = remote
			switch d.op {
			case opEqual:
				l := fitColumn(d.left, contentWidth)
				r := fitColumn(d.right, contentWidth)
				fmt.Printf("  %s%*d %s%s %s│%s %s%*d %s%s\n",
					colorDim, numWidth, leftNums[i], l, colorReset,
					colorDim, colorReset,
					colorDim, numWidth, rightNums[i], r, colorReset)

			case opChange:
				l := fitColumn(d.left, contentWidth)
				r := fitColumn(d.right, contentWidth)
				fmt.Printf("  %s%*d %s%s %s│%s %s%*d %s%s\n",
					colorCyan, numWidth, leftNums[i], l, colorReset,
					colorDim, colorReset,
					colorBlue, numWidth, rightNums[i], r, colorReset)

			case opDelete:
				// Exists locally but not on remote
				l := fitColumn(d.left, contentWidth)
				blank := fitColumn("", contentWidth)
				fmt.Printf("  %s%*d %s%s %s│%s %*s %s\n",
					colorCyan, numWidth, leftNums[i], l, colorReset,
					colorDim, colorReset,
					numWidth, "", blank)

			case opInsert:
				// Exists on remote but not locally
				blank := fitColumn("", contentWidth)
				r := fitColumn(d.right, contentWidth)
				fmt.Printf("  %*s %s %s│%s %s%*d %s%s\n",
					numWidth, "", blank,
					colorDim, colorReset,
					colorBlue, numWidth, rightNums[i], r, colorReset)
			}
		}
		fmt.Println()
	}
}

// fitColumn pads or truncates a string to exactly width characters.
func fitColumn(s string, width int) string {
	n := len(s)
	if n > width {
		if width > 1 {
			return s[:width-1] + "…"
		}
		return s[:width]
	}
	return s + strings.Repeat(" ", width-n)
}
