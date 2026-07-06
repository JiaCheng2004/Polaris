package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// CreateProject creates a project.
func (c *Client) CreateProject(ctx context.Context, req *CreateProjectRequest) (*Project, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response Project
	if err := c.doJSON(ctx, http.MethodPost, "/v1/projects", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// ListProjects lists projects.
func (c *Client) ListProjects(ctx context.Context, includeArchived bool) (*ProjectList, error) {
	query := url.Values{}
	if includeArchived {
		query.Set("include_archived", "true")
	}
	var response ProjectList
	if err := c.doJSON(ctx, http.MethodGet, "/v1/projects", query, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// CreateVirtualKey creates a virtual key.
func (c *Client) CreateVirtualKey(ctx context.Context, req *CreateVirtualKeyRequest) (*VirtualKey, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response VirtualKey
	if err := c.doJSON(ctx, http.MethodPost, "/v1/virtual_keys", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// ListVirtualKeys lists virtual keys.
func (c *Client) ListVirtualKeys(ctx context.Context, params *ListVirtualKeysParams) (*VirtualKeyList, error) {
	query := url.Values{}
	if params != nil {
		if strings.TrimSpace(params.ProjectID) != "" {
			query.Set("project_id", strings.TrimSpace(params.ProjectID))
		}
		if params.IncludeRevoked != nil {
			query.Set("include_revoked", strconv.FormatBool(*params.IncludeRevoked))
		}
	}
	var response VirtualKeyList
	if err := c.doJSON(ctx, http.MethodGet, "/v1/virtual_keys", query, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// DeleteVirtualKey deletes the virtual key.
func (c *Client) DeleteVirtualKey(ctx context.Context, id string) error {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return fmt.Errorf("id is required")
	}
	resp, err := c.do(ctx, http.MethodDelete, "/v1/virtual_keys/"+trimmedID, nil, nil, "", "application/json")
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode >= http.StatusBadRequest {
		return decodeAPIErrorResponse(resp)
	}
	return nil
}

// CreatePolicy creates a policy.
func (c *Client) CreatePolicy(ctx context.Context, req *CreatePolicyRequest) (*Policy, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response Policy
	if err := c.doJSON(ctx, http.MethodPost, "/v1/policies", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// ListPolicies lists policies.
func (c *Client) ListPolicies(ctx context.Context, projectID string) (*PolicyList, error) {
	query := url.Values{}
	if trimmed := strings.TrimSpace(projectID); trimmed != "" {
		query.Set("project_id", trimmed)
	}
	var response PolicyList
	if err := c.doJSON(ctx, http.MethodGet, "/v1/policies", query, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// CreateBudget creates a budget.
func (c *Client) CreateBudget(ctx context.Context, req *CreateBudgetRequest) (*Budget, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response Budget
	if err := c.doJSON(ctx, http.MethodPost, "/v1/budgets", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// ListBudgets lists budgets.
func (c *Client) ListBudgets(ctx context.Context, projectID string) (*BudgetList, error) {
	query := url.Values{}
	if trimmed := strings.TrimSpace(projectID); trimmed != "" {
		query.Set("project_id", trimmed)
	}
	var response BudgetList
	if err := c.doJSON(ctx, http.MethodGet, "/v1/budgets", query, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// CreateTool creates a tool.
func (c *Client) CreateTool(ctx context.Context, req *CreateToolRequest) (*ToolDefinitionResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response ToolDefinitionResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/tools", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// ListTools lists tools.
func (c *Client) ListTools(ctx context.Context) (*ToolDefinitionList, error) {
	var response ToolDefinitionList
	if err := c.doJSON(ctx, http.MethodGet, "/v1/tools", nil, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// CreateToolset creates a toolset.
func (c *Client) CreateToolset(ctx context.Context, req *CreateToolsetRequest) (*Toolset, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response Toolset
	if err := c.doJSON(ctx, http.MethodPost, "/v1/toolsets", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// ListToolsets lists toolsets.
func (c *Client) ListToolsets(ctx context.Context) (*ToolsetList, error) {
	var response ToolsetList
	if err := c.doJSON(ctx, http.MethodGet, "/v1/toolsets", nil, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// CreateMCPBinding creates a mcp binding.
func (c *Client) CreateMCPBinding(ctx context.Context, req *CreateMCPBindingRequest) (*MCPBinding, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	var response MCPBinding
	if err := c.doJSON(ctx, http.MethodPost, "/v1/mcp/bindings", nil, req, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// ListMCPBindings lists mcp bindings.
func (c *Client) ListMCPBindings(ctx context.Context) (*MCPBindingList, error) {
	var response MCPBindingList
	if err := c.doJSON(ctx, http.MethodGet, "/v1/mcp/bindings", nil, nil, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
