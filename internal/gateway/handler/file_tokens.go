package handler

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
)

const fileDownloadTokenPrefix = "flt_"

type fileDownloadToken struct {
	Version   int    `json:"v"`
	PolarisID string `json:"f"`
	ProjectID string `json:"p"`
	KeyID     string `json:"k"`
	ExpiresAt int64  `json:"e"`
}

func signFileDownloadToken(polarisID, projectID, keyID string, expiresAt int64) (string, error) {
	secret := fileDownloadSecret()
	if len(secret) == 0 {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "file_download_secret_missing", "", "File download token secret is not configured.")
	}
	payload := fileDownloadToken{
		Version:   1,
		PolarisID: strings.TrimSpace(polarisID),
		ProjectID: strings.TrimSpace(projectID),
		KeyID:     strings.TrimSpace(keyID),
		ExpiresAt: expiresAt,
	}
	signed, err := signHMACToken(secret, fileDownloadTokenPrefix, payload)
	if err != nil {
		return "", httputil.NewError(http.StatusInternalServerError, "internal_error", "file_token_encoding_failed", "", "Unable to issue file download token.")
	}
	return signed, nil
}

func parseFileDownloadToken(token string) (fileDownloadToken, error) {
	secret := fileDownloadSecret()
	if len(secret) == 0 {
		return fileDownloadToken{}, invalidFileDownloadTokenError()
	}
	var payload fileDownloadToken
	encodedPayload, signature, err := decodeSignedToken(fileDownloadTokenPrefix, token, &payload)
	if err != nil {
		return fileDownloadToken{}, invalidFileDownloadTokenError()
	}
	if payload.Version != 1 ||
		strings.TrimSpace(payload.PolarisID) == "" ||
		strings.TrimSpace(payload.ProjectID) == "" ||
		strings.TrimSpace(payload.KeyID) == "" ||
		payload.ExpiresAt <= 0 {
		return fileDownloadToken{}, invalidFileDownloadTokenError()
	}
	if err := verifyHMACSignature(secret, encodedPayload, signature); err != nil {
		return fileDownloadToken{}, invalidFileDownloadTokenError()
	}
	if time.Now().Unix() >= payload.ExpiresAt {
		return fileDownloadToken{}, httputil.NewError(http.StatusGone, "invalid_request_error", "file_expired", "token", "File download token has expired.")
	}
	return payload, nil
}

func invalidFileDownloadTokenError() error {
	return httputil.NewError(http.StatusNotFound, "invalid_request_error", "file_not_found", "token", "File content was not found.")
}

func fileDownloadSecret() []byte {
	secret := strings.TrimSpace(os.Getenv("POLARIS_FILE_DOWNLOAD_SECRET"))
	if secret == "" {
		return nil
	}
	return []byte(secret)
}
