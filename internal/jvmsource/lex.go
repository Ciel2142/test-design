package jvmsource

import "strings"

type Lang int

const (
	LangJava Lang = iota
	LangKotlin
)

type TokenKind int

const (
	TokIdent TokenKind = iota
	TokSymbol
	TokLiteral
	TokCoversTag
)

type Token struct {
	Kind TokenKind
	Text string
	Line int
}

func Lex(src string, lang Lang) ([]Token, error) {
	var toks []Token
	line := 1
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '/' && i+1 < len(src) && src[i+1] == '/':
			end := strings.IndexByte(src[i:], '\n')
			if end < 0 {
				end = len(src) - i
			}
			body := strings.TrimSpace(src[i+2 : i+end])
			if strings.HasPrefix(body, "covers:") {
				toks = append(toks, Token{Kind: TokCoversTag, Text: body, Line: line})
			}
			i += end
		case c == '/' && i+1 < len(src) && src[i+1] == '*':
			depth := 1
			j := i + 2
			for j < len(src) && depth > 0 {
				switch {
				case src[j] == '\n':
					line++
					j++
				case lang == LangKotlin && src[j] == '/' && j+1 < len(src) && src[j+1] == '*':
					depth++
					j += 2
				case src[j] == '*' && j+1 < len(src) && src[j+1] == '/':
					depth--
					j += 2
				default:
					j++
				}
			}
			i = j
		case lang == LangKotlin && strings.HasPrefix(src[i:], `"""`):
			startLine := line
			j := i + 3
			for j < len(src) && !strings.HasPrefix(src[j:], `"""`) {
				if src[j] == '\n' {
					line++
				}
				j++
			}
			if j < len(src) {
				j += 3
			}
			toks = append(toks, Token{Kind: TokLiteral, Text: src[i:j], Line: startLine})
			i = j
		case lang == LangKotlin && c == '`':
			j := i + 1
			for j < len(src) && src[j] != '`' && src[j] != '\n' {
				j++
			}
			if j < len(src) && src[j] == '`' {
				j++
			}
			toks = append(toks, Token{Kind: TokIdent, Text: src[i:j], Line: line})
			i = j
		case c == '"' || c == '\'':
			startLine := line
			j := i + 1
			for j < len(src) && src[j] != c {
				if src[j] == '\\' {
					j++
				} else if src[j] == '\n' {
					line++
				}
				j++
			}
			if j < len(src) {
				j++
			}
			toks = append(toks, Token{Kind: TokLiteral, Text: src[i:j], Line: startLine})
			i = j
		case isIdentStart(c):
			j := i
			for j < len(src) && isIdentPart(src[j]) {
				j++
			}
			toks = append(toks, Token{Kind: TokIdent, Text: src[i:j], Line: line})
			i = j
		default:
			toks = append(toks, Token{Kind: TokSymbol, Text: string(c), Line: line})
			i++
		}
	}
	return toks, nil
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= 0x80
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || c >= '0' && c <= '9'
}
