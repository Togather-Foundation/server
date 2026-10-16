package testhelpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestGuardCatchesLiteralDateProbes proves the guard can fail: it plants
// unmarked literal dates in a nested directory and a marked control, then
// checks that exactly the unmarked ones are reported.
func TestGuardCatchesLiteralDateProbes(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) {
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(src), 0o644))
	}

	write("pkg/top_test.go", `package pkg

import "time"

var _ = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
`)
	// Nested subpackage: a flat directory glob would miss this.
	write("pkg/sub/deep_test.go", `package sub

import "time"

var _ = "2031-02-03T04:05:06Z"
var _ = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) // fixed-date: control, must not be reported
`)
	// A year hidden behind a constant, declared in another file of the package.
	write("pkg/sub/consts_test.go", `package sub

const probeYear = 2028
`)
	write("pkg/sub/use_test.go", `package sub

import "time"

var _ = time.Date(probeYear, 1, 1, 0, 0, 0, 0, time.UTC)

func local() time.Time {
	year := 2029
	return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
}

// Unrelated identifiers and non-test files are not reported.
func fine(year int) time.Time { return time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC) }
`)
	write("pkg/sub/helper.go", `package sub

import "time"

var _ = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
`)
	write("pkg/testdata/fixture_test.go", `package ignored

var _ = "2031-02-03"
`)

	got := scanTree(t, root, "pkg")
	require.Equal(t, []string{
		"pkg/sub/deep_test.go:5: string literal containing date \"2031-02-03\"",
		"pkg/sub/use_test.go:5: time.Date with a year held in a constant/variable initialised from a literal",
		"pkg/sub/use_test.go:9: time.Date with a year held in a constant/variable initialised from a literal",
		"pkg/top_test.go:5: time.Date with a literal year",
	}, got)
}
