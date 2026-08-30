package junitxml

import (
	"testing"

	"test-design/internal/runrecord"
)

func TestDeriveIdentityMavenBareTechnicalNameIsDerivable(t *testing.T) {
	id := DeriveIdentity(Case{ClassName: "com.example.verifier.SimpleTest", RawName: "passingTest"})
	want := Identity{ClassName: "com.example.verifier.SimpleTest", Method: "passingTest"}
	if id != want {
		t.Errorf("DeriveIdentity = %+v, want %+v", id, want)
	}
}

func TestDeriveIdentityMavenCustomDisplayNameStillTechnicalIsDerivable(t *testing.T) {
	// maven-surefire/TEST-com.example.verifier.DisplayNameTest.xml: name="customDisplay" —
	// Maven's XML writer always uses the technical name, even under @DisplayName.
	id := DeriveIdentity(Case{ClassName: "com.example.verifier.DisplayNameTest", RawName: "customDisplay"})
	want := Identity{ClassName: "com.example.verifier.DisplayNameTest", Method: "customDisplay"}
	if id != want {
		t.Errorf("DeriveIdentity = %+v, want %+v", id, want)
	}
}

func TestDeriveIdentityMavenParameterizedDecorationStripsToDeclaredMethod(t *testing.T) {
	// maven-surefire/TEST-com.example.verifier.ParamTest.xml: name="oneFailing(int)[2]"
	id := DeriveIdentity(Case{ClassName: "com.example.verifier.ParamTest", RawName: "oneFailing(int)[2]"})
	want := Identity{ClassName: "com.example.verifier.ParamTest", Method: "oneFailing"}
	if id != want {
		t.Errorf("DeriveIdentity = %+v, want %+v", id, want)
	}

	id2 := DeriveIdentity(Case{ClassName: "com.example.verifier.ParamTest", RawName: "allPassing(int)[1]"})
	want2 := Identity{ClassName: "com.example.verifier.ParamTest", Method: "allPassing"}
	if id2 != want2 {
		t.Errorf("DeriveIdentity = %+v, want %+v", id2, want2)
	}
}

func TestDeriveIdentityGradleZeroArgMethodDecorationStripsTrailingParens(t *testing.T) {
	// gradle-java/TEST-com.example.verifier.SimpleTest.xml: name="passingTest()"
	id := DeriveIdentity(Case{ClassName: "com.example.verifier.SimpleTest", RawName: "passingTest()"})
	want := Identity{ClassName: "com.example.verifier.SimpleTest", Method: "passingTest"}
	if id != want {
		t.Errorf("DeriveIdentity = %+v, want %+v", id, want)
	}
}

func TestDeriveIdentityGradleNestedClassKeepsDollarFormAndStripsParens(t *testing.T) {
	// gradle-java/TEST-com.example.verifier.SimpleTest$NestedCases.xml: name="nestedPassingTest()"
	id := DeriveIdentity(Case{ClassName: "com.example.verifier.SimpleTest$NestedCases", RawName: "nestedPassingTest()"})
	want := Identity{ClassName: "com.example.verifier.SimpleTest$NestedCases", Method: "nestedPassingTest"}
	if id != want {
		t.Errorf("DeriveIdentity = %+v, want %+v", id, want)
	}
}

func TestDeriveIdentityGradleKotlinBacktickWithSpacesKeptVerbatim(t *testing.T) {
	// gradle-kotlin/TEST-com.example.verifier.BacktickTest.xml: name="passes with backticks and spaces()"
	id := DeriveIdentity(Case{ClassName: "com.example.verifier.BacktickTest", RawName: "passes with backticks and spaces()"})
	want := Identity{ClassName: "com.example.verifier.BacktickTest", Method: "passes with backticks and spaces"}
	if id != want {
		t.Errorf("DeriveIdentity = %+v, want %+v", id, want)
	}
}

func TestDeriveIdentityGradleKotlinPlainBacktickIsDerivable(t *testing.T) {
	// gradle-kotlin/TEST-com.example.verifier.BacktickTest.xml: name="alsoBackticked()"
	id := DeriveIdentity(Case{ClassName: "com.example.verifier.BacktickTest", RawName: "alsoBackticked()"})
	want := Identity{ClassName: "com.example.verifier.BacktickTest", Method: "alsoBackticked"}
	if id != want {
		t.Errorf("DeriveIdentity = %+v, want %+v", id, want)
	}
}

func TestDeriveIdentityGradleParameterizedDecorationIsUnsupported(t *testing.T) {
	// gradle-java/TEST-com.example.verifier.ParamTest.xml: name="[2] 2" — this exact
	// decoration is produced by both oneFailing(int) and allPassing(int) invocations
	// in the same class, so the raw name alone cannot resolve to one method.
	for _, raw := range []string{"[1] 1", "[2] 2", "[3] 3"} {
		id := DeriveIdentity(Case{ClassName: "com.example.verifier.ParamTest", RawName: raw})
		if id.Unsupported == "" {
			t.Errorf("DeriveIdentity(%q).Unsupported = %q, want non-empty", raw, id.Unsupported)
		}
		if id.Method != "" {
			t.Errorf("DeriveIdentity(%q).Method = %q, want empty when Unsupported", raw, id.Method)
		}
		if id.ClassName != "com.example.verifier.ParamTest" {
			t.Errorf("DeriveIdentity(%q).ClassName = %q, want passthrough", raw, id.ClassName)
		}
	}
}

func TestDeriveIdentityGradleCustomDisplayNameSubstitutionIsUnsupported(t *testing.T) {
	// gradle-java/TEST-com.example.verifier.DisplayNameTest.xml and
	// gradle-kotlin's equivalent: name="custom method display" — Gradle substitutes
	// the @DisplayName text, losing the source method identity.
	id := DeriveIdentity(Case{ClassName: "com.example.verifier.DisplayNameTest", RawName: "custom method display"})
	if id.Unsupported == "" {
		t.Errorf("DeriveIdentity.Unsupported = %q, want non-empty", id.Unsupported)
	}
	if id.Method != "" {
		t.Errorf("DeriveIdentity.Method = %q, want empty when Unsupported", id.Method)
	}
	if id.ClassName != "com.example.verifier.DisplayNameTest" {
		t.Errorf("DeriveIdentity.ClassName = %q, want passthrough", id.ClassName)
	}
}

func TestDeriveIdentitySameMethodNameDifferentClassesDeriveIndependently(t *testing.T) {
	// AlphaTest.sharedMethod / BetaTest.sharedMethod: identical method name in two
	// different classes; DeriveIdentity is per-Case and does not disambiguate
	// across classes (that is Merge's job at family-grouping time).
	a := DeriveIdentity(Case{ClassName: "com.example.verifier.AlphaTest", RawName: "sharedMethod"})
	b := DeriveIdentity(Case{ClassName: "com.example.verifier.BetaTest", RawName: "sharedMethod"})
	if a.Method != "sharedMethod" || b.Method != "sharedMethod" {
		t.Fatalf("Method = %q, %q, want both %q", a.Method, b.Method, "sharedMethod")
	}
	if a.ClassName == b.ClassName {
		t.Errorf("ClassName collapsed to the same value, want distinct classes preserved")
	}
}

func TestDeriveIdentityUnrecognizedShapeFailsClosed(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty raw name", ""},
		{"free text with punctuation", "should reject invalid input!"},
		{"dynamic test naming with digits and colons", "dynamic test #3: input=7"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := DeriveIdentity(Case{ClassName: "com.example.verifier.SimpleTest", RawName: tt.raw})
			if id.Unsupported == "" {
				t.Errorf("Unsupported = %q, want non-empty", id.Unsupported)
			}
			if id.Method != "" {
				t.Errorf("Method = %q, want empty when Unsupported", id.Method)
			}
		})
	}
}

func TestDeriveIdentityDoesNotMutateCaseRawName(t *testing.T) {
	c := Case{Status: runrecord.Passed, ClassName: "com.example.verifier.SimpleTest", RawName: "passingTest()"}
	before := c.RawName
	_ = DeriveIdentity(c)
	if c.RawName != before {
		t.Errorf("RawName mutated: got %q, want unchanged %q", c.RawName, before)
	}
}

func TestDeriveIdentityAgainstCapturedFixtures(t *testing.T) {
	tests := []struct {
		name string
		file []string
		raw  string
		want Identity
	}{
		{
			"maven-surefire ParamTest oneFailing[2]",
			[]string{"maven-surefire", "TEST-com.example.verifier.ParamTest.xml"},
			"oneFailing(int)[2]",
			Identity{ClassName: "com.example.verifier.ParamTest", Method: "oneFailing"},
		},
		{
			"maven-failsafe ParamIT allPassing[3]",
			[]string{"maven-failsafe", "TEST-com.example.verifier.ParamIT.xml"},
			"allPassing(int)[3]",
			Identity{ClassName: "com.example.verifier.ParamIT", Method: "allPassing"},
		},
		{
			"gradle-java SimpleTest passingTest()",
			[]string{"gradle-java", "TEST-com.example.verifier.SimpleTest.xml"},
			"passingTest()",
			Identity{ClassName: "com.example.verifier.SimpleTest", Method: "passingTest"},
		},
		{
			"gradle-kotlin BacktickTest backtick with spaces",
			[]string{"gradle-kotlin", "TEST-com.example.verifier.BacktickTest.xml"},
			"passes with backticks and spaces()",
			Identity{ClassName: "com.example.verifier.BacktickTest", Method: "passes with backticks and spaces"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := NormalizeFile(openFixture(t, tt.file...))
			if err != nil {
				t.Fatalf("NormalizeFile: %v", err)
			}
			var found *Case
			for i := range doc.Cases {
				if doc.Cases[i].RawName == tt.raw {
					found = &doc.Cases[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("fixture %v does not contain a case with RawName %q", tt.file, tt.raw)
			}
			got := DeriveIdentity(*found)
			if got != tt.want {
				t.Errorf("DeriveIdentity(%+v) = %+v, want %+v", *found, got, tt.want)
			}
		})
	}
}

func TestDeriveIdentityGradleParamTestFixtureUnsupported(t *testing.T) {
	doc, err := NormalizeFile(openFixture(t, "gradle-kotlin", "TEST-com.example.verifier.ParamTest.xml"))
	if err != nil {
		t.Fatalf("NormalizeFile: %v", err)
	}
	if len(doc.Cases) == 0 {
		t.Fatalf("fixture has no cases")
	}
	for _, c := range doc.Cases {
		id := DeriveIdentity(c)
		if id.Unsupported == "" {
			t.Errorf("DeriveIdentity(%+v).Unsupported = %q, want non-empty", c, id.Unsupported)
		}
	}
}
