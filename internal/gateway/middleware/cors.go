package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/gin-gonic/gin"
)

func CORS(runtime *gwruntime.Holder) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := effectiveCORSConfig(c, runtime)
		if !cfg.Enabled {
			c.Next()
			return
		}

		headers := c.Writer.Header()
		origin := strings.TrimSpace(c.GetHeader("Origin"))
		if origin != "" {
			allowed, value := corsAllowedOrigin(origin, cfg.AllowedOrigins)
			if !allowed {
				if c.Request.Method == http.MethodOptions {
					httputil.WriteError(c, httputil.NewError(http.StatusForbidden, "invalid_request_error", "cors_origin_not_allowed", "", "CORS origin is not allowed."))
					return
				}
			} else {
				headers.Set("Access-Control-Allow-Origin", value)
				headers.Add("Vary", "Origin")
				if cfg.AllowCredentials {
					headers.Set("Access-Control-Allow-Credentials", "true")
				}
			}
		}
		headers.Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
		headers.Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))
		headers.Set("Access-Control-Expose-Headers", strings.Join(cfg.ExposedHeaders, ", "))
		if cfg.MaxAge > 0 {
			headers.Set("Access-Control-Max-Age", strconv.FormatInt(int64(cfg.MaxAge.Seconds()), 10))
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

func effectiveCORSConfig(c *gin.Context, runtime *gwruntime.Holder) config.CORSConfig {
	if snapshot := RuntimeSnapshot(c, runtime); snapshot != nil && snapshot.Config != nil {
		cfg := snapshot.Config.Server.CORS
		if !cfg.Enabled {
			return cfg
		}
		defaults := config.DefaultCORSConfig()
		if len(cfg.AllowedOrigins) == 0 {
			cfg.AllowedOrigins = defaults.AllowedOrigins
		}
		if len(cfg.AllowedHeaders) == 0 {
			cfg.AllowedHeaders = defaults.AllowedHeaders
		}
		if len(cfg.AllowedMethods) == 0 {
			cfg.AllowedMethods = defaults.AllowedMethods
		}
		if len(cfg.ExposedHeaders) == 0 {
			cfg.ExposedHeaders = defaults.ExposedHeaders
		}
		if cfg.MaxAge == 0 {
			cfg.MaxAge = defaults.MaxAge
		}
		return cfg
	}
	return config.DefaultCORSConfig()
}

func corsAllowedOrigin(origin string, allowedOrigins []string) (bool, string) {
	for _, allowedOrigin := range allowedOrigins {
		allowedOrigin = strings.TrimSpace(allowedOrigin)
		switch {
		case allowedOrigin == "":
			continue
		case allowedOrigin == "*":
			return true, "*"
		case allowedOrigin == origin:
			return true, origin
		case strings.HasSuffix(allowedOrigin, ":*"):
			prefix := strings.TrimSuffix(allowedOrigin, "*")
			if strings.HasPrefix(origin, prefix) {
				return true, origin
			}
		}
	}
	return false, ""
}
