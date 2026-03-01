package cmd

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

var (
	// Set via ldflags at build time.
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of shync",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("shync %s\n", Version)
		fmt.Printf("  commit: %s\n", Commit)
		fmt.Printf("  built:  %s\n", Date)
		if info, ok := debug.ReadBuildInfo(); ok {
			fmt.Printf("  go:     %s\n", info.GoVersion)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
