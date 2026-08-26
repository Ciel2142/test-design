package main

import (
	"encoding/json"
	"io"
)

const (
	exitOK    = 0
	exitError = 1
)

type cliError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *cliError) Error() string { return e.Message }

func writeError(w io.Writer, err *cliError) {
	// Encode error discarded on purpose: a two-string struct cannot fail to
	// marshal, and there is no second channel left to report a stderr failure on.
	json.NewEncoder(w).Encode(err)
}
