package main

import (
	"io"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:], stdinIfPiped(), os.Stdout, os.Stderr))
}

// A read on os.Stdin from an interactive terminal blocks until input
// arrives; only treat stdin as a request source when it is actually piped.
func stdinIfPiped() io.Reader {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return nil
	}
	return os.Stdin
}
