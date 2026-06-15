package mcp

import (
	"net"
	"testing"
)

func TestClientACL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		policy HTTPPolicy
		ip     string
		want   bool
	}{
		{"loopback always allowed (remote off)", HTTPPolicy{AllowRemote: false}, "127.0.0.1", true},
		{"loopback v6 allowed", HTTPPolicy{AllowRemote: false}, "::1", true},
		{"private denied when remote off", HTTPPolicy{AllowRemote: false}, "192.168.1.5", false},
		{"private allowed by default LAN policy", HTTPPolicy{AllowRemote: true}, "192.168.1.5", true},
		{"10/8 allowed by default LAN policy", HTTPPolicy{AllowRemote: true}, "10.1.2.3", true},
		{"172.16/12 allowed by default LAN policy", HTTPPolicy{AllowRemote: true}, "172.16.0.9", true},
		{"link-local allowed by default LAN policy", HTTPPolicy{AllowRemote: true}, "169.254.3.4", true},
		{"public denied even when remote on", HTTPPolicy{AllowRemote: true}, "8.8.8.8", false},
		{
			"explicit CIDR allows listed subnet",
			HTTPPolicy{AllowRemote: true, AllowedCIDRs: []string{"192.168.1.0/24"}},
			"192.168.1.42", true,
		},
		{
			"explicit CIDR denies other private subnet",
			HTTPPolicy{AllowRemote: true, AllowedCIDRs: []string{"192.168.1.0/24"}},
			"192.168.2.42", false,
		},
		{
			"explicit CIDR denies other private range (replaces default)",
			HTTPPolicy{AllowRemote: true, AllowedCIDRs: []string{"192.168.1.0/24"}},
			"10.0.0.1", false,
		},
		{
			"explicit CIDR still allows loopback",
			HTTPPolicy{AllowRemote: true, AllowedCIDRs: []string{"192.168.1.0/24"}},
			"127.0.0.1", true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allow, err := clientACL(tc.policy)
			if err != nil {
				t.Fatalf("clientACL: %v", err)
			}
			if got := allow(net.ParseIP(tc.ip)); got != tc.want {
				t.Errorf("allow(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestClientACL_InvalidCIDR(t *testing.T) {
	t.Parallel()
	if _, err := clientACL(HTTPPolicy{AllowRemote: true, AllowedCIDRs: []string{"not-a-cidr"}}); err == nil {
		t.Fatal("expected error for invalid CIDR, got nil")
	}
}

func TestClientACL_NilIP(t *testing.T) {
	t.Parallel()
	allow, err := clientACL(HTTPPolicy{AllowRemote: true})
	if err != nil {
		t.Fatalf("clientACL: %v", err)
	}
	if allow(nil) {
		t.Error("nil IP must be denied")
	}
}
