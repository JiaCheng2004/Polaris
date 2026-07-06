package middleware

import (
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/mcp"
	"github.com/gin-gonic/gin"
)

// MCPOAuth validates OAuth 2.1 bearer JWTs for the MCP surface (RFC 9728). A
// bearer that looks like a JWT is validated against the resource server and its
// scopes mapped to binding/toolset allow-lists; otherwise, when apiKeyFallback is
// non-nil, it delegates to the API-key auth path. A failure returns 401 with a
// WWW-Authenticate challenge pointing at the protected-resource metadata.
func MCPOAuth(rs *mcp.ResourceServer, metadataURL string, apiKeyFallback gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := bearerToken(c.GetHeader("Authorization"))
		if ok && looksLikeJWT(token) {
			claims, err := rs.ValidateToken(c.Request.Context(), token)
			if err != nil {
				c.Header("WWW-Authenticate", rs.Challenge(metadataURL))
				httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "invalid_token", "", "OAuth token validation failed."))
				c.Abort()
				return
			}
			SetAuthContext(c, authContextFromClaims(claims))
			c.Next()
			return
		}
		if apiKeyFallback != nil {
			apiKeyFallback(c)
			return
		}
		c.Header("WWW-Authenticate", rs.Challenge(metadataURL))
		httputil.WriteError(c, httputil.NewError(http.StatusUnauthorized, "authentication_error", "missing_token", "", "MCP requires an OAuth bearer token."))
		c.Abort()
	}
}

func looksLikeJWT(token string) bool {
	return strings.Count(token, ".") == 2 && len(token) > 20
}

// authContextFromClaims maps OAuth scopes to Polaris binding/toolset allow-lists.
// Convention: `mcp` (or `mcp:*`) grants all; `mcp:binding:{id}` and
// `mcp:toolset:{id}` grant a specific binding/toolset.
func authContextFromClaims(claims mcp.Claims) AuthContext {
	auth := AuthContext{Mode: "oauth", TokenSource: "oauth", OwnerID: claims.Subject}
	for _, scope := range claims.Scopes {
		switch {
		case scope == "mcp" || scope == "mcp:*":
			auth.AllowedMCPBindings = []string{"*"}
			auth.AllowedToolsets = []string{"*"}
		case strings.HasPrefix(scope, "mcp:binding:"):
			auth.AllowedMCPBindings = append(auth.AllowedMCPBindings, strings.TrimPrefix(scope, "mcp:binding:"))
		case strings.HasPrefix(scope, "mcp:toolset:"):
			auth.AllowedToolsets = append(auth.AllowedToolsets, strings.TrimPrefix(scope, "mcp:toolset:"))
		}
	}
	return auth
}
