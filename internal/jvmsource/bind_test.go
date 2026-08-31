package jvmsource

import (
	"strings"
	"testing"
)

func bindAll(t *testing.T, src string, lang Lang) []tagBinding {
	t.Helper()
	toks, err := Lex(src, lang)
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	bs, err := bindTags(toks, lang)
	if err != nil {
		t.Fatalf("bindTags: %v", err)
	}
	return bs
}

func one(t *testing.T, bs []tagBinding) tagBinding {
	t.Helper()
	if len(bs) != 1 {
		t.Fatalf("want 1 binding, got %d: %+v", len(bs), bs)
	}
	return bs[0]
}

func TestBind_TagAfterAnnotationStack(t *testing.T) {
	src := `package com.example;
class OrderTest {
  @Test
  @Timeout(5)
  // covers: SC-01
  public void rejectsZeroQuantity() {
    helper();
  }
}
`
	b := one(t, bindAll(t, src, LangJava))
	if b.Ambiguous {
		t.Fatalf("ambiguous: %s", b.Detail)
	}
	if b.ClassName != "com.example.OrderTest" || b.Method != "rejectsZeroQuantity" {
		t.Errorf("got %s#%s", b.ClassName, b.Method)
	}
	if b.EndLine != 8 {
		t.Errorf("EndLine = %d, want 8 (the method's closing brace)", b.EndLine)
	}
}

func TestBind_KotlinBacktickName(t *testing.T) {
	src := "package com.example\nclass OrderTest {\n @Test\n // covers: SC-02\n fun `rejects zero quantity`() {}\n}\n"
	b := one(t, bindAll(t, src, LangKotlin))
	if b.Ambiguous {
		t.Fatalf("ambiguous: %s", b.Detail)
	}
	if b.ClassName != "com.example.OrderTest" || b.Method != "rejects zero quantity" {
		t.Errorf("got %s#%q", b.ClassName, b.Method)
	}
	if b.EndLine != 5 {
		t.Errorf("EndLine = %d, want 5 (empty body closes on the declaration line)", b.EndLine)
	}
}

func TestBind_EndLineKotlinMultiLineBody(t *testing.T) {
	src := "package com.example\n" +
		"class OrderTest {\n" +
		"  @Test\n" +
		"  // covers: SC-02\n" +
		"  fun rejectsZeroQuantity() {\n" +
		"    helper()\n" +
		"  }\n" +
		"}\n"
	b := one(t, bindAll(t, src, LangKotlin))
	if b.Ambiguous {
		t.Fatalf("ambiguous: %s", b.Detail)
	}
	if b.EndLine != 7 {
		t.Errorf("EndLine = %d, want 7", b.EndLine)
	}
}

func TestBind_JavaMethodWithThrowsClauseEndLine(t *testing.T) {
	src := `package com.example;
class OrderTest {
  // covers: SC-01
  public void rejectsZeroQuantity() throws Exception {
    helper();
  }
}
`
	b := one(t, bindAll(t, src, LangJava))
	if b.Ambiguous {
		t.Fatalf("ambiguous: %s", b.Detail)
	}
	if b.EndLine != 6 {
		t.Errorf("EndLine = %d, want 6", b.EndLine)
	}
}

func TestBind_AbstractMethodEndLineZero(t *testing.T) {
	src := `package p;
interface I {
  // covers: SC-01
  void method();
}
`
	b := one(t, bindAll(t, src, LangJava))
	if b.Ambiguous {
		t.Fatalf("ambiguous: %s", b.Detail)
	}
	if b.EndLine != 0 {
		t.Errorf("EndLine = %d, want 0 (no body)", b.EndLine)
	}
}

func TestBind_NestedClassBindsDollarName(t *testing.T) {
	src := `package p;
class Outer {
  class Inner {
    // covers: SC-03
    void deep() {}
  }
}
`
	b := one(t, bindAll(t, src, LangJava))
	if b.Ambiguous || b.ClassName != "p.Outer$Inner" || b.Method != "deep" {
		t.Errorf("got %+v, want p.Outer$Inner#deep", b)
	}
}

func TestBind_AmbiguousShapes(t *testing.T) {
	cases := []struct {
		name   string
		lang   Lang
		src    string
		detail string
	}{
		{"annotation after tag", LangJava,
			"class C {\n// covers: SC-1\n@Test\nvoid f() {}\n}\n",
			"annotation after covers tag"},
		{"statement between tag and decl", LangJava,
			"class C {\nvoid f() {\n// covers: SC-1\nint x = 1;\n}\nvoid g() {}\n}\n",
			"executable syntax"},
		{"tag above bare call in body", LangJava,
			"class C {\nvoid f() {\n// covers: SC-1\nhelper();\n}\n}\n",
			"no supported declaration"},
		{"tag above return call", LangJava,
			"class C {\nint f() {\n// covers: SC-1\nreturn helper();\n}\n}\n",
			"executable syntax"},
		{"tag above class declaration", LangJava,
			"// covers: SC-1\nclass C {\nvoid f() {}\n}\n",
			"class declaration"},
		{"tag above field", LangJava,
			"class C {\n// covers: SC-1\nint x = 1;\n}\n",
			"executable syntax"},
		{"tag above constructor", LangJava,
			"class C {\n// covers: SC-1\npublic C() {}\n}\n",
			"unsupported declaration shape"},
		{"tag at EOF", LangJava,
			"class C {\nvoid f() {}\n}\n// covers: SC-1\n",
			"no supported declaration"},
		{"kotlin top-level fun", LangKotlin,
			"package p\n// covers: SC-1\nfun topLevel() {}\n",
			"enclosing class"},
		{"kotlin companion object member", LangKotlin,
			"class C {\ncompanion object {\n// covers: SC-1\nfun helper() {}\n}\n}\n",
			"enclosing class"},
		{"kotlin local fun inside body", LangKotlin,
			"class C {\nfun outer() {\n// covers: SC-1\nfun local() {}\n}\n}\n",
			"declaration outside class body"},
		{"kotlin tag above property", LangKotlin,
			"class C {\n// covers: SC-1\nval x = 1\n}\n",
			"executable syntax"},
		{"kotlin extension fun", LangKotlin,
			"class C {\n// covers: SC-1\nfun String.ext() {}\n}\n",
			"unsupported declaration shape"},
		{"java anonymous class method", LangJava,
			"class C {\nRunnable r = new Runnable() {\n// covers: SC-1\npublic void run() {}\n};\n}\n",
			"declaration outside class body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := one(t, bindAll(t, tc.src, tc.lang))
			if !b.Ambiguous {
				t.Fatalf("bound %s#%s, want ambiguous", b.ClassName, b.Method)
			}
			if !strings.Contains(b.Detail, tc.detail) {
				t.Errorf("detail = %q, want substring %q", b.Detail, tc.detail)
			}
		})
	}
}
