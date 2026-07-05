// Package middleware holds the Gin middleware chain: recovery, request IDs,
// tracing, runtime snapshot injection, body limits, CORS, logging, metrics,
// authentication, rate limiting, budgets, and load shedding.
package middleware

import (
	"net/http"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/reliability"
	"github.com/gin-gonic/gin"
)

// Shed enforces the global in-flight request cap
// (runtime.reliability.shed.global_max_inflight). Disabled by default: when the
// cap is 0 it only tracks the in-flight gauge and never rejects. Over the cap it
// returns 503 overloaded with Retry-After so clients back off, protecting the
// gateway from metastable overload. Registered at the front of the chain so load
// is shed before expensive auth/store work.
func Shed(manager *reliability.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if manager == nil {
			c.Next()
			return
		}
		release, admitted := manager.AdmitGlobal()
		if !admitted {
			c.Header("Retry-After", "1")
			httputil.WriteError(c, httputil.NewError(http.StatusServiceUnavailable, "overloaded", "server_overloaded", "", "Server is overloaded; please retry shortly."))
			c.Abort()
			return
		}
		defer release()
		c.Next()
	}
}
