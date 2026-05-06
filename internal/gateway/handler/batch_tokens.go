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

const batchJobIDPrefix = "pl_batch_"

type batchJobToken struct {
	Version       int    `json:"v"`
	Provider      string `json:"p"`
	Model         string `json:"m"`
	ProviderJobID string `json:"j"`
	KeyID         string `json:"k"`
	ExpiresAt     int64  `json:"e"`
}

func signBatchJobID(snapshot *gwruntime.Snapshot, model provider.Model, providerJobID string, keyID string, expiresAt int64) (string, error) {
	secret, err := videoJobSecret(snapshot, model.Provider)
	if err != nil {
		return "", err
	}
	payload := batchJobToken{
		Version:       1,
		Provider:      model.Provider,
		Model:         model.ID,
		ProviderJobID: strings.TrimSpace(providerJobID),
		KeyID:         strings.TrimSpace(keyID),
		ExpiresAt:     expiresAt,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "batch_id_encoding_failed", "", "Unable to issue batch job id.")
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(encodedPayload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return batchJobIDPrefix + encodedPayload + "." + signature, nil
}

func parseBatchJobID(snapshot *gwruntime.Snapshot, token string) (batchJobToken, error) {
	if !strings.HasPrefix(token, batchJobIDPrefix) {
		return batchJobToken{}, invalidBatchJobIDError()
	}
	trimmed := strings.TrimPrefix(token, batchJobIDPrefix)
	parts := strings.SplitN(trimmed, ".", 2)
	if len(parts) != 2 {
		return batchJobToken{}, invalidBatchJobIDError()
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return batchJobToken{}, invalidBatchJobIDError()
	}
	var payload batchJobToken
	if err := json.Unmarshal(raw, &payload); err != nil {
		return batchJobToken{}, invalidBatchJobIDError()
	}
	if payload.Version != 1 || payload.Provider == "" || payload.Model == "" || payload.ProviderJobID == "" || payload.KeyID == "" || payload.ExpiresAt <= 0 {
		return batchJobToken{}, invalidBatchJobIDError()
	}
	secret, err := videoJobSecret(snapshot, payload.Provider)
	if err != nil {
		return batchJobToken{}, invalidBatchJobIDError()
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(parts[0]))
	actual, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(mac.Sum(nil), actual) {
		return batchJobToken{}, invalidBatchJobIDError()
	}
	if time.Now().Unix() >= payload.ExpiresAt {
		return batchJobToken{}, httputil.NewError(http.StatusGone, "invalid_request_error", "batch_expired", "id", "Batch job id has expired.")
	}
	return payload, nil
}

func invalidBatchJobIDError() error {
	return httputil.NewError(http.StatusNotFound, "invalid_request_error", "batch_not_found", "id", "Batch job was not found.")
}
