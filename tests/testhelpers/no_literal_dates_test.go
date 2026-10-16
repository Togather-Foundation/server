package testhelpers

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// wallClockSensitiveDirs are the packages whose tests exercise code that reads
// the wall clock (HTTP handlers, ingest, the review queue, SQL NOW(), feeds,
// stats). A literal date in these tests is a time bomb: it is fine on the day it
// is written and fails once the calendar passes it. Pure unit tests that hand
// an explicit `now` to the code under test live elsewhere and are not scanned.
var wallClockSensitiveDirs = []string{
	"tests/contracts",
	"tests/integration",
	"tests/integration_batch",
	"internal/storage/postgres",
}

// fixedDateMarker whitelists a literal date on the same line or the line above.
// The text after it must say why the date cannot rot (for example a
// serialisation round-trip that never compares against "now").
const fixedDateMarker = "fixed-date:"

var isoDatePattern = regexp.MustCompile(`\b(?:19|20)\d{2}-\d{2}-\d{2}`)

// TestNoLiteralDatesInWallClockSensitiveTests fails when a test in a
// wall-clock-sensitive package hardcodes a calendar date without a
// `// fixed-date: <reason>` marker. Use FutureTime/PastTime/FutureDate/PastDate.
func TestNoLiteralDatesInWallClockSensitiveTests(t *testing.T) {
	root := ProjectRoot(t)

	var violations []string
	for _, dir := range wallClockSensitiveDirs {
		violations = append(violations, scanTree(t, root, dir)...)
	}

	sort.Strings(violations)
	require.Empty(t, violations,
		"tests in wall-clock-sensitive packages must not hardcode dates. Use testhelpers.FutureTime/PastTime/FutureDate/PastDate, "+
			"or add `// %s <reason>` on the same or previous line if the date can never rot:\n  %s",
		fixedDateMarker, strings.Join(violations, "\n  "))
}

// scanTree walks root/dir recursively (so test subpackages cannot escape the
// guard) and returns the unmarked literal dates in its *_test.go files.
// testdata directories are skipped, as the go tool does.
func scanTree(t *testing.T, root, dir string) []string {
	t.Helper()

	start := filepath.Join(root, dir)
	byDir := map[string][]string{}
	require.NoError(t, filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			byDir[filepath.Dir(path)] = append(byDir[filepath.Dir(path)], path)
		}
		return nil
	}))
	require.NotEmpty(t, byDir, "no test files found under %s; has the directory moved?", dir)

	var out []string
	for _, files := range byDir {
		parsed := parseFiles(t, files)
		// A year hidden behind a package-level const/var (const probeYear =
		// 2028, possibly declared in another file) is still a literal.
		pkgYears := map[string]bool{}
		for _, pf := range parsed {
			for _, d := range pf.ast.Decls {
				if gd, ok := d.(*ast.GenDecl); ok {
					maps.Copy(pkgYears, yearBindings(gd))
				}
			}
		}
		for _, pf := range parsed {
			out = append(out, literalDateViolations(t, root, pf, pkgYears)...)
		}
	}
	return out
}

type parsedFile struct {
	path string
	src  []byte
	fset *token.FileSet
	ast  *ast.File
}

func parseFiles(t *testing.T, paths []string) []parsedFile {
	t.Helper()
	var out []parsedFile
	for _, path := range paths {
		src, err := os.ReadFile(path)
		require.NoError(t, err)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		require.NoError(t, err, "parse %s", path)
		out = append(out, parsedFile{path: path, src: src, fset: fset, ast: f})
	}
	return out
}

func isLiteralYear(e ast.Expr) bool {
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT || len(lit.Value) != 4 {
		return false
	}
	n, err := strconv.Atoi(lit.Value)
	return err == nil && n >= 1900 && n <= 2099
}

// yearBindings returns the names that a const/var declaration or assignment
// inside n binds to a four-digit year literal.
func yearBindings(n ast.Node) map[string]bool {
	names := map[string]bool{}
	ast.Inspect(n, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ValueSpec:
			for i, v := range x.Values {
				if i < len(x.Names) && isLiteralYear(v) {
					names[x.Names[i].Name] = true
				}
			}
		case *ast.AssignStmt:
			if len(x.Lhs) == len(x.Rhs) {
				for i, v := range x.Rhs {
					if id, ok := x.Lhs[i].(*ast.Ident); ok && isLiteralYear(v) {
						names[id.Name] = true
					}
				}
			}
		}
		return true
	})
	return names
}

func literalDateViolations(t *testing.T, root string, pf parsedFile, pkgYears map[string]bool) []string {
	t.Helper()

	path, src, fset, f := pf.path, pf.src, pf.fset, pf.ast

	lines := strings.Split(string(src), "\n")

	marked := func(line int) bool {
		for _, l := range []int{line, line - 1} {
			if l >= 1 && l <= len(lines) && strings.Contains(lines[l-1], fixedDateMarker) {
				return true
			}
		}
		return false
	}

	rel, err := filepath.Rel(root, path)
	require.NoError(t, err)

	var out []string
	report := func(pos token.Pos, what string) {
		p := fset.Position(pos)
		if marked(p.Line) {
			return
		}
		out = append(out, fmt.Sprintf("%s:%d: %s", rel, p.Line, what))
	}

	// yearIdents are the package-level year constants plus, inside a function,
	// that function's own year-literal locals (so an unrelated parameter that
	// happens to share a name with another function's local is not flagged).
	yearIdents := pkgYears
	visit := func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Date" || len(x.Args) == 0 {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "time" {
				return true
			}
			switch arg := x.Args[0].(type) {
			case *ast.BasicLit:
				if arg.Kind == token.INT {
					report(x.Pos(), "time.Date with a literal year")
				}
			case *ast.Ident:
				if yearIdents[arg.Name] {
					report(x.Pos(), "time.Date with a year held in a constant/variable initialised from a literal")
				}
			}
		case *ast.BasicLit:
			if x.Kind != token.STRING {
				return true
			}
			val, err := strconv.Unquote(x.Value)
			if err != nil {
				val = x.Value
			}
			// "2006-01-02" is Go's reference-time layout, not a date.
			val = strings.ReplaceAll(val, time.DateOnly, "")
			if m := isoDatePattern.FindString(val); m != "" {
				report(x.Pos(), fmt.Sprintf("string literal containing date %q", m))
			}
		}
		return true
	}
	for _, d := range f.Decls {
		yearIdents = pkgYears
		if fd, ok := d.(*ast.FuncDecl); ok {
			yearIdents = maps.Clone(pkgYears)
			maps.Copy(yearIdents, yearBindings(fd))
		}
		ast.Inspect(d, visit)
	}
	return out
}
