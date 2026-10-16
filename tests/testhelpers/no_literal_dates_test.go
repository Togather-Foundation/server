package testhelpers

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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
		files, err := filepath.Glob(filepath.Join(root, dir, "*_test.go"))
		require.NoError(t, err)
		require.NotEmpty(t, files, "no test files found in %s; has the directory moved?", dir)
		for _, file := range files {
			violations = append(violations, literalDateViolations(t, root, file)...)
		}
	}

	sort.Strings(violations)
	require.Empty(t, violations,
		"tests in wall-clock-sensitive packages must not hardcode dates. Use testhelpers.FutureTime/PastTime/FutureDate/PastDate, "+
			"or add `// %s <reason>` on the same or previous line if the date can never rot:\n  %s",
		fixedDateMarker, strings.Join(violations, "\n  "))
}

func literalDateViolations(t *testing.T, root, path string) []string {
	t.Helper()

	src, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(string(src), "\n")

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	require.NoError(t, err, "parse %s", path)

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

	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Date" || len(x.Args) == 0 {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "time" {
				return true
			}
			if lit, ok := x.Args[0].(*ast.BasicLit); ok && lit.Kind == token.INT {
				report(x.Pos(), "time.Date with a literal year")
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
	})
	return out
}
