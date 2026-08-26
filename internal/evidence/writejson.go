package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// WriteJSON writes doc to path as canonical JSON: struct fields in
// declaration order, map keys sorted, no HTML escaping. Two calls with
// equal input produce byte-identical files.
func WriteJSON(path string, doc any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("write json %s: %w", path, err)
	}

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write json %s: %w", path, err)
	}
	return nil
}
