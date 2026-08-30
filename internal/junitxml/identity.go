package junitxml

import "regexp"

type Identity struct {
	ClassName   string
	Method      string // derived declared-method candidate
	Unsupported string // non-empty = reason identity cannot map to one class+method
}

var (
	gradleParamDecoration  = regexp.MustCompile(`^\[\d+\]`)
	gradleZeroArgMethod    = regexp.MustCompile(`^(.+)\(\)$`)
	mavenParamDecoration   = regexp.MustCompile(`^([A-Za-z_$][A-Za-z0-9_$]*)\(.*\)\[\d+\]$`)
	bareDeclaredIdentifier = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)
)

// DeriveIdentity reduces one testcase's raw name to a declared-method
// candidate, keyed to the decoration grammar actually observed in the
// captured fixtures (fixtures/reports/jvm/{maven-surefire,maven-failsafe,
// gradle-java,gradle-kotlin}) rather than documentation. c.RawName is read
// only; the Case is never mutated.
//
// Rules, checked in order against RawName:
//
//  1. A Gradle parameterized invocation, "[index] argsDisplay" (e.g.
//     ParamTest's "[2] 2"). This decoration never carries the declared
//     method name — oneFailing(int) and allPassing(int) both render
//     invocation 2 as "[2] 2" in the same class — so it is always
//     Unsupported.
//  2. A Gradle zero-arg method-source name, "name()" (e.g. "passingTest()",
//     or the Kotlin backtick name "passes with backticks and spaces()").
//     Gradle's JUnit5 XML writer appends this literal "()" call suffix only
//     for a reflected method-source display name, never for a substituted
//     @DisplayName — so the suffix is the signal that free text is really a
//     source identifier, and stripping it keeps backtick names (spaces
//     included) verbatim.
//  3. A Maven parameterized invocation, "method(argTypes)[index]" (e.g.
//     "oneFailing(int)[2]"). Maven's decoration does carry the method name,
//     so it strips to it directly.
//  4. A bare identifier with no parens or spaces (Maven's technical name in
//     every case, including under @DisplayName, since Maven's writer never
//     substitutes display text into the name attribute). Used verbatim.
//  5. Anything else — a Gradle @DisplayName substitution (free text, no
//     trailing "()", not bracket-prefixed) or any other phrased/dynamic
//     name — is Unsupported: identity is fail-closed, never guessed.
//
// Overload resolution (same declared method name reused across classes, or
// via inheritance) needs source knowledge and happens at join time, not here.
func DeriveIdentity(c Case) Identity {
	id := Identity{ClassName: c.ClassName}

	name := c.RawName
	switch {
	case gradleParamDecoration.MatchString(name):
		id.Unsupported = `gradle parameterized invocation decoration ("[index] ...") does not encode the declared method name`
	case gradleZeroArgMethod.MatchString(name):
		id.Method = gradleZeroArgMethod.FindStringSubmatch(name)[1]
	case mavenParamDecoration.MatchString(name):
		id.Method = mavenParamDecoration.FindStringSubmatch(name)[1]
	case bareDeclaredIdentifier.MatchString(name):
		id.Method = name
	default:
		id.Unsupported = "raw name is not a recognized declared-method shape (custom display name or dynamic test naming)"
	}

	return id
}
