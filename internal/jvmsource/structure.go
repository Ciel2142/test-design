package jvmsource

import (
	"fmt"
	"strings"
)

func isClassKeyword(text string, lang Lang) bool {
	switch text {
	case "class", "interface":
		return true
	case "enum":
		return lang == LangJava
	case "object":
		return lang == LangKotlin
	}
	return false
}

// unnamed frame (companion object) poisons the chain: never guess a binding
const unresolvedMarker = "?"

func unresolvedEnclosing(chain string) bool {
	return chain == "" || strings.Contains(chain, unresolvedMarker)
}

type classFrame struct {
	name  string
	depth int
}

type fileStructure struct {
	pkg       string
	enclosing []string // per token: enclosing chain Outer$Inner, "" = top level
	member    []bool   // per token: directly at class-body depth
}

// fails closed on brace imbalance at EOF
func scanStructure(toks []Token, lang Lang) (*fileStructure, error) {
	pkg := ""
	enclosing := make([]string, len(toks))
	member := make([]bool, len(toks))
	var stack []classFrame
	depth := 0
	pendingClass := ""
	pendingKeyword := false

	current := func() string {
		names := make([]string, len(stack))
		for i, f := range stack {
			names[i] = f.name
		}
		return strings.Join(names, "$")
	}

	for i, tk := range toks {
		enclosing[i] = current()
		member[i] = len(stack) > 0 && depth == stack[len(stack)-1].depth
		switch {
		case tk.Kind == TokIdent && tk.Text == "package" && depth == 0 && pkg == "":
			var parts []string
			wantIdent := true
			for j := i + 1; j < len(toks); j++ {
				t := toks[j]
				if wantIdent && t.Kind == TokIdent {
					parts = append(parts, t.Text)
				} else if !wantIdent && t.Text == "." {
				} else {
					break
				}
				wantIdent = !wantIdent
			}
			pkg = strings.Join(parts, ".")
		case tk.Kind == TokIdent && isClassKeyword(tk.Text, lang) && !precededByColonColon(toks, i):
			pendingKeyword = true
			pendingClass = ""
		case pendingKeyword && tk.Kind == TokIdent && pendingClass == "":
			pendingClass = tk.Text
		case tk.Text == "{":
			depth++
			if pendingKeyword {
				name := pendingClass
				if name == "" {
					name = unresolvedMarker
				}
				stack = append(stack, classFrame{name: name, depth: depth})
				pendingKeyword = false
				pendingClass = ""
			}
		case tk.Text == "}":
			if len(stack) > 0 && stack[len(stack)-1].depth == depth {
				stack = stack[:len(stack)-1]
			}
			depth--
			if depth < 0 {
				return nil, fmt.Errorf("unbalanced braces: extra } at line %d", tk.Line)
			}
		case tk.Text == ";" || tk.Text == "=":
			pendingKeyword = false
			pendingClass = ""
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("unbalanced braces: %d unclosed at EOF", depth)
	}
	return &fileStructure{pkg: pkg, enclosing: enclosing, member: member}, nil
}

func precededByColonColon(toks []Token, i int) bool {
	return i >= 2 && toks[i-1].Text == ":" && toks[i-2].Text == ":"
}
