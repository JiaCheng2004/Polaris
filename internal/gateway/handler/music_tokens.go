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

const musicJobIDPrefix = "mus_"

type musicJobToken struct {
	Version   int    `json:"v"`
	Provider  string `json:"p"`
	Model     string `json:"m"`
	Operation string `json:"o"`
	CacheKey  string `json:"c"`
	KeyID     string `json:"k"`
	ExpiresAt int64  `json:"e"`
}

func signMusicJobID(snapshot *gwruntime.Snapshot, model provider.Model, operation string, cacheKey string, keyID string, expiresAt int64) (string, error) {
	secret, err := videoJobSecret(snapshot, model.Provider)
	if err != nil {
		return "", err
	}

	payload := musicJobToken{
		Version:   1,
		Provider:  model.Provider,
		Model:     model.ID,
		Operation: strings.TrimSpace(operation),
		CacheKey:  strings.TrimSpace(cacheKey),
		KeyID:     strings.TrimSpace(keyID),
		ExpiresAt: expiresAt,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "job_id_encoding_failed", "", "Unable to issue music job id.")
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return musicJobIDPrefix + encodedPayload + "." + signature, nil
}

func parseMusicJobID(snapshot *gwruntime.Snapshot, token string) (musicJobToken, error) {
	if !strings.HasPrefix(token, musicJobIDPrefix) {
		return musicJobToken{}, invalidMusicJobIDError()
	}

	trimmed := strings.TrimPrefix(token, musicJobIDPrefix)
	parts := strings.SplitN(trimmed, ".", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return musicJobToken{}, invalidMusicJobIDError()
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return musicJobToken{}, invalidMusicJobIDError()
	}

	var payload musicJobToken
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return musicJobToken{}, invalidMusicJobIDError()
	}
	if payload.Version != 1 ||
		strings.TrimSpace(payload.Provider) == "" ||
		strings.TrimSpace(payload.Model) == "" ||
		strings.TrimSpace(payload.Operation) == "" ||
		strings.TrimSpace(payload.CacheKey) == "" ||
		strings.TrimSpace(payload.KeyID) == "" ||
		payload.ExpiresAt <= 0 {
		return musicJobToken{}, invalidMusicJobIDError()
	}

	secret, err := videoJobSecret(snapshot, payload.Provider)
	if err != nil {
		return musicJobToken{}, invalidMusicJobIDError()
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0]))
	expected := mac.Sum(nil)
	actual, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(expected, actual) {
		return musicJobToken{}, invalidMusicJobIDError()
	}
	if time.Now().Unix() >= payload.ExpiresAt {
		return musicJobToken{}, httputil.NewError(http.StatusGone, "invalid_request_error", "asset_expired", "id", "Music job has expired.")
	}

	return payload, nil
}

func invalidMusicJobIDError() error {
	return httputil.NewError(http.StatusNotFound, "invalid_request_error", "job_not_found", "id", "Music job was not found.")
}

func newMusicJobCacheKey() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "music:job:" + hex.EncodeToString(raw[:]), nil
}
