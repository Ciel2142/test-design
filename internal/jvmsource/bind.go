package jvmsource

import "strings"

type tagBinding struct {
	Line      int
	Comment   string
	ClassName string
	Method    string
	Ambiguous bool
	Detail    string
}

var javaModifiers = map[string]bool{
	"public": true, "private": true, "protected": true, "static": true,
	"final": true, "abstract": true, "default": true, "synchronized": true,
	"native": true, "strictfp": true, "transient": true, "volatile": true,
}

var kotlinModifiers = map[string]bool{
	"public": true, "private": true, "protected": true, "internal": true,
	"override": true, "open": true, "final": true, "operator": true,
	"inline": true, "tailrec": true, "suspend": true, "infix": true,
	"external": true, "actual": true, "expect": true,
}

var statementKeywords = map[string]bool{
	"return": true, "throw": true, "new": true, "if": true, "else": true,
	"for": true, "while": true, "do": true, "switch": true, "when": true,
	"try": true, "assert": true, "this": true, "super": true, "yield": true,
	"break": true, "continue": true, "val": true, "var": true,
}

const (
	detailAnnotationAfterTag = "annotation after covers tag"
	detailExecutable         = "executable syntax between tag and declaration"
	detailAboveClass         = "tag above class declaration"
	detailNoDeclaration      = "no supported declaration after tag"
	detailUnsupportedShape   = "unsupported declaration shape"
	detailUnresolved         = "unresolved enclosing class"
	detailOutsideClassBody   = "declaration outside class body"
)

func bindTags(toks []Token, lang Lang) ([]tagBinding, error) {
	st, err := scanStructure(toks, lang)
	if err != nil {
		return nil, err
	}
	var out []tagBinding
	for i, tk := range toks {
		if tk.Kind != TokCoversTag {
			continue
		}
		b := tagBinding{Line: tk.Line, Comment: tk.Text}
		cls, method, detail := bindOne(toks, st, lang, i)
		if detail != "" {
			b.Ambiguous, b.Detail = true, detail
		} else {
			b.ClassName, b.Method = qualify(st.pkg, cls), method
		}
		out = append(out, b)
	}
	return out, nil
}

func bindOne(toks []Token, st *fileStructure, lang Lang, tag int) (cls, method, detail string) {
	typeSeen := false
	for j := tag + 1; j < len(toks); j++ {
		tk := toks[j]
		switch {
		case tk.Kind == TokCoversTag:
		case tk.Kind == TokLiteral:
			return "", "", detailExecutable
		case tk.Text == "@":
			return "", "", detailAnnotationAfterTag
		case tk.Kind == TokIdent && isClassKeyword(tk.Text, lang):
			return "", "", detailAboveClass
		case tk.Kind == TokIdent && statementKeywords[tk.Text]:
			return "", "", detailExecutable
		case lang == LangKotlin && tk.Kind == TokIdent && tk.Text == "fun":
			return bindKotlinFun(toks, st, j)
		case tk.Kind == TokIdent:
			if modifierFor(lang)[tk.Text] {
				continue
			}
			if j+1 < len(toks) && toks[j+1].Text == "(" && lang == LangJava {
				return bindJavaMethod(toks, st, j, typeSeen)
			}
			typeSeen = true
		case tk.Text == "<" || tk.Text == ">" || tk.Text == "," ||
			tk.Text == "." || tk.Text == "[" || tk.Text == "]" || tk.Text == "?":
		default:
			return "", "", detailExecutable
		}
	}
	return "", "", detailNoDeclaration
}

func bindJavaMethod(toks []Token, st *fileStructure, name int, typeSeen bool) (string, string, string) {
	enc := st.enclosing[name]
	if !typeSeen {
		if simpleName(enc) == toks[name].Text {
			return "", "", detailUnsupportedShape
		}
		return "", "", detailNoDeclaration
	}
	if unresolvedEnclosing(enc) {
		return "", "", detailUnresolved
	}
	if !st.member[name] {
		return "", "", detailOutsideClassBody
	}
	return enc, toks[name].Text, ""
}

func bindKotlinFun(toks []Token, st *fileStructure, fun int) (string, string, string) {
	j := fun + 1
	if j < len(toks) && toks[j].Text == "<" {
		for j < len(toks) && toks[j].Text != ">" {
			j++
		}
		j++
	}
	if j >= len(toks) || toks[j].Kind != TokIdent {
		return "", "", detailUnsupportedShape
	}
	name := j
	if name+1 >= len(toks) || toks[name+1].Text != "(" {
		return "", "", detailUnsupportedShape
	}
	enc := st.enclosing[fun]
	if unresolvedEnclosing(enc) {
		return "", "", detailUnresolved
	}
	if !st.member[fun] {
		return "", "", detailOutsideClassBody
	}
	return enc, strings.Trim(toks[name].Text, "`"), ""
}

func modifierFor(lang Lang) map[string]bool {
	if lang == LangKotlin {
		return kotlinModifiers
	}
	return javaModifiers
}

func simpleName(chain string) string {
	if i := strings.LastIndexByte(chain, '$'); i >= 0 {
		return chain[i+1:]
	}
	return chain
}

func qualify(pkg, cls string) string {
	if pkg == "" {
		return cls
	}
	return pkg + "." + cls
}
