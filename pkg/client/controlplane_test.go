package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestControlPlaneClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case http.MethodPost + " /v1/projects":
			var payload CreateProjectRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode project request: %v", err)
			}
			if payload.Name != "Acme" {
				t.Fatalf("unexpected project payload %#v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"proj_123","name":"Acme","created_at":"2026-04-20T12:34:56Z"}`))
		case http.MethodGet + " /v1/projects":
			if got := r.URL.Query().Get("include_archived"); got != "true" {
				t.Fatalf("unexpected include_archived %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"proj_123","name":"Acme","created_at":"2026-04-20T12:34:56Z"}]}`))
		case http.MethodPost + " /v1/virtual_keys":
			var payload CreateVirtualKeyRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode virtual key request: %v", err)
			}
			if payload.ProjectID != "proj_123" || payload.Name != "worker" {
				t.Fatalf("unexpected virtual key payload %#v", payload)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"vk_123","project_id":"proj_123","name":"worker","key":"polaris-sk-live-123","key_prefix":"polaris-","allowed_models":["*"],"created_at":"2026-04-20T12:34:56Z"}`))
		case http.MethodGet + " /v1/virtual_keys":
			if got := r.URL.Query().Get("project_id"); got != "proj_123" {
				t.Fatalf("unexpected project_id %q", got)
			}
			if got := r.URL.Query().Get("include_revoked"); got != "true" {
				t.Fatalf("unexpected include_revoked %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"vk_123","project_id":"proj_123","name":"worker","key_prefix":"polaris-","allowed_models":["*"],"created_at":"2026-04-20T12:34:56Z"}]}`))
		case http.MethodDelete + " /v1/virtual_keys/vk_123":
			w.WriteHeader(http.StatusNoContent)
		case http.MethodPost + " /v1/policies":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"pol_123","project_id":"proj_123","name":"allow-core","allowed_models":["openai/*"],"created_at":"2026-04-20T12:34:56Z"}`))
		case http.MethodPost + " /v1/budgets":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"bud_123","project_id":"proj_123","name":"monthly","mode":"soft","window":"monthly","created_at":"2026-04-20T12:34:56Z"}`))
		case http.MethodPost + " /v1/tools":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"tool_123","name":"Echo","implementation":"echo","enabled":true,"created_at":"2026-04-20T12:34:56Z"}`))
		case http.MethodPost + " /v1/toolsets":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ts_123","name":"Core","tool_ids":["tool_123"],"created_at":"2026-04-20T12:34:56Z"}`))
		case http.MethodPost + " /v1/mcp/bindings":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"mcp_123","name":"Local Tools","kind":"local_toolset","toolset_id":"ts_123","enabled":true,"created_at":"2026-04-20T12:34:56Z"}`))
		default:
			t.Fatalf("unexpected route %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	project, err := client.CreateProject(context.Background(), &CreateProjectRequest{Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateProject() error = %v", err)
	}
	if project.ID != "proj_123" {
		t.Fatalf("unexpected project %#v", project)
	}

	projects, err := client.ListProjects(context.Background(), true)
	if err != nil {
		t.Fatalf("ListProjects() error = %v", err)
	}
	if len(projects.Data) != 1 || projects.Data[0].ID != "proj_123" {
		t.Fatalf("unexpected project list %#v", projects)
	}

	virtualKey, err := client.CreateVirtualKey(context.Background(), &CreateVirtualKeyRequest{
		ProjectID: "proj_123",
		Name:      "worker",
	})
	if err != nil {
		t.Fatalf("CreateVirtualKey() error = %v", err)
	}
	if virtualKey.Key != "polaris-sk-live-123" {
		t.Fatalf("unexpected virtual key %#v", virtualKey)
	}

	keys, err := client.ListVirtualKeys(context.Background(), &ListVirtualKeysParams{
		ProjectID:      "proj_123",
		IncludeRevoked: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("ListVirtualKeys() error = %v", err)
	}
	if len(keys.Data) != 1 || keys.Data[0].ID != "vk_123" {
		t.Fatalf("unexpected virtual key list %#v", keys)
	}

	if err := client.DeleteVirtualKey(context.Background(), "vk_123"); err != nil {
		t.Fatalf("DeleteVirtualKey() error = %v", err)
	}

	if _, err := client.CreatePolicy(context.Background(), &CreatePolicyRequest{
		ProjectID:     "proj_123",
		Name:          "allow-core",
		AllowedModels: []string{"openai/*"},
	}); err != nil {
		t.Fatalf("CreatePolicy() error = %v", err)
	}

	if _, err := client.CreateBudget(context.Background(), &CreateBudgetRequest{
		ProjectID: "proj_123",
		Name:      "monthly",
		Mode:      "soft",
	}); err != nil {
		t.Fatalf("CreateBudget() error = %v", err)
	}

	if _, err := client.CreateTool(context.Background(), &CreateToolRequest{
		Name:           "Echo",
		Implementation: "echo",
	}); err != nil {
		t.Fatalf("CreateTool() error = %v", err)
	}

	if _, err := client.CreateToolset(context.Background(), &CreateToolsetRequest{
		Name:    "Core",
		ToolIDs: []string{"tool_123"},
	}); err != nil {
		t.Fatalf("CreateToolset() error = %v", err)
	}

	if _, err := client.CreateMCPBinding(context.Background(), &CreateMCPBindingRequest{
		Name:      "Local Tools",
		Kind:      "local_toolset",
		ToolsetID: "ts_123",
	}); err != nil {
		t.Fatalf("CreateMCPBinding() error = %v", err)
	}
}
