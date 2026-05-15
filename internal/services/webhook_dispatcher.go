package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// signBody returns "sha256=<hex>" where the digest is
// HMAC_SHA256(secret, timestamp + "." + body).
func signBody(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte{'.'})
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// validateWebhookURL enforces http/https + non-empty host. It does NOT resolve
// the host — that happens at dial time via the private-IP guard.
func validateWebhookURL(raw string) error {
	if raw == "" {
		return errors.New("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("url scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("url host is required")
	}
	return nil
}

// isPrivateAddr returns true for IPs we refuse to dial when
// WEBHOOK_ALLOW_PRIVATE_IPS is false.
func isPrivateAddr(ip net.IP) bool {
	if ip == nil {
		return true // be conservative
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return true
	}
	// IsPrivate covers RFC1918 + RFC4193 (fc00::/7).
	if ip.IsPrivate() {
		return true
	}
	// Carrier-grade NAT 100.64.0.0/10 — not flagged by IsPrivate.
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1]&0xc0 == 64 {
			return true
		}
	}
	return false
}
