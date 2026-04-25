package middleware

import (
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
)

const (
	authContextKey = "polaris.auth"
	requestIDKey   = "polaris.request_id"
	traceIDKey     = "polaris.trace_id"
	outcomeKey     = "polaris.request_outcome"
	runtimeKey     = "polaris.runtime"
)

type AuthContext struct {
	ProjectID          string
	VirtualKeyID       string
	KeyID              string
	OwnerID            string
	KeyPrefix          string
	RateLimit          string
	AllowedModels      []string
	AllowedModalities  []modality.Modality
	AllowedToolsets    []string
	AllowedMCPBindings []string
	PolicyModels       []string
	PolicyModalities   []modality.Modality
	PolicyToolsets     []string
	PolicyMCPBindings  []string
	IsAdmin            bool
	Mode               string
	TokenSource        string
}

type RequestOutcome struct {
	Model             string
	Provider          string
	Modality          modality.Modality
	InterfaceFamily   string
	TokenSource       modality.TokenCountSource
	CacheStatus       string
	FallbackModel     string
	Toolset           string
	MCPBinding        string
	StatusCode        int
	ErrorType         string
	ProviderLatencyMs int
	TotalLatencyMs    int
	PromptTokens      int
	CompletionTokens  int
	TotalTokens       int
}

func SetAuthContext(c *gin.Context, auth AuthContext) {
	c.Set(authContextKey, auth)
}

func GetAuthContext(c *gin.Context) AuthContext {
	value, ok := c.Get(authContextKey)
	if !ok {
		return AuthContext{}
	}
	auth, ok := value.(AuthContext)
	if !ok {
		return AuthContext{}
	}
	return auth
}

func SetRequestID(c *gin.Context, requestID string) {
	c.Set(requestIDKey, requestID)
}

func GetRequestID(c *gin.Context) string {
	value, ok := c.Get(requestIDKey)
	if !ok {
		return ""
	}
	requestID, _ := value.(string)
	return requestID
}

func SetTraceID(c *gin.Context, traceID string) {
	c.Set(traceIDKey, traceID)
}

func GetTraceID(c *gin.Context) string {
	value, ok := c.Get(traceIDKey)
	if !ok {
		return ""
	}
	traceID, _ := value.(string)
	return traceID
}

func SetRequestOutcome(c *gin.Context, outcome RequestOutcome) {
	c.Set(outcomeKey, outcome)
}

func GetRequestOutcome(c *gin.Context) (RequestOutcome, bool) {
	value, ok := c.Get(outcomeKey)
	if !ok {
		return RequestOutcome{}, false
	}
	outcome, ok := value.(RequestOutcome)
	if !ok {
		return RequestOutcome{}, false
	}
	return outcome, true
}

func SetRuntimeSnapshot(c *gin.Context, snapshot *gwruntime.Snapshot) {
	c.Set(runtimeKey, snapshot)
}

func GetRuntimeSnapshot(c *gin.Context) (*gwruntime.Snapshot, bool) {
	value, ok := c.Get(runtimeKey)
	if !ok {
		return nil, false
	}
	snapshot, ok := value.(*gwruntime.Snapshot)
	if !ok {
		return nil, false
	}
	return snapshot, true
}

func RuntimeSnapshot(c *gin.Context, holder *gwruntime.Holder) *gwruntime.Snapshot {
	if snapshot, ok := GetRuntimeSnapshot(c); ok {
		return snapshot
	}
	if holder == nil {
		return nil
	}
	return holder.Current()
}
