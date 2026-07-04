package handler

import (
	"crypto/rand"
	"encoding/hex"
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
	signed, err := signHMACToken(secret, podcastJobIDPrefix, payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "job_id_encoding_failed", "", "Unable to issue podcast job id.")
	}
	return signed, nil
}

func parsePodcastJobID(snapshot *gwruntime.Snapshot, token string) (podcastJobToken, error) {
	var payload podcastJobToken
	encodedPayload, signature, err := decodeSignedToken(podcastJobIDPrefix, token, &payload)
	if err != nil {
		return podcastJobToken{}, invalidPodcastJobIDError()
	}
	if payload.Version != 1 || strings.TrimSpace(payload.Provider) == "" || strings.TrimSpace(payload.Model) == "" || strings.TrimSpace(payload.CacheKey) == "" || strings.TrimSpace(payload.KeyID) == "" || payload.ExpiresAt <= 0 {
		return podcastJobToken{}, invalidPodcastJobIDError()
	}
	secret, err := speechSessionSecret(snapshot, payload.Provider)
	if err != nil {
		return podcastJobToken{}, invalidPodcastJobIDError()
	}
	if err := verifyHMACSignature(secret, encodedPayload, signature); err != nil {
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
