package store

import (
	"context"
	"time"
)

// The Store contract is composed from focused role interfaces so callers can
// depend on the narrow surface they use (and be tested with small fakes). The
// concrete sqlite/postgres implementations satisfy the full Store; compile-time
// assertions live in those packages.

// KeyStore manages static/owner API keys.
type KeyStore interface {
	CreateAPIKey(ctx context.Context, key APIKey) error
	GetAPIKeyByHash(ctx context.Context, keyHash string) (*APIKey, error)
	ListAPIKeys(ctx context.Context, ownerID string, includeRevoked bool) ([]APIKey, error)
	DeleteAPIKey(ctx context.Context, id string) error
	UpdateAPIKeyLastUsed(ctx context.Context, id string, usedAt time.Time) error
}

// ProjectStore manages projects.
type ProjectStore interface {
	CreateProject(ctx context.Context, project Project) error
	ListProjects(ctx context.Context, includeArchived bool) ([]Project, error)
	GetProject(ctx context.Context, id string) (*Project, error)
}

// VirtualKeyStore manages per-project virtual keys.
type VirtualKeyStore interface {
	CreateVirtualKey(ctx context.Context, key VirtualKey) error
	GetVirtualKeyByHash(ctx context.Context, keyHash string) (*VirtualKey, error)
	ListVirtualKeys(ctx context.Context, projectID string, includeRevoked bool) ([]VirtualKey, error)
	DeleteVirtualKey(ctx context.Context, id string) error
	UpdateVirtualKeyLastUsed(ctx context.Context, id string, usedAt time.Time) error
}

// PolicyStore manages project policies.
type PolicyStore interface {
	CreatePolicy(ctx context.Context, policy Policy) error
	ListPolicies(ctx context.Context, projectID string) ([]Policy, error)
}

// BudgetStore manages project budgets.
type BudgetStore interface {
	CreateBudget(ctx context.Context, budget Budget) error
	ListBudgets(ctx context.Context, projectID string) ([]Budget, error)
}

// FileStore manages uploaded files, provider handles, and understanding artifacts.
type FileStore interface {
	CreateFile(ctx context.Context, file File) error
	GetFile(ctx context.Context, polarisID string) (*File, error)
	GetFileForProject(ctx context.Context, polarisID, projectID string) (*File, error)
	ListFiles(ctx context.Context, filter FileFilter) ([]File, error)
	DeleteFile(ctx context.Context, polarisID string) error
	GetFileInline(ctx context.Context, polarisID string) ([]byte, error)
	PutFileProviderHandle(ctx context.Context, handle FileProviderHandle) error
	GetFileProviderHandle(ctx context.Context, polarisID, provider string) (*FileProviderHandle, bool, error)
	DeleteFileProviderHandlesByPolarisID(ctx context.Context, polarisID string) error
	PurgeExpiredFiles(ctx context.Context, now time.Time) (int64, error)
	SumProjectFileBytes(ctx context.Context, projectID string) (int64, error)
	CountProjectFiles(ctx context.Context, projectID string) (int64, error)
	CountFilesByBlobKey(ctx context.Context, blobKey string) (int64, error)
	PutFileUnderstandingArtifact(ctx context.Context, artifact FileUnderstandingArtifact) error
	GetFileUnderstandingArtifact(ctx context.Context, sha256, processor, version string) (*FileUnderstandingArtifact, bool, error)
}

// AuditStore records audit events.
type AuditStore interface {
	LogAuditEvent(ctx context.Context, event AuditEvent) error
	LogAuditEventBatch(ctx context.Context, events []AuditEvent) error
}

// ToolStore manages tool definitions.
type ToolStore interface {
	CreateToolDefinition(ctx context.Context, tool ToolDefinition) error
	ListToolDefinitions(ctx context.Context) ([]ToolDefinition, error)
	GetToolDefinition(ctx context.Context, id string) (*ToolDefinition, error)
}

// ToolsetStore manages toolsets.
type ToolsetStore interface {
	CreateToolset(ctx context.Context, toolset Toolset) error
	ListToolsets(ctx context.Context) ([]Toolset, error)
	GetToolset(ctx context.Context, id string) (*Toolset, error)
}

// MCPStore manages MCP bindings.
type MCPStore interface {
	CreateMCPBinding(ctx context.Context, binding MCPBinding) error
	ListMCPBindings(ctx context.Context) ([]MCPBinding, error)
	GetMCPBinding(ctx context.Context, id string) (*MCPBinding, error)
}

// VoiceStore manages archived voice metadata.
type VoiceStore interface {
	ArchiveVoice(ctx context.Context, voice ArchivedVoice) error
	UnarchiveVoice(ctx context.Context, provider string, model string, voiceID string) error
	GetArchivedVoice(ctx context.Context, provider string, model string, voiceID string) (*ArchivedVoice, error)
	ListArchivedVoices(ctx context.Context, provider string, model string) ([]ArchivedVoice, error)
}

// RequestLogStore records request logs (billing/usage rows).
type RequestLogStore interface {
	LogRequest(ctx context.Context, log RequestLog) error
	LogRequestBatch(ctx context.Context, logs []RequestLog) error
}

// UsageStore reports aggregated usage.
type UsageStore interface {
	GetUsage(ctx context.Context, filter UsageFilter) (UsageReport, error)
	GetUsageByModel(ctx context.Context, filter UsageFilter) (UsageReport, error)
}

// IdempotencyStore persists idempotency-key outcomes for job-submit endpoints.
type IdempotencyStore interface {
	CheckIdempotencyKey(ctx context.Context, key, projectID, endpoint string) (*IdempotencyKey, bool, error)
	PutIdempotencyKey(ctx context.Context, rec IdempotencyKey) error
	PurgeExpiredIdempotencyKeys(ctx context.Context, now time.Time) (int64, error)
}

// MaintenanceStore covers lifecycle/maintenance operations.
type MaintenanceStore interface {
	PurgeOldLogs(ctx context.Context, olderThan time.Time) (int64, error)
	Migrate(ctx context.Context) error
	Ping(ctx context.Context) error
	Close() error
}

// Store is the full persistence contract, composed of the role interfaces above.
type Store interface {
	KeyStore
	ProjectStore
	VirtualKeyStore
	PolicyStore
	BudgetStore
	FileStore
	AuditStore
	ToolStore
	ToolsetStore
	MCPStore
	VoiceStore
	RequestLogStore
	UsageStore
	IdempotencyStore
	MaintenanceStore
}
