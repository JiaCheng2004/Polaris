package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

const videoJobIDPrefix = "vid_"

type videoJobToken struct {
	Version       int    `json:"v"`
	Provider      string `json:"p"`
	Model         string `json:"m"`
	ProviderJobID string `json:"j"`
	KeyID         string `json:"k"`
}

func signVideoJobID(snapshot *gwruntime.Snapshot, model provider.Model, providerJobID string, keyID string) (string, error) {
	secret, err := videoJobSecret(snapshot, model.Provider)
	if err != nil {
		return "", err
	}

	payload := videoJobToken{
		Version:       1,
		Provider:      model.Provider,
		Model:         model.ID,
		ProviderJobID: strings.TrimSpace(providerJobID),
		KeyID:         strings.TrimSpace(keyID),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "job_id_encoding_failed", "", "Unable to issue video job id.")
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return videoJobIDPrefix + encodedPayload + "." + signature, nil
}

func parseVideoJobID(snapshot *gwruntime.Snapshot, token string) (videoJobToken, error) {
	if !strings.HasPrefix(token, videoJobIDPrefix) {
		return videoJobToken{}, invalidVideoJobIDError()
	}

	trimmed := strings.TrimPrefix(token, videoJobIDPrefix)
	parts := strings.SplitN(trimmed, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return videoJobToken{}, invalidVideoJobIDError()
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return videoJobToken{}, invalidVideoJobIDError()
	}

	var payload videoJobToken
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return videoJobToken{}, invalidVideoJobIDError()
	}
	if payload.Version != 1 || strings.TrimSpace(payload.Provider) == "" || strings.TrimSpace(payload.Model) == "" || strings.TrimSpace(payload.ProviderJobID) == "" || strings.TrimSpace(payload.KeyID) == "" {
		return videoJobToken{}, invalidVideoJobIDError()
	}

	secret, err := videoJobSecret(snapshot, payload.Provider)
	if err != nil {
		return videoJobToken{}, invalidVideoJobIDError()
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expected, actual) {
		return videoJobToken{}, invalidVideoJobIDError()
	}

	return payload, nil
}

func videoJobSecret(snapshot *gwruntime.Snapshot, providerName string) ([]byte, error) {
	if snapshot == nil || snapshot.Config == nil {
		return nil, httputil.NewError(http.StatusServiceUnavailable, "provider_error", "registry_unavailable", "", "Runtime configuration is unavailable.")
	}

	cfg, ok := snapshot.Config.Providers[providerName]
	if !ok {
		return nil, invalidVideoJobIDError()
	}
	switch {
	case strings.TrimSpace(cfg.SecretKey) != "":
		return []byte(cfg.SecretKey), nil
	case strings.TrimSpace(cfg.APIKey) != "":
		return []byte(cfg.APIKey), nil
	case strings.TrimSpace(cfg.SpeechAPIKey) != "":
		return []byte(cfg.SpeechAPIKey), nil
	default:
		return nil, invalidVideoJobIDError()
	}
}

func invalidVideoJobIDError() error {
	return httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Video job was not found.")
}
