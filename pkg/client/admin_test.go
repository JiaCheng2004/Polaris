package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("include_aliases"); got != "true" {
			t.Fatalf("unexpected include_aliases %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"list",
			"data":[
				{"id":"openai/gpt-4o","object":"model","provider":"openai","modality":"chat","capabilities":["streaming"],"aliases":["default-chat"],"capability_flags":{"chat":true,"streaming":true},"lifecycle":{"enabled":true,"status":"ga","stability":"stable"},"billing":{"billing_mode":"usage_based","unit":"token","currency":"USD","rates":{"input_per_mtok":2.5,"output_per_mtok":10}}},
				{"id":"default-chat","object":"model","provider":"openai","modality":"chat","capabilities":["streaming"],"resolves_to":"openai/gpt-4o"}
			],
			"routing":{"defaults":{"chat":{"alias":"default-chat","model":"openai/gpt-4o","provider":"openai","modality":"chat"}}}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.ListModels(context.Background(), true)
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(response.Data) != 2 || response.Data[1].ResolvesTo != "openai/gpt-4o" {
		t.Fatalf("unexpected response %#v", response)
	}
	if !response.Data[0].CapabilityFlags.Chat || response.Data[0].Billing == nil || response.Data[0].Billing.Rates["input_per_mtok"] != 2.5 {
		t.Fatalf("unexpected model metadata %#v", response.Data[0])
	}
	if response.Routing == nil || response.Routing.Defaults["chat"].Model != "openai/gpt-4o" {
		t.Fatalf("unexpected routing metadata %#v", response.Routing)
	}
}

func TestListModelCapabilities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/model-capabilities" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("include_aliases"); got != "true" {
			t.Fatalf("unexpected include_aliases %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"object":"model_capabilities.list",
			"data":[{"id":"openai/gpt-4o","object":"model","provider":"openai","modality":"chat","capability_flags":{"chat":true}}],
			"routing":{"defaults":{"chat":{"alias":"default-chat","model":"openai/gpt-4o"}}}
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.ListModelCapabilities(context.Background(), true)
	if err != nil {
		t.Fatalf("ListModelCapabilities() error = %v", err)
	}
	if response.Object != "model_capabilities.list" || !response.Data[0].CapabilityFlags.Chat {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestGetUsage(t *testing.T) {
	from := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.April, 2, 0, 0, 0, 0, time.UTC)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("group_by") != "model" || query.Get("modality") != "embed" || query.Get("model") != "openai/text-embedding-3-small" {
			t.Fatalf("unexpected query %#v", query)
		}
		if query.Get("from") != from.Format(time.RFC3339) || query.Get("to") != to.Format(time.RFC3339) {
			t.Fatalf("unexpected time range %#v", query)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"from":"2026-04-01T00:00:00Z",
			"to":"2026-04-02T00:00:00Z",
			"total_requests":1,
			"total_tokens":8,
			"total_cost_usd":0.00000016,
			"by_model":[{"model":"openai/text-embedding-3-small","requests":1,"tokens":8,"cost_usd":0.00000016}]
		}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	response, err := client.GetUsage(context.Background(), &UsageParams{
		From:     &from,
		To:       &to,
		Model:    "openai/text-embedding-3-small",
		Modality: "embed",
		GroupBy:  "model",
	})
	if err != nil {
		t.Fatalf("GetUsage() error = %v", err)
	}
	if response.TotalTokens != 8 || len(response.ByModel) != 1 {
		t.Fatalf("unexpected response %#v", response)
	}
}

func TestCreateListDeleteKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case http.MethodPost + " /v1/keys":
			var payload CreateKeyRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode request: %v", err)
			}
			if payload.Name != "worker" {
				t.Fatalf("unexpected payload %#v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"id":"key_123",
				"name":"worker",
				"key":"polaris-sk-live-123",
				"key_prefix":"polaris-",
				"allowed_models":["*"],
				"is_admin":false,
				"created_at":"2026-04-11T12:34:56Z"
			}`))
		case http.MethodGet + " /v1/keys":
			if got := r.URL.Query().Get("include_revoked"); got != "true" {
				t.Fatalf("unexpected include_revoked %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"object":"list",
				"data":[{"id":"key_123","name":"worker","key_prefix":"polaris-","allowed_models":["*"],"is_admin":false,"created_at":"2026-04-11T12:34:56Z"}]
			}`))
		case http.MethodDelete + " /v1/keys/key_123":
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected route %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	created, err := client.CreateKey(context.Background(), &CreateKeyRequest{Name: "worker"})
	if err != nil {
		t.Fatalf("CreateKey() error = %v", err)
	}
	if created.Key != "polaris-sk-live-123" {
		t.Fatalf("unexpected created key %#v", created)
	}

	listed, err := client.ListKeys(context.Background(), &ListKeysParams{IncludeRevoked: boolPtr(true)})
	if err != nil {
		t.Fatalf("ListKeys() error = %v", err)
	}
	if len(listed.Data) != 1 || listed.Data[0].ID != "key_123" {
		t.Fatalf("unexpected listed keys %#v", listed)
	}

	if err := client.DeleteKey(context.Background(), "key_123"); err != nil {
		t.Fatalf("DeleteKey() error = %v", err)
	}
}
