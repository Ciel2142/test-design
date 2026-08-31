// Package trace binds `covers:` comments in Go source to the test
// declarations they document.
package trace

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"test-design/internal/tags"
)

type TagSite struct {
	ImportPath string
	TestName   string
	File       string
	Line       int
	EndLine    int
	IDs        []string
	Ambiguous  bool
	Detail     string
}

var testFuncNamePattern = regexp.MustCompile(`^Test([A-Z0-9_].*)?$`)

// ScanTags walks worktreeRoot for *_test.go files and returns one TagSite
// per covers: comment found, bound or ambiguous. It fails closed: a
// *_test.go file that fails to parse aborts the whole scan.
func ScanTags(worktreeRoot, modulePath string) ([]TagSite, error) {
	var sites []TagSite

	err := filepath.WalkDir(worktreeRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == worktreeRoot {
				return nil
			}
			name := d.Name()
			if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if _, statErr := os.Stat(filepath.Join(path, "go.mod")); statErr == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		fileSites, scanErr := scanFile(path, worktreeRoot, modulePath)
		if scanErr != nil {
			return scanErr
		}
		sites = append(sites, fileSites...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.SliceStable(sites, func(i, j int) bool {
		if sites[i].File != sites[j].File {
			return sites[i].File < sites[j].File
		}
		return sites[i].Line < sites[j].Line
	})
	return sites, nil
}

func scanFile(path, worktreeRoot, modulePath string) ([]TagSite, error) {
	fset := token.NewFileSet()
	astFile, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("trace: parse %s: %w", path, err)
	}

	relFile, err := filepath.Rel(worktreeRoot, path)
	if err != nil {
		return nil, fmt.Errorf("trace: relativize %s: %w", path, err)
	}
	relFile = filepath.ToSlash(relFile)
	importPath := importPathFor(modulePath, filepath.ToSlash(filepath.Dir(relFile)))

	var sites []TagSite
	consumed := make(map[*ast.CommentGroup]bool)

	for _, decl := range astFile.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Doc == nil {
			continue
		}
		consumed[fd.Doc] = true

		ids := tags.Extract(commentGroupText(fd.Doc))
		if len(ids) == 0 {
			continue
		}
		line := markerLine(fset, fd.Doc)

		if reason := testShapeMismatch(fd); reason != "" {
			sites = append(sites, TagSite{
				ImportPath: importPath,
				File:       relFile,
				Line:       line,
				IDs:        ids,
				Ambiguous:  true,
				Detail:     fmt.Sprintf("func %s: %s", fd.Name.Name, reason),
			})
			continue
		}

		sites = append(sites, TagSite{
			ImportPath: importPath,
			TestName:   fd.Name.Name,
			File:       relFile,
			Line:       line,
			EndLine:    bodyEndLine(fset, fd),
			IDs:        ids,
			Ambiguous:  false,
		})
	}

	for _, cg := range astFile.Comments {
		if consumed[cg] {
			continue
		}
		ids := tags.Extract(commentGroupText(cg))
		if len(ids) == 0 {
			continue
		}
		line := markerLine(fset, cg)
		sites = append(sites, TagSite{
			ImportPath: importPath,
			File:       relFile,
			Line:       line,
			IDs:        ids,
			Ambiguous:  true,
			Detail:     unboundReason(astFile, cg),
		})
	}

	return sites, nil
}

func bodyEndLine(fset *token.FileSet, fd *ast.FuncDecl) int {
	if fd.Body == nil {
		return 0
	}
	return fset.Position(fd.Body.Rbrace).Line
}

func importPathFor(modulePath, relDir string) string {
	if relDir == "." {
		return modulePath
	}
	return modulePath + "/" + relDir
}

func commentGroupText(cg *ast.CommentGroup) string {
	lines := make([]string, len(cg.List))
	for i, c := range cg.List {
		lines[i] = c.Text
	}
	return strings.Join(lines, "\n")
}

func markerLine(fset *token.FileSet, cg *ast.CommentGroup) int {
	for _, c := range cg.List {
		if len(tags.Extract(c.Text)) > 0 {
			return fset.Position(c.Pos()).Line
		}
	}
	return fset.Position(cg.Pos()).Line
}

func testShapeMismatch(fd *ast.FuncDecl) string {
	if fd.Recv != nil {
		return "has a receiver; go test only runs top-level functions"
	}
	if !testFuncNamePattern.MatchString(fd.Name.Name) {
		return `name does not match Test[A-Z0-9_].* (or exactly "Test")`
	}
	if !hasSingleTestingTParam(fd.Type) {
		return "does not take a single *testing.T parameter"
	}
	return ""
}

func hasSingleTestingTParam(ft *ast.FuncType) bool {
	if ft.Params == nil {
		return false
	}
	total := 0
	for _, field := range ft.Params.List {
		n := len(field.Names)
		if n == 0 {
			n = 1
		}
		total += n
	}
	if total != 1 {
		return false
	}
	return isTestingTType(ft.Params.List[0].Type)
}

func isTestingTType(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkgIdent.Name == "testing" && sel.Sel.Name == "T"
}

func unboundReason(file *ast.File, cg *ast.CommentGroup) string {
	if insideFuncBody(file, cg) {
		return "comment is inside a function body, not immediately above a test function"
	}
	if aboveTypeVarConst(file, cg) {
		return "comment is above a type/var/const declaration, not a test function"
	}
	if trailingAtEOF(file, cg) {
		return "comment trails at end of file with no following declaration"
	}
	return "comment is free-floating between declarations, not attached as doc to any function"
}

func insideFuncBody(file *ast.File, cg *ast.CommentGroup) bool {
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		if cg.Pos() > fd.Body.Lbrace && cg.End() <= fd.Body.Rbrace {
			return true
		}
	}
	return false
}

func aboveTypeVarConst(file *ast.File, cg *ast.CommentGroup) bool {
	found := false
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		if gd.Tok != token.TYPE && gd.Tok != token.VAR && gd.Tok != token.CONST {
			continue
		}
		if gd.Doc == cg {
			found = true
			break
		}
		for _, spec := range gd.Specs {
			switch s := spec.(type) {
			case *ast.TypeSpec:
				if s.Doc == cg {
					found = true
				}
			case *ast.ValueSpec:
				if s.Doc == cg {
					found = true
				}
			}
		}
		if found {
			break
		}
	}
	return found
}

func trailingAtEOF(file *ast.File, cg *ast.CommentGroup) bool {
	if len(file.Decls) == 0 {
		return true
	}
	last := file.Decls[len(file.Decls)-1]
	return cg.Pos() > last.End()
}
