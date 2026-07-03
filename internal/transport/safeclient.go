package transport

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
)

// SSRFClient is an outbound HTTP client hardened against server-side request
// forgery: it enforces an allowed-scheme list, a denied-host list, and blocks
// private/reserved IPs at both URL-validation and dial time (so DNS rebinding
// cannot slip a public hostname onto an internal address). It is used to fetch
// caller-supplied file URLs.
type SSRFClient struct {
	client        *http.Client
	allowedScheme map[string]struct{}
	denyHosts     map[string]struct{}
}

// NewSSRFClient builds an SSRFClient from the files SSRF configuration.
func NewSSRFClient(cfg config.FileSSRFConfig, timeout time.Duration) *SSRFClient {
	allowedSchemes := cfg.AllowedSchemes
	if len(allowedSchemes) == 0 {
		allowedSchemes = []string{"https"}
	}
	denyHosts := cfg.DenyHosts
	if len(denyHosts) == 0 {
		denyHosts = []string{"169.254.169.254", "metadata.google.internal"}
	}
	client := &SSRFClient{
		allowedScheme: make(map[string]struct{}, len(allowedSchemes)),
		denyHosts:     make(map[string]struct{}, len(denyHosts)),
	}
	for _, scheme := range allowedSchemes {
		if trimmed := strings.ToLower(strings.TrimSpace(scheme)); trimmed != "" {
			client.allowedScheme[trimmed] = struct{}{}
		}
	}
	for _, host := range denyHosts {
		if trimmed := strings.ToLower(strings.TrimSpace(host)); trimmed != "" {
			client.denyHosts[trimmed] = struct{}{}
		}
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ip, err := client.resolveAllowedIP(ctx, host)
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       90 * time.Second,
	}
	client.client = &http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return client.ValidateURL(req.URL.String())
		},
	}
	return client
}

// Do validates req.URL then executes it.
func (c *SSRFClient) Do(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("ssrf: request URL is required")
	}
	if err := c.ValidateURL(req.URL.String()); err != nil {
		return nil, err
	}
	return c.client.Do(req)
}

// Get validates rawURL then issues a GET.
func (c *SSRFClient) Get(ctx context.Context, rawURL string) (*http.Response, error) {
	if err := c.ValidateURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	return c.client.Do(req)
}

// ValidateURL enforces the scheme allow-list, host deny-list, and literal-IP
// block.
func (c *SSRFClient) ValidateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed == nil || parsed.Host == "" {
		return fmt.Errorf("ssrf: invalid url")
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if _, ok := c.allowedScheme[scheme]; !ok {
		return fmt.Errorf("ssrf: scheme %q is not allowed", scheme)
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return fmt.Errorf("ssrf: host is required")
	}
	if _, denied := c.denyHosts[host]; denied {
		return fmt.Errorf("ssrf: host %q is denied", host)
	}
	if ip := net.ParseIP(host); ip != nil && blockedIP(ip) {
		return fmt.Errorf("ssrf: private or reserved IP is not allowed")
	}
	return nil
}

func (c *SSRFClient) resolveAllowedIP(ctx context.Context, host string) (net.IP, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, fmt.Errorf("ssrf: host is required")
	}
	if ip := net.ParseIP(host); ip != nil {
		if blockedIP(ip) {
			return nil, fmt.Errorf("ssrf: private or reserved IP is not allowed")
		}
		return ip, nil
	}
	if _, denied := c.denyHosts[strings.ToLower(host)]; denied {
		return nil, fmt.Errorf("ssrf: host %q is denied", host)
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if blockedIP(ip) {
			return nil, fmt.Errorf("ssrf: private or reserved IP is not allowed")
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("ssrf: host resolved to no addresses")
	}
	return ips[0], nil
}

func blockedIP(ip net.IP) bool {
	return ip.IsPrivate() ||
		ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}
