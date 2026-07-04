// Package handler implements the gateway's HTTP handlers: it translates HTTP
// requests into modality operations, dispatches them through the provider
// registry, and renders OpenAI-compatible responses and SSE streams.
package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

// Shared HMAC signed-token mechanics used by the session-secret tokens (audio,
// interpreting, streaming transcription) and the job/download ID signers. Only
// the byte-level crypto is shared here; each caller keeps its own wire-facing
// error text and payload types, which differ per surface.

var (
	errSignedTokenNotFound = errors.New("signed token: prefix not found")
	errSignedTokenInvalid  = errors.New("signed token: invalid")
)

// signHMACToken JSON-encodes payload, base64url-encodes it, appends an HMAC-SHA256
// signature, and returns prefix + payload + "." + signature.
func signHMACToken(secret []byte, prefix string, payload any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return prefix + encodedPayload + "." + signature, nil
}

// decodeSignedToken strips prefix, splits the payload and signature, base64url-
// decodes the payload, and unmarshals it into out. It returns the still-encoded
// payload and signature (for a later HMAC check, since the signing secret is
// often derived from the decoded payload). It returns errSignedTokenNotFound if
// the prefix does not match and errSignedTokenInvalid on any structural error.
func decodeSignedToken(prefix, token string, out any) (encodedPayload string, signature string, err error) {
	if !strings.HasPrefix(token, prefix) {
		return "", "", errSignedTokenNotFound
	}
	trimmed := strings.TrimPrefix(token, prefix)
	parts := strings.SplitN(trimmed, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", errSignedTokenInvalid
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", "", errSignedTokenInvalid
	}
	if err := json.Unmarshal(payloadBytes, out); err != nil {
		return "", "", errSignedTokenInvalid
	}
	return parts[0], parts[1], nil
}

// verifyHMACSignature constant-time compares the signature over encodedPayload
// against secret. Returns errSignedTokenInvalid on mismatch.
func verifyHMACSignature(secret []byte, encodedPayload, signature string) error {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encodedPayload))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil || !hmac.Equal(expected, actual) {
		return errSignedTokenInvalid
	}
	return nil
}
