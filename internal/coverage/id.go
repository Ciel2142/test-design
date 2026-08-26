package coverage

import "regexp"

var idPattern = regexp.MustCompile(`^([A-Za-z0-9._-]+/)?SC-[0-9]+$`)

var validLevels = map[string]bool{
	"unit":        true,
	"component":   true,
	"integration": true,
	"smoke":       true,
	"e2e":         true,
}

var validRisks = map[string]bool{
	"low":  true,
	"med":  true,
	"high": true,
}
