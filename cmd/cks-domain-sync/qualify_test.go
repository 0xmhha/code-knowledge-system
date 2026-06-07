package main

import "testing"

func TestQualifyGovernsSymbol(t *testing.T) {
	cases := []struct {
		name           string
		file, symbol   string
		want           string
	}{
		{"plain function",
			"params/config_wbft.go", "DefaultAnzeonConfig",
			"params.DefaultAnzeonConfig"},
		{"method qname (receiver included)",
			"consensus/wbft/validator/default.go", "defaultSet.QuorumSize",
			"validator.defaultSet.QuorumSize"},
		{"already qualified — no double prefix",
			"params/config.go", "params.DefaultAnzeonConfig",
			"params.DefaultAnzeonConfig"},
		{"deeper path — last segment wins",
			"core/txpool/legacypool/legacypool.go", "LegacyPool.loop",
			"legacypool.LegacyPool.loop"},
		{"empty file falls back to symbol",
			"", "Foo",
			"Foo"},
		{"empty symbol passes through",
			"params/config.go", "",
			""},
		{"top-level file (no dir) falls back",
			"Makefile", "Build",
			"Build"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := qualifyGovernsSymbol(tc.file, tc.symbol)
			if got != tc.want {
				t.Errorf("qualifyGovernsSymbol(%q, %q) = %q, want %q",
					tc.file, tc.symbol, got, tc.want)
			}
		})
	}
}
