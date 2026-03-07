//go:build !windows

package cmd

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// pasteToShellPrompt injects cmd into the calling terminal's input buffer via
// TIOCSTI, so the command appears on the shell prompt ready to run or edit.
// Falls back to printing the command if TIOCSTI is unavailable (e.g. Linux
// kernels ≥6.2 with CONFIG_LEGACY_TIOCSTI=n).
func pasteToShellPrompt(cmd string) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		printFallback(cmd)
		return
	}
	defer tty.Close()

	for i := 0; i < len(cmd); i++ {
		b := cmd[i]
		_, _, errno := unix.Syscall(unix.SYS_IOCTL, tty.Fd(), unix.TIOCSTI, uintptr(unsafe.Pointer(&b)))
		if errno != 0 {
			// TIOCSTI not permitted — fall back gracefully.
			printFallback(cmd)
			return
		}
	}
}

func printFallback(cmd string) {
	fmt.Fprintf(os.Stderr, "Note: could not inject into shell prompt (TIOCSTI unavailable).\n")
	fmt.Println(cmd)
}
