package handler

import (
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
	secret, err := speechSessionSecret(snapshot, model.Provider)
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
	signed, err := signHMACToken(secret, audioNoteIDPrefix, payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "job_id_encoding_failed", "", "Unable to issue audio note id.")
	}
	return signed, nil
}

func parseAudioNoteID(snapshot *gwruntime.Snapshot, token string) (audioNoteToken, error) {
	var payload audioNoteToken
	encodedPayload, signature, err := decodeSignedToken(audioNoteIDPrefix, token, &payload)
	if err != nil {
		return audioNoteToken{}, invalidAudioNoteIDError()
	}
	if payload.Version != 1 || strings.TrimSpace(payload.Provider) == "" || strings.TrimSpace(payload.Model) == "" || strings.TrimSpace(payload.ProviderTaskID) == "" || strings.TrimSpace(payload.KeyID) == "" || payload.ExpiresAt <= 0 {
		return audioNoteToken{}, invalidAudioNoteIDError()
	}
	secret, err := speechSessionSecret(snapshot, payload.Provider)
	if err != nil {
		return audioNoteToken{}, invalidAudioNoteIDError()
	}
	if err := verifyHMACSignature(secret, encodedPayload, signature); err != nil {
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
