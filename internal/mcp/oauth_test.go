package mcp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func jwkFromRSA(pub rsa.PublicKey, kid string) map[string]any {
	return map[string]any{
		"kty": "RSA",
		"kid": kid,
		"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

func newMockAS(t *testing.T, key *rsa.PrivateKey, kid string) (*httptest.Server, string) {
	t.Helper()
	var issuer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/jwks"})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{jwkFromRSA(key.PublicKey, kid)}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	issuer = srv.URL
	return srv, issuer
}

func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

func TestResourceServerValidateToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	as, issuer := newMockAS(t, key, "k1")
	defer as.Close()

	const resource = "https://polaris.example/mcp"
	rs := NewResourceServer(OAuthConfig{ResourceURI: resource, AuthorizationServers: []string{issuer}}, as.Client())

	good := signToken(t, key, "k1", jwt.MapClaims{
		"iss": issuer, "aud": resource, "sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(), "scope": "mcp:read mcp:call",
	})
	claims, err := rs.ValidateToken(context.Background(), good)
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if claims.Subject != "user-1" || !claims.HasScope("mcp:call") {
		t.Fatalf("claims wrong: %+v", claims)
	}

	// Wrong audience → rejected.
	badAud := signToken(t, key, "k1", jwt.MapClaims{"iss": issuer, "aud": "https://evil/mcp", "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := rs.ValidateToken(context.Background(), badAud); err == nil {
		t.Fatal("wrong-audience token should be rejected")
	}

	// Expired → rejected.
	expired := signToken(t, key, "k1", jwt.MapClaims{"iss": issuer, "aud": resource, "exp": time.Now().Add(-time.Hour).Unix()})
	if _, err := rs.ValidateToken(context.Background(), expired); err == nil {
		t.Fatal("expired token should be rejected")
	}

	// Untrusted issuer → rejected (issuer not in allow-list).
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	otherAS, otherIss := newMockAS(t, otherKey, "k1")
	defer otherAS.Close()
	foreign := signToken(t, otherKey, "k1", jwt.MapClaims{"iss": otherIss, "aud": resource, "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := rs.ValidateToken(context.Background(), foreign); err == nil {
		t.Fatal("untrusted-issuer token should be rejected")
	}
}

func TestResourceServerECKey(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	const kid = "ec-1"
	var issuer string
	as := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			w.WriteHeader(http.StatusNotFound) // exercise the oauth-authorization-server fallback
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"jwks_uri": issuer + "/jwks"})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{
				"kty": "EC", "kid": kid, "crv": "P-256",
				"x": base64.RawURLEncoding.EncodeToString(key.X.Bytes()),
				"y": base64.RawURLEncoding.EncodeToString(key.Y.Bytes()),
			}}})
		}
	}))
	defer as.Close()
	issuer = as.URL

	rs := NewResourceServer(OAuthConfig{ResourceURI: "https://p/mcp", AuthorizationServers: []string{issuer}}, as.Client())
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"iss": issuer, "aud": "https://p/mcp", "sub": "u", "exp": time.Now().Add(time.Hour).Unix()})
	tok.Header["kid"] = kid
	signed, _ := tok.SignedString(key)
	if _, err := rs.ValidateToken(context.Background(), signed); err != nil {
		t.Fatalf("EC token (with discovery fallback) rejected: %v", err)
	}
}

func TestResourceServerKeyRotation(t *testing.T) {
	key1, _ := rsa.GenerateKey(rand.Reader, 2048)
	key2, _ := rsa.GenerateKey(rand.Reader, 2048)
	servedKid, servedKey := "k1", key1
	var issuer string
	as := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"jwks_uri": issuer + "/jwks"})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{jwkFromRSA(servedKey.PublicKey, servedKid)}})
		}
	}))
	defer as.Close()
	issuer = as.URL

	const resource = "https://p/mcp"
	rs := NewResourceServer(OAuthConfig{ResourceURI: resource, AuthorizationServers: []string{issuer}}, as.Client())

	t1 := signToken(t, key1, "k1", jwt.MapClaims{"iss": issuer, "aud": resource, "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := rs.ValidateToken(context.Background(), t1); err != nil {
		t.Fatalf("initial token rejected: %v", err)
	}
	// Rotate the signing key; a token with the new kid forces a JWKS refresh.
	servedKid, servedKey = "k2", key2
	t2 := signToken(t, key2, "k2", jwt.MapClaims{"iss": issuer, "aud": resource, "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := rs.ValidateToken(context.Background(), t2); err != nil {
		t.Fatalf("post-rotation token rejected (refresh failed): %v", err)
	}
}

func TestResourceServerStaleGrace(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	up := true
	var issuer string
	as := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !up {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]any{"jwks_uri": issuer + "/jwks"})
		case "/jwks":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{jwkFromRSA(key.PublicKey, "k1")}})
		}
	}))
	defer as.Close()
	issuer = as.URL

	const resource = "https://p/mcp"
	rs := NewResourceServer(OAuthConfig{ResourceURI: resource, AuthorizationServers: []string{issuer}, JWKSCacheTTL: time.Minute}, as.Client())
	base := time.Unix(1_700_000_000, 0)
	rs.now = func() time.Time { return base }

	tok := signToken(t, key, "k1", jwt.MapClaims{"iss": issuer, "aud": resource, "exp": time.Now().Add(time.Hour).Unix()})
	if _, err := rs.ValidateToken(context.Background(), tok); err != nil {
		t.Fatalf("initial validate: %v", err)
	}
	// Authorization server is down and the cache has expired: the stale-grace
	// fallback must keep validating with the last-known key.
	up = false
	rs.now = func() time.Time { return base.Add(2 * time.Minute) }
	if _, err := rs.ValidateToken(context.Background(), tok); err != nil {
		t.Fatalf("stale-grace validation failed with AS down: %v", err)
	}
}

func TestResourceServerMetadataAndChallenge(t *testing.T) {
	rs := NewResourceServer(OAuthConfig{ResourceURI: "https://polaris.example/mcp", AuthorizationServers: []string{"https://as.example"}}, nil)
	md := rs.ProtectedResourceMetadata()
	if md["resource"] != "https://polaris.example/mcp" {
		t.Fatalf("metadata resource wrong: %v", md)
	}
	if got := rs.Challenge("https://polaris.example/.well-known/oauth-protected-resource"); got == "" || got[:6] != "Bearer" {
		t.Fatalf("challenge wrong: %q", got)
	}
}
