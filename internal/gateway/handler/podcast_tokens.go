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
	"github.com/JiaCheng2004/Polaris/internal/provider"
)

const podcastJobIDPrefix = "pod_"

type podcastJobToken struct {
	Version   int    `json:"v"`
	Provider  string `json:"p"`
	Model     string `json:"m"`
	CacheKey  string `json:"c"`
	KeyID     string `json:"k"`
	ExpiresAt int64  `json:"e"`
}

func signPodcastJobID(snapshot *gwruntime.Snapshot, model provider.Model, cacheKey string, keyID string, expiresAt int64) (string, error) {
	secret, err := speechSessionSecret(snapshot, model.Provider)
	if err != nil {
		return "", err
	}
	payload := podcastJobToken{
		Version:   1,
		Provider:  model.Provider,
		Model:     model.ID,
		CacheKey:  strings.TrimSpace(cacheKey),
		KeyID:     strings.TrimSpace(keyID),
		ExpiresAt: expiresAt,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "job_id_encoding_failed", "", "Unable to issue podcast job id.")
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return podcastJobIDPrefix + encodedPayload + "." + signature, nil
}

func parsePodcastJobID(snapshot *gwruntime.Snapshot, token string) (podcastJobToken, error) {
	if !strings.HasPrefix(token, podcastJobIDPrefix) {
		return podcastJobToken{}, invalidPodcastJobIDError()
	}
	trimmed := strings.TrimPrefix(token, podcastJobIDPrefix)
	parts := strings.SplitN(trimmed, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return podcastJobToken{}, invalidPodcastJobIDError()
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return podcastJobToken{}, invalidPodcastJobIDError()
	}
	var payload podcastJobToken
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return podcastJobToken{}, invalidPodcastJobIDError()
	}
	if payload.Version != 1 || strings.TrimSpace(payload.Provider) == "" || strings.TrimSpace(payload.Model) == "" || strings.TrimSpace(payload.CacheKey) == "" || strings.TrimSpace(payload.KeyID) == "" || payload.ExpiresAt <= 0 {
		return podcastJobToken{}, invalidPodcastJobIDError()
	}
	secret, err := speechSessionSecret(snapshot, payload.Provider)
	if err != nil {
		return podcastJobToken{}, invalidPodcastJobIDError()
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expected, actual) {
		return podcastJobToken{}, invalidPodcastJobIDError()
	}
	if time.Now().Unix() >= payload.ExpiresAt {
		return podcastJobToken{}, httputil.NewError(http.StatusGone, "invalid_request_error", "asset_expired", "id", "Podcast job has expired.")
	}
	return payload, nil
}

func invalidPodcastJobIDError() error {
	return httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Podcast job was not found.")
}

func newPodcastJobCacheKey() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "podcast:job:" + hex.EncodeToString(raw[:]), nil
}
