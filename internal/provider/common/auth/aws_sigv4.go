package auth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const awsAlgorithm = "AWS4-HMAC-SHA256"

func SignAWSRequest(req *http.Request, payload []byte, service string, region string, accessKeyID string, accessKeySecret string, sessionToken string, now time.Time) error {
	if strings.TrimSpace(accessKeyID) == "" || strings.TrimSpace(accessKeySecret) == "" {
		return fmt.Errorf("aws access key id and secret are required")
	}
	if strings.TrimSpace(service) == "" || strings.TrimSpace(region) == "" {
		return fmt.Errorf("aws service and region are required")
	}

	amzDate := now.UTC().Format("20060102T150405Z")
	dateStamp := now.UTC().Format("20060102")
	payloadHash := sha256Hex(payload)

	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	if strings.TrimSpace(sessionToken) != "" {
		req.Header.Set("X-Amz-Security-Token", strings.TrimSpace(sessionToken))
	}
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.URL.Host)
	}

	canonicalRequest, signedHeaders := canonicalRequest(req, payloadHash)
	credentialScope := dateStamp + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		awsAlgorithm,
		amzDate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := awsSigningKey(accessKeySecret, dateStamp, region, service)
	signature := hmacHex(signingKey, stringToSign)

	req.Header.Set("Authorization", strings.Join([]string{
		awsAlgorithm + " Credential=" + accessKeyID + "/" + credentialScope,
		"SignedHeaders=" + signedHeaders,
		"Signature=" + signature,
	}, ", "))
	return nil
}

func canonicalRequest(req *http.Request, payloadHash string) (string, string) {
	headerKeys := make([]string, 0, len(req.Header)+1)
	headerValues := make(map[string]string, len(req.Header)+1)

	headerValues["host"] = strings.TrimSpace(req.URL.Host)
	headerKeys = append(headerKeys, "host")

	for key, values := range req.Header {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if lowerKey == "authorization" || lowerKey == "host" {
			continue
		}
		headerKeys = append(headerKeys, lowerKey)
		headerValues[lowerKey] = canonicalHeaderValue(values)
	}

	sort.Strings(headerKeys)
	headerKeys = uniqueStrings(headerKeys)

	var canonicalHeaders strings.Builder
	for _, key := range headerKeys {
		canonicalHeaders.WriteString(key)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(headerValues[key])
		canonicalHeaders.WriteByte('\n')
	}

	signedHeaders := strings.Join(headerKeys, ";")
	return strings.Join([]string{
		req.Method,
		canonicalPath(req.URL),
		canonicalQuery(req.URL),
		canonicalHeaders.String(),
		signedHeaders,
		payloadHash,
	}, "\n"), signedHeaders
}

func canonicalPath(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func canonicalQuery(u *url.URL) string {
	if u == nil || u.RawQuery == "" {
		return ""
	}
	query := u.Query()
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	pairs := make([]string, 0)
	for _, key := range keys {
		values := query[key]
		if len(values) == 0 {
			pairs = append(pairs, awsQueryEscape(key)+"=")
			continue
		}
		sort.Strings(values)
		for _, value := range values {
			pairs = append(pairs, awsQueryEscape(key)+"="+awsQueryEscape(value))
		}
	}
	return strings.Join(pairs, "&")
}

func awsQueryEscape(value string) string {
	escaped := url.QueryEscape(value)
	escaped = strings.ReplaceAll(escaped, "+", "%20")
	escaped = strings.ReplaceAll(escaped, "*", "%2A")
	return strings.ReplaceAll(escaped, "%7E", "~")
}

func canonicalHeaderValue(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	}
	return strings.Join(parts, ",")
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := values[:0]
	var previous string
	for _, value := range values {
		if value == previous {
			continue
		}
		out = append(out, value)
		previous = value
	}
	return out
}

func awsSigningKey(secret string, date string, region string, service string) []byte {
	seed := []byte("AWS4" + secret)
	dateKey := hmacBytes(seed, date)
	regionKey := hmacBytes(dateKey, region)
	serviceKey := hmacBytes(regionKey, service)
	return hmacBytes(serviceKey, "aws4_request")
}

func hmacBytes(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func hmacHex(key []byte, value string) string {
	return hex.EncodeToString(hmacBytes(key, value))
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func CanonicalRequestForTest(req *http.Request, payload []byte, service string, region string, accessKeyID string, accessKeySecret string, sessionToken string, now time.Time) (string, error) {
	clone := req.Clone(req.Context())
	clone.Header = clone.Header.Clone()
	if err := SignAWSRequest(clone, payload, service, region, accessKeyID, accessKeySecret, sessionToken, now); err != nil {
		return "", err
	}
	canonicalRequest, _ := canonicalRequest(clone, sha256Hex(payload))
	return canonicalRequest, nil
}

func AuthorizationHeaderForTest(req *http.Request, payload []byte, service string, region string, accessKeyID string, accessKeySecret string, sessionToken string, now time.Time) (string, error) {
	clone := req.Clone(req.Context())
	clone.Header = clone.Header.Clone()
	if err := SignAWSRequest(clone, payload, service, region, accessKeyID, accessKeySecret, sessionToken, now); err != nil {
		return "", err
	}
	return clone.Header.Get("Authorization"), nil
}

func HashedPayloadForTest(payload []byte) string {
	return sha256Hex(bytes.Clone(payload))
}
