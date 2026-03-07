package cmd

import (
	"fmt"
	"time"
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// runWithSpinner executes fn in a goroutine while showing an animated spinner
// with msg. The spinner is displayed for at least 500 ms even if fn finishes
// sooner, so the animation is always visible. The spinner line is erased
// before returning.
func runWithSpinner(msg string, fn func() error) error {
	ch := make(chan error, 1)
	go func() { ch <- fn() }()

	const minDisplay = 500 * time.Millisecond
	start := time.Now()
	frame := 0
	var result error
	done := false

	for {
		select {
		case result = <-ch:
			done = true
		default:
		}
		if done && time.Since(start) >= minDisplay {
			break
		}
		fmt.Printf("\r%s %s", spinnerFrames[frame%len(spinnerFrames)], msg)
		frame++
		time.Sleep(80 * time.Millisecond)
	}

	fmt.Printf("\r\033[K") // erase spinner line
	return result
}
