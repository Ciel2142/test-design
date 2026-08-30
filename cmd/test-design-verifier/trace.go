package main

import (
	"flag"
	"io"
)

func traceHandler(fs *flag.FlagSet, requestBody []byte, stdout io.Writer) *cliError {
	return &cliError{Code: "not-implemented", Message: "trace: not implemented"}
}
