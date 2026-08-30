package jvmsource

import "testing"

func lexTexts(t *testing.T, src string, lang Lang) []string {
	t.Helper()
	toks, err := Lex(src, lang)
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	var out []string
	for _, tk := range toks {
		out = append(out, tk.Text)
	}
	return out
}

func TestLexer_KotlinRawStringOpaque(t *testing.T) {
	src := "val s = \"\"\"line \" {\n// covers: SC-88\n\"\"\"\n// covers: SC-03\n"
	toks, err := Lex(src, LangKotlin)
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	var covers []Token
	for _, tk := range toks {
		if tk.Kind == TokCoversTag {
			covers = append(covers, tk)
		}
		if tk.Text == "{" {
			t.Errorf("brace inside raw string leaked: %+v", tk)
		}
	}
	if len(covers) != 1 || covers[0].Line != 4 {
		t.Fatalf("covers = %+v, want one tag on line 4", covers)
	}
}

func TestLexer_BacktickIdentifierSingleToken(t *testing.T) {
	src := "fun `passes with spaces and {`() {}\n"
	toks, err := Lex(src, LangKotlin)
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	var found bool
	for _, tk := range toks {
		if tk.Kind == TokIdent && tk.Text == "`passes with spaces and {`" {
			found = true
		}
	}
	if !found {
		t.Fatalf("backtick ident missing as one token: %v", lexTexts(t, src, LangKotlin))
	}
	var braces int
	for _, tk := range toks {
		if tk.Text == "{" {
			braces++
		}
	}
	if braces != 1 {
		t.Errorf("brace tokens = %d, want 1 (only body brace)", braces)
	}
}

func TestLexer_JavaBlockCommentRemovedTracksLines(t *testing.T) {
	src := "/* covers: SC-77 {\nstill comment */\n// covers: SC-01\nvoid f() {}\n"
	toks, err := Lex(src, LangJava)
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	var covers []Token
	for _, tk := range toks {
		if tk.Kind == TokCoversTag {
			covers = append(covers, tk)
		}
	}
	if len(covers) != 1 || covers[0].Line != 3 {
		t.Fatalf("covers = %+v, want one tag on line 3", covers)
	}
	for _, tk := range toks {
		if tk.Text == "{" && tk.Line < 4 {
			t.Errorf("brace from comment leaked: %+v", tk)
		}
	}
}

func TestLexer_KotlinNestedBlockComment(t *testing.T) {
	src := "/* outer /* inner */ still outer */\n// covers: SC-02\nfun f() {}\n"
	toks, err := Lex(src, LangKotlin)
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	var covers int
	for _, tk := range toks {
		if tk.Kind == TokCoversTag {
			covers++
		}
		if tk.Text == "outer" || tk.Text == "*" && tk.Line == 1 {
			t.Errorf("nested comment content leaked: %+v", tk)
		}
	}
	if covers != 1 {
		t.Errorf("covers tokens = %d, want 1", covers)
	}
}

func TestLexer_StringAndCharLiteralsOpaque(t *testing.T) {
	src := "String s = \"// covers: SC-99 \\\" {\"; char c = '{';\n// covers: SC-01\n"
	toks, err := Lex(src, LangJava)
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	var covers, braces, lits int
	for _, tk := range toks {
		switch {
		case tk.Kind == TokCoversTag:
			covers++
		case tk.Kind == TokLiteral:
			lits++
		case tk.Text == "{":
			braces++
		}
	}
	if covers != 1 {
		t.Errorf("covers tokens = %d, want 1 (literal content must be opaque)", covers)
	}
	if braces != 0 {
		t.Errorf("brace tokens = %d, want 0 (braces inside literals must not count)", braces)
	}
	if lits != 2 {
		t.Errorf("literal tokens = %d, want 2", lits)
	}
}

func TestLexer_KeepsCoversCommentDropsOthers(t *testing.T) {
	src := "// setup helper\n// covers: SC-01\nvoid f() {}\n"
	toks, err := Lex(src, LangJava)
	if err != nil {
		t.Fatalf("Lex: %v", err)
	}
	var covers []Token
	for _, tk := range toks {
		if tk.Kind == TokCoversTag {
			covers = append(covers, tk)
		}
	}
	if len(covers) != 1 {
		t.Fatalf("want 1 covers token, got %d: %v", len(covers), toks)
	}
	if covers[0].Text != "covers: SC-01" {
		t.Errorf("covers text = %q", covers[0].Text)
	}
	if covers[0].Line != 2 {
		t.Errorf("covers line = %d, want 2", covers[0].Line)
	}
	for _, tk := range toks {
		if tk.Kind != TokCoversTag && tk.Text == "setup" {
			t.Errorf("non-tag comment leaked into tokens: %v", toks)
		}
	}
}
