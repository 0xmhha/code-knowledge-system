// Package testpath classifies source-file paths as test or test-only
// support files. It is the single source of truth for the exclude_tests
// query filter and for the composer's test-demotion heuristic, so graph
// (ckg) and vector (ckv) citations are judged identically without
// re-indexing.
package testpath

import (
	"path/filepath"
	"strings"
)

// IsTest reports whether file is a test source file or a test-only support
// file, using language conventions plus common test-helper naming.
//
// Strict test files:
//
//	Go         : base ends with "_test.go"
//	TypeScript : base ends with ".test.ts"/".test.tsx"/".spec.ts"/".spec.tsx"
//	JavaScript : base ends with ".test.js"/".test.jsx"/".spec.js"/".spec.jsx"
//	Solidity   : base ends with ".t.sol"
//
// Test-only support code (compiled into the package, but exists solely to
// serve tests, so it is noise for design/implementation retrieval):
//
//	Go          : base begins with "testutil" or "testhelper"
//	any language: a path segment is test/tests/testdata/testutil/testutils/testhelpers
func IsTest(file string) bool {
	if file == "" {
		return false
	}

	slashed := filepath.ToSlash(file)
	base := filepath.Base(slashed)

	// Go test files.
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	// Go test-only helpers: testutil.go, testutils.go, testutil_xxx.go,
	// testhelper.go, ... — compiled into the package but test-support only.
	if strings.HasSuffix(base, ".go") &&
		(strings.HasPrefix(base, "testutil") || strings.HasPrefix(base, "testhelper")) {
		return true
	}

	// TypeScript / JavaScript test files.
	for _, suf := range []string{
		".test.ts", ".test.tsx", ".spec.ts", ".spec.tsx",
		".test.js", ".test.jsx", ".spec.js", ".spec.jsx",
	} {
		if strings.HasSuffix(base, suf) {
			return true
		}
	}

	// Solidity test contracts.
	if strings.HasSuffix(base, ".t.sol") {
		return true
	}

	// Test directories anywhere in the path.
	for _, seg := range strings.Split(slashed, "/") {
		switch seg {
		case "test", "tests", "testdata", "testutil", "testutils", "testhelpers":
			return true
		}
	}

	return false
}
