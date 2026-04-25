package bytedance

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
)

const (
	bytedanceSpeechControlRegion  = "cn-beijing"
	bytedanceSpeechControlService = "speech_saas_prod"
)

type bytedanceControlErrorEnvelope struct {
	ResponseMetadata struct {
		Error *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error,omitempty"`
		RequestID string `json:"RequestId,omitempty"`
	} `json:"ResponseMetadata"`
}

func (c *Client) speechControlJSON(ctx context.Context, action string, version string, body any, out any) error {
	if strings.TrimSpace(c.accessKeyID) == "" || strings.TrimSpace(c.accessKeySecret) == "" {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_misconfigured", "", "ByteDance speech control APIs require providers.bytedance.access_key_id and providers.bytedance.access_key_secret.")
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal bytedance speech control request: %w", err)
	}

	query := url.Values{}
	query.Set("Action", action)
	query.Set("Version", version)

	xDate := time.Now().UTC().Format("20060102T150405Z")
	shortDate := xDate[:8]
	payloadHash := sha256Hex(payload)
	host := controlHost(c.controlBaseURL)
	signedHeaders := "content-type;host;x-content-sha256;x-date"
	canonicalHeaders := strings.Join([]string{
		"content-type:application/json",
		"host:" + host,
		"x-content-sha256:" + payloadHash,
		"x-date:" + xDate,
		"",
	}, "\n")
	canonicalRequest := strings.Join([]string{
		http.MethodPost,
		"/",
		canonicalQueryString(query),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")
	credentialScope := shortDate + "/" + bytedanceSpeechControlRegion + "/" + bytedanceSpeechControlService + "/request"
	stringToSign := strings.Join([]string{
		"HMAC-SHA256",
		xDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(deriveSigningKey(c.accessKeySecret, shortDate, bytedanceSpeechControlRegion, bytedanceSpeechControlService), stringToSign))
	authorization := fmt.Sprintf(
		"HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		c.accessKeyID,
		credentialScope,
		signedHeaders,
		signature,
	)

	requestURL := strings.TrimRight(c.controlBaseURL, "/") + "/?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build bytedance speech control request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Date", xDate)
	req.Header.Set("X-Content-Sha256", payloadHash)
	req.Header.Set("Authorization", authorization)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return translateTransportError(err, "ByteDance")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance speech control returned an unreadable response.")
	}

	var envelope bytedanceControlErrorEnvelope
	_ = json.Unmarshal(raw, &envelope)
	if resp.StatusCode >= http.StatusBadRequest {
		return bytedanceSpeechControlError(resp.StatusCode, envelope, raw)
	}
	if envelope.ResponseMetadata.Error != nil {
		return bytedanceSpeechControlError(http.StatusBadGateway, envelope, raw)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", "ByteDance speech control returned an invalid JSON response.")
	}
	return nil
}

func bytedanceSpeechControlError(status int, envelope bytedanceControlErrorEnvelope, raw []byte) error {
	code := ""
	message := strings.TrimSpace(string(raw))
	if envelope.ResponseMetadata.Error != nil {
		code = strings.TrimSpace(envelope.ResponseMetadata.Error.Code)
		if strings.TrimSpace(envelope.ResponseMetadata.Error.Message) != "" {
			message = strings.TrimSpace(envelope.ResponseMetadata.Error.Message)
		}
	}
	return httputil.ProviderAPIError("ByteDance", status, httputil.ProviderErrorDetails{
		Message: message,
		Code:    code,
		Body:    string(raw),
	})
}

func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		encodedKey := percentEncode(key)
		items := append([]string(nil), values[key]...)
		sort.Strings(items)
		for _, item := range items {
			pairs = append(pairs, encodedKey+"="+percentEncode(item))
		}
	}
	return strings.Join(pairs, "&")
}

func percentEncode(value string) string {
	escaped := url.QueryEscape(value)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "*", "%2A")
	return strings.ReplaceAll(escaped, "%7E", "~")
}

func controlHost(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" {
		return "open.volcengineapi.com"
	}
	return parsed.Host
}

func deriveSigningKey(secret string, shortDate string, region string, service string) []byte {
	dateKey := hmacSHA256([]byte(secret), shortDate)
	regionKey := hmacSHA256(dateKey, region)
	serviceKey := hmacSHA256(regionKey, service)
	return hmacSHA256(serviceKey, "request")
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
