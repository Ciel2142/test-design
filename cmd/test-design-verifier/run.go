package main

import (
	"encoding/json"
	"fmt"
	"io"

	"test-design/internal/evidence"
)

type stdinProbe struct {
	SchemaVersion string `json:"schemaVersion"`
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		writeError(stderr, &cliError{Code: "unknown-operation", Message: "no operation specified"})
		return exitError
	}

	opName := args[0]
	spec, ok := ops[opName]
	if !ok {
		writeError(stderr, &cliError{Code: "unknown-operation", Message: fmt.Sprintf("unknown operation %q", opName)})
		return exitError
	}

	requestBody, cliErr := readStdinRequest(stdin)
	if cliErr != nil {
		writeError(stderr, cliErr)
		return exitError
	}
	if requestBody != nil {
		if cliErr := checkSchemaVersion(requestBody); cliErr != nil {
			writeError(stderr, cliErr)
			return exitError
		}
	}

	fs := spec.newFlags()
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args[1:]); err != nil {
		writeError(stderr, &cliError{Code: "unknown-flag", Message: fmt.Sprintf("%s: %v", opName, err)})
		return exitError
	}

	if cliErr := spec.handler(fs, requestBody, stdout); cliErr != nil {
		writeError(stderr, cliErr)
		return exitError
	}
	return exitOK
}

// readStdinRequest returns nil, nil when stdin carries no request document —
// a nil reader (not a pipe) or a pipe with zero bytes — per R2: with nothing
// to gate, no version check applies.
func readStdinRequest(stdin io.Reader) ([]byte, *cliError) {
	if stdin == nil {
		return nil, nil
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, &cliError{Code: "stdin-read-failed", Message: fmt.Sprintf("read stdin: %v", err)}
	}
	if len(data) == 0 {
		return nil, nil
	}
	return data, nil
}

func checkSchemaVersion(data []byte) *cliError {
	var probe stdinProbe
	if err := json.Unmarshal(data, &probe); err != nil {
		return &cliError{Code: "malformed-request", Message: fmt.Sprintf("decode stdin request: %v", err)}
	}
	if probe.SchemaVersion != evidence.SchemaVersion {
		return &cliError{
			Code:    "unsupported-schema-version",
			Message: fmt.Sprintf("unsupported schema version %q, supported %q", probe.SchemaVersion, evidence.SchemaVersion),
		}
	}
	return nil
}
