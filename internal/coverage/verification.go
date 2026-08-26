package coverage

import (
	"encoding/json"
	"fmt"
	"strings"
)

type verificationFields struct {
	Runner               RunnerConfig
	BaselineEvidencePath string
	BaselineSHA256       string
	LiveProfile          string
	FinalCommand         []string
}

func parseBulletKV(line string) (key, value string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "- ") {
		return "", "", false
	}
	trimmed = strings.TrimPrefix(trimmed, "- ")
	idx := strings.Index(trimmed, ":")
	if idx < 0 {
		return "", "", false
	}
	return strings.TrimSpace(trimmed[:idx]), strings.TrimSpace(trimmed[idx+1:]), true
}

func requiredString(raw map[string]string, path, key string) (string, error) {
	value, ok := raw[key]
	if !ok {
		return "", fmt.Errorf("parse source %s: verification key %q: missing", path, key)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("parse source %s: verification key %q: empty", path, key)
	}
	return value, nil
}

func requiredStringArray(raw map[string]string, path, key string, allowEmpty bool) ([]string, error) {
	value, ok := raw[key]
	if !ok {
		return nil, fmt.Errorf("parse source %s: verification key %q: missing", path, key)
	}
	var arr []string
	if err := json.Unmarshal([]byte(value), &arr); err != nil {
		return nil, fmt.Errorf("parse source %s: verification key %q: not a JSON array of strings: %w", path, key, err)
	}
	if !allowEmpty && len(arr) == 0 {
		return nil, fmt.Errorf("parse source %s: verification key %q: empty array", path, key)
	}
	return arr, nil
}

func parseVerificationSection(path string, lines []string) (verificationFields, error) {
	section := findSection(lines, "## Verification")
	if section == nil {
		return verificationFields{}, fmt.Errorf("parse source %s: missing ## Verification section", path)
	}

	raw := make(map[string]string)
	for _, line := range section {
		if key, value, ok := parseBulletKV(line); ok {
			raw[key] = value
		}
	}

	var vf verificationFields
	var err error

	if vf.Runner.Kind, err = requiredString(raw, path, "runner-kind"); err != nil {
		return verificationFields{}, err
	}
	if vf.Runner.Root, err = requiredString(raw, path, "runner-root"); err != nil {
		return verificationFields{}, err
	}
	if vf.Runner.Command, err = requiredStringArray(raw, path, "runner-command", false); err != nil {
		return verificationFields{}, err
	}
	if vf.Runner.ReportGlobs, err = requiredStringArray(raw, path, "report-globs", true); err != nil {
		return verificationFields{}, err
	}
	if vf.BaselineEvidencePath, err = requiredString(raw, path, "baseline-evidence"); err != nil {
		return verificationFields{}, err
	}
	if vf.BaselineSHA256, err = requiredString(raw, path, "baseline-sha256"); err != nil {
		return verificationFields{}, err
	}
	if vf.FinalCommand, err = requiredStringArray(raw, path, "final-command", false); err != nil {
		return verificationFields{}, err
	}
	vf.LiveProfile = raw["live-profile"]

	return vf, nil
}
