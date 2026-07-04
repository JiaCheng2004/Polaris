package handler

import (
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
	token, err := signHMACToken(secret, batchJobIDPrefix, payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "batch_id_encoding_failed", "", "Unable to issue batch job id.")
	}
	return token, nil
}

func parseBatchJobID(snapshot *gwruntime.Snapshot, token string) (batchJobToken, error) {
	var payload batchJobToken
	encodedPayload, signature, err := decodeSignedToken(batchJobIDPrefix, token, &payload)
	if err != nil {
		return batchJobToken{}, invalidBatchJobIDError()
	}
	if payload.Version != 1 || payload.Provider == "" || payload.Model == "" || payload.ProviderJobID == "" || payload.KeyID == "" || payload.ExpiresAt <= 0 {
		return batchJobToken{}, invalidBatchJobIDError()
	}
	secret, err := videoJobSecret(snapshot, payload.Provider)
	if err != nil {
		return batchJobToken{}, invalidBatchJobIDError()
	}
	if err := verifyHMACSignature(secret, encodedPayload, signature); err != nil {
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
