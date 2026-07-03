package transport

import "net/http"

// BearerAuth returns an AuthFunc that sets "Authorization: Bearer <token>".
func BearerAuth(token string) AuthFunc {
	return func(req *http.Request, _ []byte) error {
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}
}

// APIKeyHeaderAuth returns an AuthFunc that sets a single API-key header, e.g.
// ("x-api-key", key) for Anthropic or ("xi-api-key", key) for ElevenLabs.
func APIKeyHeaderAuth(header, key string) AuthFunc {
	return func(req *http.Request, _ []byte) error {
		req.Header.Set(header, key)
		return nil
	}
}
