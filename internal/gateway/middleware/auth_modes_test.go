package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/gin-gonic/gin"
)

func authEngine(t *testing.T, cfg config.Config) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	holder := gwruntime.NewHolder(&cfg, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) { SetRuntimeSnapshot(c, holder.Current()); c.Next() })
	r.Use(Auth(holder, nil, NewAPIKeyCache(time.Minute), NewVirtualKeyCache(time.Minute), discardLog()))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func authGet(r *gin.Engine, bearer string) int {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestAuthStaticMode(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.Mode = config.AuthModeStatic
	cfg.Auth.StaticKeys = []config.StaticKeyConfig{
		{Name: "k", KeyHash: HashAPIKey("secret"), AllowedModels: []string{"*"}},
	}
	r := authEngine(t, cfg)

	if code := authGet(r, "secret"); code != http.StatusOK {
		t.Fatalf("valid key = %d, want 200", code)
	}
	if code := authGet(r, "wrong"); code != http.StatusUnauthorized {
		t.Fatalf("invalid key = %d, want 401", code)
	}
	if code := authGet(r, ""); code != http.StatusUnauthorized {
		t.Fatalf("missing key = %d, want 401", code)
	}
}

func TestAuthNoneMode(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.Mode = config.AuthModeNone
	r := authEngine(t, cfg)
	if code := authGet(r, ""); code != http.StatusOK {
		t.Fatalf("auth=none unauthenticated request = %d, want 200", code)
	}
}
