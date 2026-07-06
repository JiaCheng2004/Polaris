package mcp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// validSigningMethods are the asymmetric algorithms the resource server accepts.
var validSigningMethods = []string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512"}

// OAuthConfig configures the MCP OAuth 2.1 resource server (RFC 9728).
type OAuthConfig struct {
	ResourceURI          string
	AuthorizationServers []string
	JWKSCacheTTL         time.Duration
}

// Claims are the validated token claims relevant to MCP authorization.
type Claims struct {
	Subject string
	Issuer  string
	Scopes  []string
}

// HasScope reports whether the token carries scope.
func (c Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

// ResourceServer validates bearer JWTs against the configured authorization
// servers' JWKS (fetched, cached, and rotated with a stale-grace fallback) and
// serves RFC 9728 protected-resource metadata. Downstream tokens are never
// forwarded upstream.
type ResourceServer struct {
	cfg  OAuthConfig
	http *http.Client
	now  func() time.Time

	mu   sync.Mutex
	jwks map[string]jwksEntry // issuer -> cached keys
}

type jwksEntry struct {
	keys    map[string]any // kid -> *rsa.PublicKey | *ecdsa.PublicKey
	expires time.Time
}

// NewResourceServer builds a resource server. A nil client uses a 10s default.
func NewResourceServer(cfg OAuthConfig, httpClient *http.Client) *ResourceServer {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.JWKSCacheTTL <= 0 {
		cfg.JWKSCacheTTL = time.Hour
	}
	return &ResourceServer{cfg: cfg, http: httpClient, now: time.Now, jwks: map[string]jwksEntry{}}
}

// ProtectedResourceMetadata is the RFC 9728 document served at
// /.well-known/oauth-protected-resource.
func (rs *ResourceServer) ProtectedResourceMetadata() map[string]any {
	return map[string]any{
		"resource":                 rs.cfg.ResourceURI,
		"authorization_servers":    rs.cfg.AuthorizationServers,
		"bearer_methods_supported": []string{"header"},
	}
}

// Challenge is the WWW-Authenticate value for a 401 pointing at the metadata.
func (rs *ResourceServer) Challenge(metadataURL string) string {
	return fmt.Sprintf(`Bearer resource_metadata="%s"`, metadataURL)
}

// ValidateToken parses and verifies a bearer JWT and returns its claims. It
// enforces the signing algorithm allow-list, an allowed issuer, aud == resource
// URI, and a required expiry.
func (rs *ResourceServer) ValidateToken(ctx context.Context, tokenString string) (Claims, error) {
	keyfunc := func(token *jwt.Token) (any, error) {
		iss, err := token.Claims.GetIssuer()
		if err != nil || !rs.issuerAllowed(iss) {
			return nil, fmt.Errorf("issuer not allowed")
		}
		kid, _ := token.Header["kid"].(string)
		return rs.publicKey(ctx, iss, kid)
	}
	token, err := jwt.Parse(tokenString, keyfunc,
		jwt.WithValidMethods(validSigningMethods),
		jwt.WithAudience(rs.cfg.ResourceURI),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return Claims{}, err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, fmt.Errorf("unexpected claims type")
	}
	iss, _ := claims.GetIssuer()
	sub, _ := claims.GetSubject()
	return Claims{Subject: sub, Issuer: iss, Scopes: parseScopes(claims["scope"])}, nil
}

func (rs *ResourceServer) issuerAllowed(iss string) bool {
	for _, a := range rs.cfg.AuthorizationServers {
		if strings.TrimRight(a, "/") == strings.TrimRight(iss, "/") {
			return true
		}
	}
	return false
}

func (rs *ResourceServer) publicKey(ctx context.Context, iss, kid string) (any, error) {
	if key := rs.cachedKey(iss, kid, false); key != nil {
		return key, nil
	}
	// Cache miss or rotation: refresh once.
	if err := rs.refresh(ctx, iss); err != nil {
		// Stale-grace: fall back to any cached key we still hold.
		if key := rs.cachedKey(iss, kid, true); key != nil {
			return key, nil
		}
		return nil, err
	}
	if key := rs.cachedKey(iss, kid, true); key != nil {
		return key, nil
	}
	return nil, fmt.Errorf("unknown key id")
}

// cachedKey returns a cached key; when ignoreExpiry is false, an expired entry is
// treated as a miss (forcing a refresh).
func (rs *ResourceServer) cachedKey(iss, kid string, ignoreExpiry bool) any {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	entry, ok := rs.jwks[iss]
	if !ok {
		return nil
	}
	if !ignoreExpiry && rs.now().After(entry.expires) {
		return nil
	}
	return entry.keys[kid]
}

func (rs *ResourceServer) refresh(ctx context.Context, iss string) error {
	keys, err := rs.fetchJWKS(ctx, iss)
	if err != nil {
		return err
	}
	rs.mu.Lock()
	rs.jwks[iss] = jwksEntry{keys: keys, expires: rs.now().Add(rs.cfg.JWKSCacheTTL)}
	rs.mu.Unlock()
	return nil
}

func (rs *ResourceServer) fetchJWKS(ctx context.Context, iss string) (map[string]any, error) {
	jwksURI, err := rs.discoverJWKSURI(ctx, iss)
	if err != nil {
		return nil, err
	}
	body, err := rs.getJSON(ctx, jwksURI)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	out := make(map[string]any, len(doc.Keys))
	for _, k := range doc.Keys {
		if pub, err := k.publicKey(); err == nil {
			out[k.Kid] = pub
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("jwks contained no usable keys")
	}
	return out, nil
}

func (rs *ResourceServer) discoverJWKSURI(ctx context.Context, iss string) (string, error) {
	base := strings.TrimRight(iss, "/")
	for _, wellKnown := range []string{"/.well-known/openid-configuration", "/.well-known/oauth-authorization-server"} {
		body, err := rs.getJSON(ctx, base+wellKnown)
		if err != nil {
			continue
		}
		var doc struct {
			JWKSURI string `json:"jwks_uri"`
		}
		if json.Unmarshal(body, &doc) == nil && doc.JWKSURI != "" {
			return doc.JWKSURI, nil
		}
	}
	return "", fmt.Errorf("could not discover jwks_uri for issuer %s", iss)
}

func (rs *ResourceServer) getJSON(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := rs.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// jwk is one JSON Web Key.
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

func (k jwk) publicKey() (any, error) {
	switch k.Kty {
	case "RSA":
		n, err := b64uBigInt(k.N)
		if err != nil {
			return nil, err
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
		return &rsa.PublicKey{N: n, E: int(new(big.Int).SetBytes(eBytes).Int64())}, nil
	case "EC":
		curve, err := curveFor(k.Crv)
		if err != nil {
			return nil, err
		}
		x, err := b64uBigInt(k.X)
		if err != nil {
			return nil, err
		}
		y, err := b64uBigInt(k.Y)
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
	default:
		return nil, fmt.Errorf("unsupported key type: %s", k.Kty)
	}
}

func b64uBigInt(s string) (*big.Int, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(b), nil
}

func curveFor(crv string) (elliptic.Curve, error) {
	switch crv {
	case "P-256":
		return elliptic.P256(), nil
	case "P-384":
		return elliptic.P384(), nil
	case "P-521":
		return elliptic.P521(), nil
	default:
		return nil, fmt.Errorf("unsupported curve: %s", crv)
	}
}

func parseScopes(v any) []string {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	return strings.Fields(s)
}
