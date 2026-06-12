package stage2

import "github.com/0xmhha/code-knowledge-system/pkg/testpath"

// testDemotionFactor is the multiplier applied to a test-file citation's
// final RRF score when the active intent is not test-oriented. A value of
// 0.25 means a test file must accumulate four times the raw RRF evidence
// of a production file to outrank it — enough to keep tests accessible
// lower in the evidence pack without hiding them entirely.
const testDemotionFactor = 0.25

// isTestPath reports whether file is a test or test-only support file. It
// delegates to pkg/testpath.IsTest, the single source of truth shared with
// the exclude_tests query filter, so graph and vector citations are
// classified consistently without re-indexing.
func isTestPath(file string) bool {
	return testpath.IsTest(file)
}
