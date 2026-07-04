package handler

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

const (
	audioSessionIDPrefix     = "audsess_"
	audioSessionSecretPrefix = "audsec_"
)

type audioSessionPublicToken struct {
	Version   int    `json:"v"`
	Provider  string `json:"p"`
	Model     string `json:"m"`
	KeyID     string `json:"k"`
	ExpiresAt int64  `json:"e"`
	Nonce     string `json:"n"`
}

type audioSessionSecretToken struct {
	Version   int                         `json:"v"`
	Provider  string                      `json:"p"`
	Model     string                      `json:"m"`
	KeyID     string                      `json:"k"`
	ExpiresAt int64                       `json:"e"`
	Nonce     string                      `json:"n"`
	Config    modality.AudioSessionConfig `json:"c"`
}

func issueAudioSession(snapshot *gwruntime.Snapshot, model provider.Model, keyID string, cfg modality.AudioSessionConfig, ttl time.Duration) (*modality.AudioSessionDescriptor, error) {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	expiresAt := time.Now().Add(ttl).Unix()
	nonce, err := newAudioNonce()
	if err != nil {
		return nil, httputil.NewError(http.StatusInternalServerError, "internal_error", "session_id_encoding_failed", "", "Unable to issue audio session id.")
	}

	publicToken := audioSessionPublicToken{
		Version:   1,
		Provider:  model.Provider,
		Model:     model.ID,
		KeyID:     strings.TrimSpace(keyID),
		ExpiresAt: expiresAt,
		Nonce:     nonce,
	}
	secretToken := audioSessionSecretToken{
		Version:   publicToken.Version,
		Provider:  publicToken.Provider,
		Model:     publicToken.Model,
		KeyID:     publicToken.KeyID,
		ExpiresAt: publicToken.ExpiresAt,
		Nonce:     publicToken.Nonce,
		Config:    cfg,
	}

	signedID, err := signAudioPayload(snapshot, model.Provider, audioSessionIDPrefix, publicToken)
	if err != nil {
		return nil, err
	}
	signedSecret, err := signAudioPayload(snapshot, model.Provider, audioSessionSecretPrefix, secretToken)
	if err != nil {
		return nil, err
	}

	return &modality.AudioSessionDescriptor{
		ID:           signedID,
		Object:       "audio.session",
		Model:        model.ID,
		ExpiresAt:    expiresAt,
		ClientSecret: signedSecret,
	}, nil
}

func parseAudioSession(snapshot *gwruntime.Snapshot, sessionID string, clientSecret string) (audioSessionSecretToken, error) {
	publicToken, err := parseSignedAudioPayload[audioSessionPublicToken](snapshot, audioSessionIDPrefix, sessionID)
	if err != nil {
		return audioSessionSecretToken{}, err
	}
	secretToken, err := parseSignedAudioPayload[audioSessionSecretToken](snapshot, audioSessionSecretPrefix, clientSecret)
	if err != nil {
		return audioSessionSecretToken{}, err
	}
	if publicToken.Version != secretToken.Version || publicToken.Provider != secretToken.Provider || publicToken.Model != secretToken.Model || publicToken.KeyID != secretToken.KeyID || publicToken.ExpiresAt != secretToken.ExpiresAt || publicToken.Nonce != secretToken.Nonce {
		return audioSessionSecretToken{}, invalidAudioSessionError("session", "Audio session token mismatch.")
	}
	if time.Now().Unix() >= secretToken.ExpiresAt {
		return audioSessionSecretToken{}, invalidAudioSessionError("session", "Audio session has expired.")
	}
	return secretToken, nil
}

func signAudioPayload(snapshot *gwruntime.Snapshot, providerName string, prefix string, payload any) (string, error) {
	secret, err := speechSessionSecret(snapshot, providerName)
	if err != nil {
		return "", invalidAudioSessionError("session", "Audio session signing secret is unavailable.")
	}
	token, err := signHMACToken(secret, prefix, payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "session_id_encoding_failed", "", "Unable to issue audio session token.")
	}
	return token, nil
}

func parseSignedAudioPayload[T any](snapshot *gwruntime.Snapshot, prefix string, token string) (T, error) {
	var payload T
	encodedPayload, signature, err := decodeSignedToken(prefix, token, &payload)
	if err != nil {
		if errors.Is(err, errSignedTokenNotFound) {
			return payload, invalidAudioSessionError("session", "Audio session token was not found.")
		}
		return payload, invalidAudioSessionError("session", "Audio session token is invalid.")
	}

	var providerName string
	switch typed := any(payload).(type) {
	case audioSessionPublicToken:
		providerName = typed.Provider
	case audioSessionSecretToken:
		providerName = typed.Provider
	default:
		return payload, invalidAudioSessionError("session", "Audio session token is invalid.")
	}
	secret, err := speechSessionSecret(snapshot, providerName)
	if err != nil {
		return payload, invalidAudioSessionError("session", "Audio session token is invalid.")
	}
	if err := verifyHMACSignature(secret, encodedPayload, signature); err != nil {
		return payload, invalidAudioSessionError("session", "Audio session token is invalid.")
	}
	return payload, nil
}

func invalidAudioSessionError(param string, message string) error {
	if strings.TrimSpace(message) == "" {
		message = "Audio session was not found."
	}
	return httputil.NewError(http.StatusUnauthorized, "invalid_request_error", "session_not_found", param, message)
}

func newAudioNonce() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}
