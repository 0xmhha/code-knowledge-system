package stage2

import (
	"path/filepath"
	"strings"
)

// testDemotionFactor is the multiplier applied to a test-file citation's
// final RRF score when the active intent is not test-oriented. A value of
// 0.25 means a test file must accumulate four times the raw RRF evidence
// of a production file to outrank it — enough to keep tests accessible
// lower in the evidence pack without hiding them entirely.
const testDemotionFactor = 0.25

// isTestPath reports whether file is a test source file according to the
// conventions used across Go, TypeScript, JavaScript, and Solidity. The
// rules mirror ckv's pkg/types/test_classify.go so citations from both
// backends are classified consistently without requiring re-indexing.
//
// Rules (all comparisons are on the base file name or path segments):
//
//	Go         : file ends with "_test.go"
//	TypeScript : file ends with ".test.ts", ".test.tsx", ".spec.ts", or ".spec.tsx"
//	JavaScript : file ends with ".test.js", ".test.jsx", ".spec.js", or ".spec.jsx"
//	Solidity   : file ends with ".t.sol", OR any path segment is "test" or "tests"
func isTestPath(file string) bool {
	if file == "" {
		return false
	}

	base := filepath.Base(file)

	// Go
	if strings.HasSuffix(base, "_test.go") {
		return true
	}

	// TypeScript
	if strings.HasSuffix(base, ".test.ts") ||
		strings.HasSuffix(base, ".test.tsx") ||
		strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".spec.tsx") {
		return true
	}

	// JavaScript
	if strings.HasSuffix(base, ".test.js") ||
		strings.HasSuffix(base, ".test.jsx") ||
		strings.HasSuffix(base, ".spec.js") ||
		strings.HasSuffix(base, ".spec.jsx") {
		return true
	}

	// Solidity: *.t.sol
	if strings.HasSuffix(base, ".t.sol") {
		return true
	}

	// Solidity (and any language): path segment named "test" or "tests"
	// Use the full path so directory names anywhere in the tree are caught.
	for _, seg := range strings.Split(filepath.ToSlash(file), "/") {
		if seg == "test" || seg == "tests" {
			return true
		}
	}

	return false
}
