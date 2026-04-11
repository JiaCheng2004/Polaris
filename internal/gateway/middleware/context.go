package middleware

import (
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
)

const (
	authContextKey = "polaris.auth"
	requestIDKey   = "polaris.request_id"
	outcomeKey     = "polaris.request_outcome"
)

type AuthContext struct {
	KeyID         string
	OwnerID       string
	KeyPrefix     string
	RateLimit     string
	AllowedModels []string
	IsAdmin       bool
	Mode          string
}

type RequestOutcome struct {
	Model             string
	Provider          string
	Modality          modality.Modality
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
