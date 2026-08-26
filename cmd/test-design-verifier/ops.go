package main

import (
	"flag"
	"io"
)

// Task 7 swaps each ops entry's handler for the real implementation; this
// type and the registry shape must not change to make that possible.
type opHandler func(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError

type opSpec struct {
	newFlags func() *flag.FlagSet
	handler  opHandler
}

var ops = map[string]opSpec{
	"next-id": {
		newFlags: newNextIDFlags,
		handler:  nextIDHandler,
	},
	"preflight": {
		newFlags: newPreflightFlags,
		handler:  preflightHandler,
	},
}

func newNextIDFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("next-id", flag.ContinueOnError)
	fs.String("area", "", "coverage area for the generated ID")
	fs.String("repo", ".", "repository root")
	return fs
}

func newPreflightFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	fs.String("source", "", "coverage source markdown path")
	fs.String("repo", ".", "repository root")
	return fs
}
