package guardrails

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// webhookDetector calls an external HTTP endpoint to classify text. On error or
// timeout it returns an error, which the engine handles per the policy fail_mode
// (open = skip, closed = block).
type webhookDetector struct {
	client *http.Client
}

func newWebhookDetector() *webhookDetector {
	return &webhookDetector{client: &http.Client{}}
}

func (d *webhookDetector) Name() string { return "webhook" }

type webhookRequest struct {
	Texts []string `json:"texts"`
}

type webhookResponse struct {
	Findings []struct {
		Detector string `json:"detector"`
		Type     string `json:"type"`
		Start    int    `json:"start"`
		End      int    `json:"end"`
		Severity string `json:"severity"`
	} `json:"findings"`
}

func (d *webhookDetector) Detect(ctx context.Context, text string, spec DetectorSpec) ([]Finding, error) {
	if spec.URL == "" {
		return nil, nil
	}
	callCtx := ctx
	if spec.TimeoutMs > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(spec.TimeoutMs)*time.Millisecond)
		defer cancel()
	}

	payload, err := json.Marshal(webhookRequest{Texts: []string{text}})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, spec.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("webhook detector: status %d", resp.StatusCode)
	}

	var out webhookResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}
	findings := make([]Finding, 0, len(out.Findings))
	for _, f := range out.Findings {
		typ := f.Type
		if typ == "" {
			typ = f.Detector
		}
		findings = append(findings, Finding{Detector: "webhook", Type: typ, Start: f.Start, End: f.End, Severity: normalizeSeverity(f.Severity)})
	}
	return findings, nil
}

func normalizeSeverity(s string) Severity {
	switch Severity(s) {
	case SeverityLow, SeverityMedium, SeverityHigh:
		return Severity(s)
	default:
		return SeverityMedium
	}
}
