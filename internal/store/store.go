package store

import (
	"context"
	"time"
)

type Store interface {
	CreateAPIKey(ctx context.Context, key APIKey) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error)
	ListAPIKeys(ctx context.Context, ownerID string, includeRevoked bool) ([]APIKey, error)
	DeleteAPIKey(ctx context.Context, id string) error
	UpdateAPIKeyLastUsed(ctx context.Context, id string, usedAt time.Time) error

	CreateProject(ctx context.Context, project Project) error
	ListProjects(ctx context.Context, includeArchived bool) ([]Project, error)
	GetProject(ctx context.Context, id string) (*Project, error)

	CreateVirtualKey(ctx context.Context, key VirtualKey) error
	GetVirtualKeyByHash(ctx context.Context, keyHash string) (*VirtualKey, error)
	ListVirtualKeys(ctx context.Context, projectID string, includeRevoked bool) ([]VirtualKey, error)
	DeleteVirtualKey(ctx context.Context, id string) error
	UpdateVirtualKeyLastUsed(ctx context.Context, id string, usedAt time.Time) error

	CreatePolicy(ctx context.Context, policy Policy) error
	ListPolicies(ctx context.Context, projectID string) ([]Policy, error)

	CreateBudget(ctx context.Context, budget Budget) error
	ListBudgets(ctx context.Context, projectID string) ([]Budget, error)

	LogAuditEvent(ctx context.Context, event AuditEvent) error
	LogAuditEventBatch(ctx context.Context, events []AuditEvent) error

	CreateToolDefinition(ctx context.Context, tool ToolDefinition) error
	ListToolDefinitions(ctx context.Context) ([]ToolDefinition, error)
	GetToolDefinition(ctx context.Context, id string) (*ToolDefinition, error)

	CreateToolset(ctx context.Context, toolset Toolset) error
	ListToolsets(ctx context.Context) ([]Toolset, error)
	GetToolset(ctx context.Context, id string) (*Toolset, error)

	CreateMCPBinding(ctx context.Context, binding MCPBinding) error
	ListMCPBindings(ctx context.Context) ([]MCPBinding, error)
	GetMCPBinding(ctx context.Context, id string) (*MCPBinding, error)

	ArchiveVoice(ctx context.Context, voice ArchivedVoice) error
	UnarchiveVoice(ctx context.Context, provider string, model string, voiceID string) error
	GetArchivedVoice(ctx context.Context, provider string, model string, voiceID string) (*ArchivedVoice, error)
	ListArchivedVoices(ctx context.Context, provider string, model string) ([]ArchivedVoice, error)

	LogRequest(ctx context.Context, log RequestLog) error
	LogRequestBatch(ctx context.Context, logs []RequestLog) error

	GetUsage(ctx context.Context, filter UsageFilter) (UsageReport, error)
	GetUsageByModel(ctx context.Context, filter UsageFilter) (UsageReport, error)

	PurgeOldLogs(ctx context.Context, olderThan time.Time) (int64, error)
	Migrate(ctx context.Context) error
	Ping(ctx context.Context) error
	Close() error
}
