package services

import (
	"net"
	"testing"
)

func TestSignBody(t *testing.T) {
	// Fixed inputs so a future refactor can't silently change the wire format.
	got := signBody("topsecret", "1700000000", []byte(`{"hello":"world"}`))
	// Pre-computed: hex(HMAC_SHA256("topsecret", "1700000000.{\"hello\":\"world\"}"))
	want := "sha256=79883357e4c4c4abee43cf4b32367d67a1344520479e3e8c85e98406a6d6a2a5"
	if got != want {
		t.Fatalf("signBody mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestIsPrivateAddr(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"1.2.3.4", false},
		{"8.8.8.8", false},
		{"127.0.0.1", true},
		{"127.255.255.255", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.32.0.1", false},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"0.0.0.0", true},
		{"100.64.0.1", true},
		{"::1", true},
		{"fc00::1", true},
		{"fe80::1", true},
		{"2001:db8::1", false},
		{"::ffff:127.0.0.1", true},
		{"::ffff:100.64.0.1", true},  // CGNAT mapped IPv6
		{"::ffff:1.2.3.4", false},    // public mapped IPv6
		{"224.0.0.1", true},          // multicast
		{"239.255.255.255", true},    // multicast
	}
	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			if ip == nil {
				t.Fatalf("parse failed for %q", tc.ip)
			}
			if got := isPrivateAddr(ip); got != tc.want {
				t.Fatalf("isPrivateAddr(%s) = %v, want %v", tc.ip, got, tc.want)
			}
		})
	}
}

func TestValidateWebhookURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"plain http", "http://example.com/hook", false},
		{"https with port", "https://example.com:8443/hook", false},
		{"missing scheme", "example.com/hook", true},
		{"ftp scheme", "ftp://example.com", true},
		{"no host", "http:///hook", true},
		{"empty", "", true},
		{"malformed", "://bad", true},
		{"with userinfo", "http://user:pass@example.com/hook", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWebhookURL(tc.url)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
