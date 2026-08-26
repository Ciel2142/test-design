package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"test-design/internal/evidence"
)

type requestProbe struct {
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

	fs := spec.newFlags()
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args[1:]); err != nil {
		writeError(stderr, &cliError{Code: "unknown-flag", Message: fmt.Sprintf("%s: %v", opName, err)})
		return exitError
	}

	requestBody, cliErr := readRequest(fs, stdin)
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

	if cliErr := spec.handler(fs, requestBody, stdout); cliErr != nil {
		writeError(stderr, cliErr)
		return exitError
	}
	return exitOK
}

// readRequest returns nil, nil unless --request names a document. stdin is
// never touched without it: a harness that hands the process an inherited
// pipe it never closes must not be able to block an operation.
func readRequest(fs *flag.FlagSet, stdin io.Reader) ([]byte, *cliError) {
	if !flagWasSet(fs, "request") {
		return nil, nil
	}
	path := flagString(fs, "request")
	if path == "" {
		return nil, &cliError{Code: "missing-request-path", Message: "read request: --request needs a path or -"}
	}

	if path == "-" {
		if stdin == nil {
			return nil, &cliError{Code: "request-read-failed", Message: "read request -: no stdin available"}
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, &cliError{Code: "request-read-failed", Message: fmt.Sprintf("read request -: %v", err)}
		}
		return data, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &cliError{Code: "request-read-failed", Message: fmt.Sprintf("read request %s: %v", path, err)}
	}
	return data, nil
}

func checkSchemaVersion(data []byte) *cliError {
	var probe requestProbe
	if err := json.Unmarshal(data, &probe); err != nil {
		return &cliError{Code: "malformed-request", Message: fmt.Sprintf("decode request: %v", err)}
	}
	if probe.SchemaVersion != evidence.SchemaVersion {
		return &cliError{
			Code:    "unsupported-schema-version",
			Message: fmt.Sprintf("unsupported schema version %q, supported %q", probe.SchemaVersion, evidence.SchemaVersion),
		}
	}
	return nil
}
