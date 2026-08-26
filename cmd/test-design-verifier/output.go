package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

func writeStdout(stdout io.Writer, doc any) *cliError {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		return &cliError{Code: "encode-output-failed", Message: fmt.Sprintf("encode output: %v", err)}
	}
	if _, err := stdout.Write(buf.Bytes()); err != nil {
		return &cliError{Code: "write-output-failed", Message: fmt.Sprintf("write output: %v", err)}
	}
	return nil
}
