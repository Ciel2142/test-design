package live

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	decisiveContextLines = 2
	decisiveTailCap      = 4096
)

// ReducedStep is the durable transcript record for one executed step: just
// enough to audit the run without ever carrying a full raw capture or a
// non-allowlisted env value.
type ReducedStep struct {
	Expectation string   `json:"expectation"`
	Command     []string `json:"command"`
	ExitStatus  int      `json:"exitStatus"`
	Decisive    string   `json:"decisiveOutput"`
	Result      string   `json:"result"`
}

var (
	secretArgPattern  = regexp.MustCompile(`(?i)^(-{0,2}[A-Za-z0-9_.-]*(token|secret|password|passwd|api[_-]?key|credential)[A-Za-z0-9_.-]*)=.*$`)
	secretFlagPattern = regexp.MustCompile(`(?i)^-{1,2}[A-Za-z0-9_.-]*(token|secret|password|passwd|api[_-]?key|credential)[A-Za-z0-9_.-]*$`)
)

// ReduceTranscript evaluates step's predicates against res and produces the
// durable, redacted record of the run. It refuses to return a step whose
// decisive output contains the value of any parent env var not on the
// allowlist — the redaction barrier — naming only the variable, never the
// value, in that case.
func ReduceTranscript(step Step, res *StepResult, parentEnv []string, allowlist []string) (*ReducedStep, error) {
	pass, failed := EvaluatePredicates(step.Predicates, res)
	decisive, err := decisiveOutput(step.Predicates, failed, res)
	if err != nil {
		return nil, err
	}
	command := redactCommand(step.Command)

	// redactCommand misses secrets in plain argv slots (e.g. `mysql -p <value>`);
	// barrier must scan the command too, not just decisive.
	if err := redactionBarrier(decisive+"\n"+strings.Join(command, " "), parentEnv, allowlist); err != nil {
		return nil, err
	}

	result := "pass"
	if !pass {
		result = "fail"
	}

	return &ReducedStep{
		Expectation: step.Expectation,
		Command:     command,
		ExitStatus:  res.ExitStatus,
		Decisive:    decisive,
		Result:      result,
	}, nil
}

// decisiveOutput picks the evidence a reviewer needs to see: the first
// violated predicate's evidence when the step failed, else the first
// predicate's evidence as a representative pass sample, else a bounded tail
// when the step carries no predicates at all.
func decisiveOutput(preds []Predicate, failed []Predicate, res *StepResult) (string, error) {
	if len(failed) > 0 {
		return decisiveOutputFor(failed[0], res)
	}
	if len(preds) > 0 {
		return decisiveOutputFor(preds[0], res)
	}
	data, err := streamBytes("combined", res)
	return boundedTail(data, decisiveTailCap), err
}

func decisiveOutputFor(pred Predicate, res *StepResult) (string, error) {
	data, err := streamBytes(pred.Stream, res)
	if err != nil {
		return "", err
	}
	if pred.Form != "matches" {
		return boundedTail(data, decisiveTailCap), nil
	}
	if excerpt, ok := matchWithContext(data, pred.Value); ok {
		return excerpt, nil
	}
	return boundedTail(data, decisiveTailCap), nil
}

func matchWithContext(data []byte, pattern string) (string, bool) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", false
	}
	loc := re.FindIndex(data)
	if loc == nil {
		return "", false
	}

	text := string(data)
	matchLine := strings.Count(text[:loc[0]], "\n")
	lines := strings.Split(text, "\n")

	start := matchLine - decisiveContextLines
	if start < 0 {
		start = 0
	}
	end := matchLine + decisiveContextLines
	if end >= len(lines) {
		end = len(lines) - 1
	}

	excerpt := strings.Join(lines[start:end+1], "\n")
	return boundedTailString(excerpt, decisiveTailCap), true
}

func boundedTail(data []byte, limit int) string {
	return boundedTailString(string(data), limit)
}

func boundedTailString(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[len(s)-limit:]
}

// redactionBarrier fails closed when decisive contains the value of any
// parent env var not on the allowlist. Values shorter than 6 characters are
// skipped: a value that short (e.g. a port number) collides with ordinary
// output too often to be a meaningful leak signal.
func redactionBarrier(decisive string, parentEnv []string, allowlist []string) error {
	allowed := make(map[string]bool, len(allowlist))
	for _, name := range allowlist {
		allowed[name] = true
	}

	for _, kv := range parentEnv {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || allowed[name] {
			continue
		}
		if len(value) < 6 {
			continue
		}
		if strings.Contains(decisive, value) {
			return fmt.Errorf("reduce transcript: redaction barrier: non-allowlisted env var %s value present in decisive output", name)
		}
	}
	return nil
}

// redactCommand mirrors cmd/test-design-verifier's redactCommand: the
// reduced transcript must not carry a credential a step inlined into an
// argument. Reimplemented locally since that helper lives in package main.
func redactCommand(command []string) []string {
	redacted := make([]string, len(command))
	for i := 0; i < len(command); i++ {
		arg := command[i]
		if m := secretArgPattern.FindStringSubmatch(arg); m != nil {
			redacted[i] = m[1] + "=REDACTED"
			continue
		}
		redacted[i] = arg
		if secretFlagPattern.MatchString(arg) && i+1 < len(command) {
			i++
			redacted[i] = "REDACTED"
		}
	}
	return redacted
}
