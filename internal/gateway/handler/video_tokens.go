package handler

import (
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
	token, err := signHMACToken(secret, videoJobIDPrefix, payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "job_id_encoding_failed", "", "Unable to issue video job id.")
	}
	return token, nil
}

func parseVideoJobID(snapshot *gwruntime.Snapshot, token string) (videoJobToken, error) {
	var payload videoJobToken
	encodedPayload, signature, err := decodeSignedToken(videoJobIDPrefix, token, &payload)
	if err != nil {
		return videoJobToken{}, invalidVideoJobIDError()
	}
	if payload.Version != 1 || strings.TrimSpace(payload.Provider) == "" || strings.TrimSpace(payload.Model) == "" || strings.TrimSpace(payload.ProviderJobID) == "" || strings.TrimSpace(payload.KeyID) == "" {
		return videoJobToken{}, invalidVideoJobIDError()
	}

	secret, err := videoJobSecret(snapshot, payload.Provider)
	if err != nil {
		return videoJobToken{}, invalidVideoJobIDError()
	}
	if err := verifyHMACSignature(secret, encodedPayload, signature); err != nil {
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

func speechSessionSecret(snapshot *gwruntime.Snapshot, providerName string) ([]byte, error) {
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
	case strings.TrimSpace(cfg.SpeechAccessToken) != "":
		return []byte(cfg.SpeechAccessToken), nil
	case strings.TrimSpace(cfg.SpeechAPIKey) != "":
		return []byte(cfg.SpeechAPIKey), nil
	case strings.TrimSpace(cfg.APIKey) != "":
		return []byte(cfg.APIKey), nil
	default:
		return nil, invalidVideoJobIDError()
	}
}

func invalidVideoJobIDError() error {
	return httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Video job was not found.")
}
