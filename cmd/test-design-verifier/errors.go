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
	json.NewEncoder(w).Encode(err)
}
