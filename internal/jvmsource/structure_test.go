package jvmsource

import "testing"

func mustStructure(t *testing.T, src string, lang Lang) (string, []Token, []string) {
	t.Helper()
	toks, err := Lex(src, lang)
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	st, err := scanStructure(toks, lang)
	if err != nil {
		t.Fatalf("scanStructure: %v", err)
	}
	return st.pkg, toks, st.enclosing
}

func enclosingOf(toks []Token, enclosing []string, text string) (string, bool) {
	for i, tk := range toks {
		if tk.Text == text {
			return enclosing[i], true
		}
	}
	return "", false
}

func TestStructure_NestedClassDollarName(t *testing.T) {
	src := `package com.example;
class Outer {
  class Inner {
    void deep() {}
  }
  void shallow() {}
}
`
	pkg, toks, enclosing := mustStructure(t, src, LangJava)
	if pkg != "com.example" {
		t.Errorf("pkg = %q, want com.example", pkg)
	}
	if got, ok := enclosingOf(toks, enclosing, "deep"); !ok || got != "Outer$Inner" {
		t.Errorf("deep enclosing = %q, want Outer$Inner", got)
	}
	if got, ok := enclosingOf(toks, enclosing, "shallow"); !ok || got != "Outer" {
		t.Errorf("shallow enclosing = %q, want Outer", got)
	}
}

func TestStructure_KotlinTopLevelFunHasNoEnclosing(t *testing.T) {
	src := "package com.example\nfun topLevel() {}\nclass Real { fun inside() {} }\n"
	_, toks, enclosing := mustStructure(t, src, LangKotlin)
	if got, ok := enclosingOf(toks, enclosing, "topLevel"); !ok || got != "" {
		t.Errorf("topLevel enclosing = %q, want empty", got)
	}
	if got, ok := enclosingOf(toks, enclosing, "inside"); !ok || got != "Real" {
		t.Errorf("inside enclosing = %q, want Real", got)
	}
}

func TestStructure_CompanionObjectMarksUnresolved(t *testing.T) {
	src := "class Real {\n companion object {\n fun helper() {}\n }\n}\n"
	_, toks, enclosing := mustStructure(t, src, LangKotlin)
	got, ok := enclosingOf(toks, enclosing, "helper")
	if !ok {
		t.Fatal("helper token missing")
	}
	if !unresolvedEnclosing(got) {
		t.Errorf("helper enclosing = %q, want unresolved marker", got)
	}
}

func TestStructure_JavaObjectIdentifierNotClassKeyword(t *testing.T) {
	src := "class C {\n void f(Object object) {\n if (true) { object.toString(); }\n }\n}\n"
	_, toks, enclosing := mustStructure(t, src, LangJava)
	if got, ok := enclosingOf(toks, enclosing, "toString"); !ok || got != "C" {
		t.Errorf("toString enclosing = %q, want C", got)
	}
}

func TestStructure_BraceImbalanceFails(t *testing.T) {
	toks, err := Lex("class Broken {\n void f() {}\n", LangJava)
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	if _, err := scanStructure(toks, LangJava); err == nil {
		t.Fatal("want error on unbalanced braces, got nil")
	}
}
