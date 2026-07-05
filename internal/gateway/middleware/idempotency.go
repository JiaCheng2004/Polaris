package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/gin-gonic/gin"
)

// IdempotencyCoordinator serializes concurrent requests bearing the same
// idempotency key (per project+endpoint) so a duplicate waits for the first to
// finish and then replays its stored response. Process-lifetime; the map is
// bounded by the number of concurrent in-flight keys (ref-counted eviction).
type IdempotencyCoordinator struct {
	mu    sync.Mutex
	locks map[string]*idemLock
}

type idemLock struct {
	mu   sync.Mutex
	refs int
}

// NewIdempotencyCoordinator builds an empty coordinator.
func NewIdempotencyCoordinator() *IdempotencyCoordinator {
	return &IdempotencyCoordinator{locks: map[string]*idemLock{}}
}

func (c *IdempotencyCoordinator) acquire(key string) func() {
	c.mu.Lock()
	l, ok := c.locks[key]
	if !ok {
		l = &idemLock{}
		c.locks[key] = l
	}
	l.refs++
	c.mu.Unlock()

	l.mu.Lock()
	return func() {
		l.mu.Unlock()
		c.mu.Lock()
		l.refs--
		if l.refs == 0 {
			delete(c.locks, key)
		}
		c.mu.Unlock()
	}
}

// idemCapture tees the response body so a fresh key's response can be persisted.
// The status is read back from gin's ResponseWriter.Status() after the handler
// runs (gin's WriteHeaderNow bypasses a wrapper's WriteHeader).
type idemCapture struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *idemCapture) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w *idemCapture) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// Idempotency wraps a job-submit route. On an Idempotency-Key header it replays
// the stored response for a repeated key (409 idempotency_key_reuse on a
// request-body mismatch) and persists the response of a fresh key. No header or
// disabled config → passthrough. Bodies are captured before binding and
// restored so the handler is otherwise untouched.
func Idempotency(runtime *gwruntime.Holder, st store.Store, coord *IdempotencyCoordinator, recorder *metrics.Recorder, endpoint string) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := RuntimeSnapshot(c, runtime)
		if snapshot == nil || snapshot.Config == nil || !snapshot.Config.Reliability.Idempotency.Enabled || st == nil || coord == nil {
			c.Next()
			return
		}
		key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
		if key == "" {
			c.Next()
			return
		}

		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "invalid_body", "", "Request body could not be read."))
			c.Abort()
			return
		}
		_ = c.Request.Body.Close()
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))
		sum := sha256.Sum256(raw)
		reqHash := hex.EncodeToString(sum[:])

		projectID := GetAuthContext(c).ProjectID
		release := coord.acquire(projectID + "\x00" + endpoint + "\x00" + key)
		defer release()

		ctx := c.Request.Context()
		if existing, ok, checkErr := st.CheckIdempotencyKey(ctx, key, projectID, endpoint); checkErr == nil && ok {
			if existing.RequestHash != reqHash {
				httputil.WriteError(c, httputil.NewError(http.StatusConflict, "invalid_request_error", "idempotency_key_reuse", "Idempotency-Key", "This Idempotency-Key was already used with a different request body."))
				c.Abort()
				return
			}
			c.Header("Idempotency-Replayed", "true")
			c.Data(existing.ResponseStatus, "application/json", existing.ResponseBody)
			recorder.IncIdempotentReplay(endpoint)
			c.Abort()
			return
		}

		capture := &idemCapture{ResponseWriter: c.Writer}
		c.Writer = capture
		c.Next()

		status := capture.Status()
		if status >= 200 && status < 300 {
			now := time.Now().UTC()
			ttl := snapshot.Config.Reliability.Idempotency.TTL
			if ttl <= 0 {
				ttl = 24 * time.Hour
			}
			_ = st.PutIdempotencyKey(ctx, store.IdempotencyKey{
				Key:            key,
				ProjectID:      projectID,
				Endpoint:       endpoint,
				RequestHash:    reqHash,
				ResponseStatus: status,
				ResponseBody:   append([]byte(nil), capture.body.Bytes()...),
				CreatedAt:      now,
				ExpiresAt:      now.Add(ttl),
			})
		}
	}
}
