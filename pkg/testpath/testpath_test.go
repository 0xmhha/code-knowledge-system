package testpath

import "testing"

func TestIsTest(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file string
		want bool
	}{
		// Go production files — not test
		{"main.go", false},
		{"handler.go", false},
		{"core/txpool/legacypool.go", false},
		{"consensus/wbft/finalize.go", false},

		// Go test files
		{"handler_test.go", true},
		{"core/txpool/legacypool_test.go", true},

		// Go test-only support files
		{"consensus/wbft/testutil.go", true},
		{"triedb/pathdb/testutils.go", true},
		{"pkg/foo/testutil_setup.go", true},
		{"pkg/foo/testhelper.go", true},
		{"pkg/foo/testhelpers.go", true},
		{"consensus/wbft/testutils/genesis.go", true},
		{"x/testdata/fixture.go", true},

		// TypeScript / JavaScript
		{"app.ts", false},
		{"handler.test.ts", true},
		{"handler.spec.ts", true},
		{"index.test.js", true},

		// Solidity
		{"contracts/Token.sol", false},
		{"contracts/Token.t.sol", true},
		{"systemcontracts/solidity/test/MockFiatToken.sol", true},
		{"systemcontracts/test/contracts_wbft.go", true},

		// Edge cases — "test" substring must not over-match
		{"", false},
		{"test.go", false},
		{"tests.go", false},
		{"contextual.go", false},
		{"protest.go", false},
		{"latest.go", false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			if got := IsTest(tc.file); got != tc.want {
				t.Errorf("IsTest(%q) = %v, want %v", tc.file, got, tc.want)
			}
		})
	}
}
