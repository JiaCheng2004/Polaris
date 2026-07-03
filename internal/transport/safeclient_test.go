package transport

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
)

func newTestSSRF() *SSRFClient {
	return NewSSRFClient(config.FileSSRFConfig{
		AllowedSchemes: []string{"https"},
		DenyHosts:      []string{"169.254.169.254", "metadata.google.internal"},
	}, time.Second)
}

func TestSSRFValidateURL(t *testing.T) {
	c := newTestSSRF()
	cases := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{"valid https", "https://api.example.com/v1", false},
		{"scheme not allowed", "http://api.example.com", true},
		{"denied host", "https://169.254.169.254/latest/meta-data", true},
		{"literal private ip", "https://10.0.0.1/", true},
		{"literal loopback", "https://127.0.0.1/", true},
		{"invalid url", "://nope", true},
		{"missing host", "https://", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := c.ValidateURL(tc.url)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateURL(%q) err = %v, wantErr = %v", tc.url, err, tc.wantErr)
			}
		})
	}
}

func TestSSRFDefaultsWhenConfigEmpty(t *testing.T) {
	c := NewSSRFClient(config.FileSSRFConfig{}, 0)
	if err := c.ValidateURL("https://example.com"); err != nil {
		t.Fatalf("default https should be allowed: %v", err)
	}
	if err := c.ValidateURL("http://example.com"); err == nil {
		t.Fatal("default config should deny http")
	}
	if err := c.ValidateURL("https://169.254.169.254/"); err == nil {
		t.Fatal("default config should deny the cloud metadata host")
	}
}

func TestSSRFResolveAllowedIP(t *testing.T) {
	c := newTestSSRF()
	ctx := context.Background()
	if _, err := c.resolveAllowedIP(ctx, "10.0.0.1"); err == nil {
		t.Fatal("private IP should be blocked")
	}
	if _, err := c.resolveAllowedIP(ctx, "127.0.0.1"); err == nil {
		t.Fatal("loopback should be blocked")
	}
	if _, err := c.resolveAllowedIP(ctx, ""); err == nil {
		t.Fatal("empty host should error")
	}
	// localhost resolves to a loopback address -> blocked after DNS lookup.
	if _, err := c.resolveAllowedIP(ctx, "localhost"); err == nil {
		t.Fatal("localhost should resolve to a blocked address")
	}
	// A public literal IP passes resolution (connection is attempted elsewhere).
	if ip, err := c.resolveAllowedIP(ctx, "203.0.113.1"); err != nil || ip == nil {
		t.Fatalf("public literal IP should resolve: ip=%v err=%v", ip, err)
	}
}

func TestSSRFGetRejectsBlockedTargets(t *testing.T) {
	c := newTestSSRF()
	ctx := context.Background()
	for _, u := range []string{"http://example.com", "https://10.0.0.1/", "https://169.254.169.254/", "://bad"} {
		if _, err := c.Get(ctx, u); err == nil {
			t.Fatalf("Get(%q) should have been rejected", u)
		}
	}
}

func TestSSRFDoValidatesAndDials(t *testing.T) {
	c := newTestSSRF()
	if _, err := c.Do(nil); err == nil {
		t.Fatal("Do(nil) should error")
	}
	// A public but unreachable TEST-NET address passes validation, then the dial
	// fails fast (covers the DialContext path without a real network dependency).
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := c.Get(ctx, "https://203.0.113.1:1/"); err == nil {
		t.Fatal("Get to an unreachable public host should fail at dial")
	}
}

func TestBlockedIP(t *testing.T) {
	blocked := []string{"10.0.0.1", "192.168.1.1", "127.0.0.1", "169.254.1.1", "224.0.0.1", "0.0.0.0", "::1"}
	for _, s := range blocked {
		if !blockedIP(net.ParseIP(s)) {
			t.Fatalf("blockedIP(%s) = false, want true", s)
		}
	}
	if blockedIP(net.ParseIP("203.0.113.1")) {
		t.Fatal("public IP should not be blocked")
	}
}
