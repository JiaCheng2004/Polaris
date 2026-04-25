package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSignAWSRequestAddsExpectedHeaders(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://bedrock-runtime.us-east-1.amazonaws.com/model/amazon.nova-2-lite-v1%3A0/converse", strings.NewReader(`{"messages":[]}`))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	now := time.Date(2026, time.April, 22, 18, 30, 0, 0, time.UTC)
	payload := []byte(`{"messages":[]}`)
	if err := SignAWSRequest(req, payload, "bedrock", "us-east-1", "AKIAEXAMPLE", "secret", "session-token", now); err != nil {
		t.Fatalf("SignAWSRequest() error = %v", err)
	}

	if got := req.Header.Get("X-Amz-Date"); got != "20260422T183000Z" {
		t.Fatalf("X-Amz-Date = %q", got)
	}
	if got := req.Header.Get("X-Amz-Security-Token"); got != "session-token" {
		t.Fatalf("X-Amz-Security-Token = %q", got)
	}
	authHeader := req.Header.Get("Authorization")
	if !strings.Contains(authHeader, "Credential=AKIAEXAMPLE/20260422/us-east-1/bedrock/aws4_request") {
		t.Fatalf("Authorization missing credential scope: %q", authHeader)
	}
	if !strings.Contains(authHeader, "SignedHeaders=") || !strings.Contains(authHeader, "Signature=") {
		t.Fatalf("Authorization missing signed headers or signature: %q", authHeader)
	}
}
