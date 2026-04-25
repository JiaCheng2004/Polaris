package handler

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

const (
	streamTranscriptionSessionIDPrefix     = "sttsess_"
	streamTranscriptionSessionSecretPrefix = "sttsec_"
)

type streamTranscriptionPublicToken struct {
	Version   int    `json:"v"`
	Provider  string `json:"p"`
	Model     string `json:"m"`
	KeyID     string `json:"k"`
	ExpiresAt int64  `json:"e"`
	Nonce     string `json:"n"`
}

type streamTranscriptionSecretToken struct {
	Version   int                                          `json:"v"`
	Provider  string                                       `json:"p"`
	Model     string                                       `json:"m"`
	KeyID     string                                       `json:"k"`
	ExpiresAt int64                                        `json:"e"`
	Nonce     string                                       `json:"n"`
	Config    modality.StreamingTranscriptionSessionConfig `json:"c"`
}

func issueStreamingTranscriptionSession(snapshot *gwruntime.Snapshot, model provider.Model, keyID string, cfg modality.StreamingTranscriptionSessionConfig, ttl time.Duration) (*modality.StreamingTranscriptionSessionDescriptor, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	expiresAt := time.Now().Add(ttl).Unix()
	nonce, err := newStreamingTranscriptionNonce()
	if err != nil {
		return nil, httputil.NewError(http.StatusInternalServerError, "internal_error", "session_id_encoding_failed", "", "Unable to issue streaming transcription session id.")
	}

	publicToken := streamTranscriptionPublicToken{
		Version:   1,
		Provider:  model.Provider,
		Model:     model.ID,
		KeyID:     strings.TrimSpace(keyID),
		ExpiresAt: expiresAt,
		Nonce:     nonce,
	}
	secretToken := streamTranscriptionSecretToken{
		Version:   publicToken.Version,
		Provider:  publicToken.Provider,
		Model:     publicToken.Model,
		KeyID:     publicToken.KeyID,
		ExpiresAt: publicToken.ExpiresAt,
		Nonce:     publicToken.Nonce,
		Config:    cfg,
	}

	signedID, err := signStreamingTranscriptionPayload(snapshot, model.Provider, streamTranscriptionSessionIDPrefix, publicToken)
	if err != nil {
		return nil, err
	}
	signedSecret, err := signStreamingTranscriptionPayload(snapshot, model.Provider, streamTranscriptionSessionSecretPrefix, secretToken)
	if err != nil {
		return nil, err
	}

	return &modality.StreamingTranscriptionSessionDescriptor{
		ID:           signedID,
		Object:       "audio.transcription.session",
		Model:        model.ID,
		ExpiresAt:    expiresAt,
		ClientSecret: signedSecret,
	}, nil
}

func parseStreamingTranscriptionSession(snapshot *gwruntime.Snapshot, sessionID string, clientSecret string) (streamTranscriptionSecretToken, error) {
	publicToken, err := parseSignedStreamingTranscriptionPayload[streamTranscriptionPublicToken](snapshot, streamTranscriptionSessionIDPrefix, sessionID)
	if err != nil {
		return streamTranscriptionSecretToken{}, err
	}
	secretToken, err := parseSignedStreamingTranscriptionPayload[streamTranscriptionSecretToken](snapshot, streamTranscriptionSessionSecretPrefix, clientSecret)
	if err != nil {
		return streamTranscriptionSecretToken{}, err
	}
	if publicToken.Version != secretToken.Version || publicToken.Provider != secretToken.Provider || publicToken.Model != secretToken.Model || publicToken.KeyID != secretToken.KeyID || publicToken.ExpiresAt != secretToken.ExpiresAt || publicToken.Nonce != secretToken.Nonce {
		return streamTranscriptionSecretToken{}, invalidStreamingTranscriptionSessionError("session", "Streaming transcription session token mismatch.")
	}
	if time.Now().Unix() >= secretToken.ExpiresAt {
		return streamTranscriptionSecretToken{}, invalidStreamingTranscriptionSessionError("session", "Streaming transcription session has expired.")
	}
	return secretToken, nil
}

func signStreamingTranscriptionPayload(snapshot *gwruntime.Snapshot, providerName string, prefix string, payload any) (string, error) {
	secret, err := videoJobSecret(snapshot, providerName)
	if err != nil {
		return "", invalidStreamingTranscriptionSessionError("session", "Streaming transcription session signing secret is unavailable.")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "session_id_encoding_failed", "", "Unable to issue streaming transcription session token.")
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return prefix + encodedPayload + "." + signature, nil
}

func parseSignedStreamingTranscriptionPayload[T any](snapshot *gwruntime.Snapshot, prefix string, token string) (T, error) {
	var payload T
	if !strings.HasPrefix(token, prefix) {
		return payload, invalidStreamingTranscriptionSessionError("session", "Streaming transcription session token was not found.")
	}
	trimmed := strings.TrimPrefix(token, prefix)
	parts := strings.SplitN(trimmed, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return payload, invalidStreamingTranscriptionSessionError("session", "Streaming transcription session token is invalid.")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return payload, invalidStreamingTranscriptionSessionError("session", "Streaming transcription session token is invalid.")
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return payload, invalidStreamingTranscriptionSessionError("session", "Streaming transcription session token is invalid.")
	}

	var providerName string
	switch typed := any(payload).(type) {
	case streamTranscriptionPublicToken:
		providerName = typed.Provider
	case streamTranscriptionSecretToken:
		providerName = typed.Provider
	default:
		return payload, invalidStreamingTranscriptionSessionError("session", "Streaming transcription session token is invalid.")
	}
	secret, err := videoJobSecret(snapshot, providerName)
	if err != nil {
		return payload, invalidStreamingTranscriptionSessionError("session", "Streaming transcription session token is invalid.")
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expected, actual) {
		return payload, invalidStreamingTranscriptionSessionError("session", "Streaming transcription session token is invalid.")
	}
	return payload, nil
}

func invalidStreamingTranscriptionSessionError(param string, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "Streaming transcription session was not found."
	}
	return httputil.NewError(http.StatusUnauthorized, "invalid_request_error", "session_not_found", param, message)
}

func newStreamingTranscriptionNonce() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
