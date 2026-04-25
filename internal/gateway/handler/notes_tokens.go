package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

const (
	audioNoteIDPrefix = "not_"
	audioNoteTTL      = 24 * time.Hour
)

type audioNoteToken struct {
	Version        int    `json:"v"`
	Provider       string `json:"p"`
	Model          string `json:"m"`
	ProviderTaskID string `json:"t"`
	KeyID          string `json:"k"`
	ExpiresAt      int64  `json:"e"`
}

func signAudioNoteID(snapshot *gwruntime.Snapshot, model provider.Model, providerTaskID string, keyID string, expiresAt int64) (string, error) {
	secret, err := videoJobSecret(snapshot, model.Provider)
	if err != nil {
		return "", err
	}
	payload := audioNoteToken{
		Version:        1,
		Provider:       model.Provider,
		Model:          model.ID,
		ProviderTaskID: strings.TrimSpace(providerTaskID),
		KeyID:          strings.TrimSpace(keyID),
		ExpiresAt:      expiresAt,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "job_id_encoding_failed", "", "Unable to issue audio note id.")
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return audioNoteIDPrefix + encodedPayload + "." + signature, nil
}

func parseAudioNoteID(snapshot *gwruntime.Snapshot, token string) (audioNoteToken, error) {
	if !strings.HasPrefix(token, audioNoteIDPrefix) {
		return audioNoteToken{}, invalidAudioNoteIDError()
	}
	trimmed := strings.TrimPrefix(token, audioNoteIDPrefix)
	parts := strings.SplitN(trimmed, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return audioNoteToken{}, invalidAudioNoteIDError()
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return audioNoteToken{}, invalidAudioNoteIDError()
	}
	var payload audioNoteToken
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return audioNoteToken{}, invalidAudioNoteIDError()
	}
	if payload.Version != 1 || strings.TrimSpace(payload.Provider) == "" || strings.TrimSpace(payload.Model) == "" || strings.TrimSpace(payload.ProviderTaskID) == "" || strings.TrimSpace(payload.KeyID) == "" || payload.ExpiresAt <= 0 {
		return audioNoteToken{}, invalidAudioNoteIDError()
	}
	secret, err := videoJobSecret(snapshot, payload.Provider)
	if err != nil {
		return audioNoteToken{}, invalidAudioNoteIDError()
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expected, actual) {
		return audioNoteToken{}, invalidAudioNoteIDError()
	}
	if time.Now().Unix() >= payload.ExpiresAt {
		return audioNoteToken{}, httputil.NewError(http.StatusGone, "invalid_request_error", "asset_expired", "id", "Audio note has expired.")
	}
	return payload, nil
}

func invalidAudioNoteIDError() error {
	return httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Audio note job was not found.")
}
