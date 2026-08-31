package jvmsource

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"test-design/internal/tags"
)

type TagSite struct {
	ClassName string
	Method    string
	File      string
	Line      int
	EndLine   int
	IDs       []string
	Ambiguous bool
	Detail    string
}

// ScanTags walks root for .java/.kt files and returns one TagSite per
// covers: comment, bound or ambiguous. Fails closed: a file that does not
// lex or balance aborts the whole scan.
func ScanTags(root string) ([]TagSite, error) {
	var sites []TagSite
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		var lang Lang
		switch filepath.Ext(path) {
		case ".java":
			lang = LangJava
		case ".kt":
			lang = LangKotlin
		default:
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		toks, err := Lex(string(src), lang)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		bindings, err := bindTags(toks, lang)
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		for _, b := range bindings {
			sites = append(sites, TagSite{
				ClassName: b.ClassName,
				Method:    b.Method,
				File:      filepath.ToSlash(rel),
				Line:      b.Line,
				EndLine:   b.EndLine,
				IDs:       tags.Extract(b.Comment),
				Ambiguous: b.Ambiguous,
				Detail:    b.Detail,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(sites, func(i, j int) bool {
		if sites[i].File != sites[j].File {
			return sites[i].File < sites[j].File
		}
		return sites[i].Line < sites[j].Line
	})
	return sites, nil
}
