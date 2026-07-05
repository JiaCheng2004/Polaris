// Package transport is Polaris's single outbound HTTP core. Every provider
// adapter issues upstream requests through a transport.Client, which owns the
// one retry loop (full-jitter backoff, Retry-After, per-provider attempt count),
// authentication decoration (Bearer, API-key headers, SigV4, OAuth, HMAC),
// error translation into apierror, and provider client spans. It replaces the
// ~8 copy-pasted retry loops that previously lived in each provider package.
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/apierror"
	"github.com/JiaCheng2004/Polaris/internal/obs"
)

// AuthFunc decorates an outbound request with credentials. It receives the
// exact request body bytes so signing strategies (AWS SigV4, Volcengine HMAC)
// can hash the payload. It is invoked once per attempt on a freshly built
// request. A nil AuthFunc leaves the request unauthenticated (e.g. local Ollama).
type AuthFunc func(req *http.Request, body []byte) error

// ErrorTranslator converts an upstream error response (status + body) into a
// canonical Polaris APIError. Each provider supplies one so its native error
// envelope is parsed while the resulting taxonomy stays uniform.
type ErrorTranslator func(providerName string, status int, body []byte) *apierror.APIError

// Options configures a Client.
type Options struct {
	BaseURL         string
	ProviderName    string // display name used in error messages, e.g. "OpenAI"
	ProviderSlug    string // low-cardinality slug for provider spans, e.g. "openai"
	Auth            AuthFunc
	StaticHeaders   map[string]string
	Timeout         time.Duration
	Retry           RetryPolicy
	ErrorTranslator ErrorTranslator
	Hooks           []AttemptHook
	// HTTPClient overrides the constructed client (tests). When set, Timeout and
	// the provider-span RoundTripper are not applied.
	HTTPClient *http.Client
}

// Client is a configured outbound HTTP client for one provider.
type Client struct {
	baseURL       string
	providerName  string
	slug          string
	auth          AuthFunc
	staticHeaders map[string]string
	retry         RetryPolicy
	translateErr  ErrorTranslator
	hooks         []AttemptHook
	httpClient    *http.Client
}

// New builds a Client.
func New(opts Options) *Client {
	slug := opts.ProviderSlug
	if slug == "" {
		slug = strings.ToLower(strings.TrimSpace(opts.ProviderName))
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		timeout := opts.Timeout
		if timeout <= 0 {
			timeout = time.Minute
		}
		httpClient = &http.Client{
			Timeout:   timeout,
			Transport: obs.NewProviderTransport(slug, nil),
		}
	}
	headers := make(map[string]string, len(opts.StaticHeaders))
	for k, v := range opts.StaticHeaders {
		headers[k] = v
	}
	return &Client{
		baseURL:       strings.TrimRight(opts.BaseURL, "/"),
		providerName:  opts.ProviderName,
		slug:          slug,
		auth:          opts.Auth,
		staticHeaders: headers,
		retry:         opts.Retry,
		translateErr:  opts.ErrorTranslator,
		hooks:         opts.Hooks,
		httpClient:    httpClient,
	}
}

// BaseURL returns the client's configured base URL (no trailing slash).
func (c *Client) BaseURL() string { return c.baseURL }

// ProviderName returns the display name used in error messages.
func (c *Client) ProviderName() string { return c.providerName }

// HTTPClient exposes the underlying *http.Client for the rare adapter that must
// issue a bespoke request (e.g. binary asset download) outside the retry loop.
func (c *Client) HTTPClient() *http.Client { return c.httpClient }

// Request is one logical outbound request. Body carries the canonical payload
// bytes so the retry loop can rebuild a fresh *http.Request each attempt; use
// BodyReader only for non-retryable streaming bodies (multipart uploads), which
// force a single attempt.
type Request struct {
	Method      string
	Path        string // relative to BaseURL, or an absolute URL
	Query       url.Values
	Body        []byte
	BodyReader  io.Reader
	ContentType string
	Accept      string
	Headers     map[string]string
}

// ReqOption mutates a Request built by the JSON/Stream/Raw helpers.
type ReqOption func(*Request)

// WithAccept sets the Accept header.
func WithAccept(accept string) ReqOption { return func(r *Request) { r.Accept = accept } }

// WithHeader sets one per-request header.
func WithHeader(key, value string) ReqOption {
	return func(r *Request) {
		if r.Headers == nil {
			r.Headers = map[string]string{}
		}
		r.Headers[key] = value
	}
}

// WithHeaders sets several per-request headers.
func WithHeaders(headers map[string]string) ReqOption {
	return func(r *Request) {
		if len(headers) == 0 {
			return
		}
		if r.Headers == nil {
			r.Headers = map[string]string{}
		}
		for k, v := range headers {
			r.Headers[k] = v
		}
	}
}

// WithQuery sets the query string.
func WithQuery(q url.Values) ReqOption { return func(r *Request) { r.Query = q } }

// Do runs the retry loop and returns the raw response. The caller owns and must
// close resp.Body. Do never returns a drained/closed response: a status- or
// transport-level retry that is interrupted mid-backoff returns an error.
func (c *Client) Do(ctx context.Context, r Request) (*http.Response, error) {
	attempts := c.retry.attempts()
	// A streaming (non-byte) body cannot be safely replayed, so never retry it.
	if r.BodyReader != nil {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		req, err := c.build(ctx, r)
		if err != nil {
			return nil, err
		}

		start := time.Now()
		resp, err := c.httpClient.Do(req)
		latency := time.Since(start)
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		c.report(r, attempt, status, err, latency)

		if err != nil {
			lastErr = err
			if attempt < attempts && retryableTransportError(err) {
				if backoffSleep(ctx, c.retry.backoff(attempt)) == nil {
					continue
				}
			}
			return nil, apierror.ProviderTransportError(err, c.providerName)
		}

		if attempt < attempts && retryableStatus(resp.StatusCode) {
			delay := c.retry.backoff(attempt)
			if c.retry.RespectRetryAfter {
				if ra, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
					delay = min(ra, 2*c.retry.maxDelay())
				}
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if backoffSleep(ctx, delay) == nil {
				continue
			}
			// B2 fix: the sleep was interrupted after we drained+closed the body,
			// so this response is dead. Return the context error, never the resp.
			return nil, apierror.ProviderTransportError(ctx.Err(), c.providerName)
		}

		return resp, nil
	}

	return nil, apierror.ProviderTransportError(lastErr, c.providerName)
}

func (c *Client) build(ctx context.Context, r Request) (*http.Request, error) {
	method := r.Method
	if method == "" {
		method = http.MethodPost
	}
	target := r.Path
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		target = c.baseURL + r.Path
	}
	if len(r.Query) > 0 {
		if strings.Contains(target, "?") {
			target += "&" + r.Query.Encode()
		} else {
			target += "?" + r.Query.Encode()
		}
	}

	var body io.Reader
	if r.BodyReader != nil {
		body = r.BodyReader
	} else if r.Body != nil {
		body = bytes.NewReader(r.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", strings.ToLower(c.providerName), err)
	}
	if r.ContentType != "" {
		req.Header.Set("Content-Type", r.ContentType)
	}
	if r.Accept != "" {
		req.Header.Set("Accept", r.Accept)
	}
	for k, v := range c.staticHeaders {
		req.Header.Set(k, v)
	}
	for k, v := range r.Headers {
		req.Header.Set(k, v)
	}
	if c.auth != nil {
		if err := c.auth(req, r.Body); err != nil {
			return nil, err
		}
	}
	return req, nil
}

func (c *Client) report(r Request, attempt, status int, err error, latency time.Duration) {
	obsHooks := loadObservers()
	if len(c.hooks) == 0 && len(obsHooks) == 0 {
		return
	}
	info := AttemptInfo{
		Provider: c.providerName,
		Slug:     c.slug,
		Attempt:  attempt,
		Method:   r.Method,
		Path:     r.Path,
		Status:   status,
		Err:      err,
		Latency:  latency,
	}
	for _, hook := range c.hooks {
		hook(info)
	}
	for _, hook := range obsHooks {
		hook(info)
	}
}

// JSON marshals in (when non-nil) as the request body, executes the request,
// and decodes a success response into out (when non-nil). Non-2xx responses are
// converted through the provider's ErrorTranslator.
func (c *Client) JSON(ctx context.Context, method, path string, in, out any, mods ...ReqOption) error {
	req, err := c.jsonRequest(method, path, in, mods)
	if err != nil {
		return err
	}
	req.Accept = firstNonEmpty(req.Accept, "application/json")

	resp, err := c.Do(ctx, req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return c.apiError(resp)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return apierror.NewError(http.StatusBadGateway, "provider_error", "provider_invalid_response", "", c.providerName+" returned an invalid JSON response.")
	}
	return nil
}

// Stream marshals in (when non-nil) and returns the raw response for the caller
// to read (SSE or binary). The caller owns and must close resp.Body. Error
// statuses are translated and the body closed before returning.
func (c *Client) Stream(ctx context.Context, method, path string, in any, mods ...ReqOption) (*http.Response, error) {
	req, err := c.jsonRequest(method, path, in, mods)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() { _ = resp.Body.Close() }()
		return nil, c.apiError(resp)
	}
	return resp, nil
}

// Raw executes a request with a caller-provided body reader (e.g. multipart)
// and returns the raw response. The caller owns and must close resp.Body.
func (c *Client) Raw(ctx context.Context, method, path string, body io.Reader, contentType string, mods ...ReqOption) (*http.Response, error) {
	req := Request{Method: method, Path: path, BodyReader: body, ContentType: contentType}
	for _, mod := range mods {
		mod(&req)
	}
	resp, err := c.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		defer func() { _ = resp.Body.Close() }()
		return nil, c.apiError(resp)
	}
	return resp, nil
}

func (c *Client) jsonRequest(method, path string, in any, mods []ReqOption) (Request, error) {
	req := Request{Method: method, Path: path}
	if in != nil {
		payload, err := json.Marshal(in)
		if err != nil {
			return Request{}, fmt.Errorf("marshal %s request: %w", strings.ToLower(c.providerName), err)
		}
		req.Body = payload
		req.ContentType = "application/json"
	}
	for _, mod := range mods {
		mod(&req)
	}
	return req, nil
}

// apiError reads and translates an error response through the provider's
// ErrorTranslator (or a generic default).
func (c *Client) apiError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if c.translateErr != nil {
		if translated := c.translateErr(c.providerName, resp.StatusCode, body); translated != nil {
			return translated
		}
	}
	return apierror.ProviderAPIError(c.providerName, resp.StatusCode, apierror.ProviderErrorDetails{
		Body: string(body),
	})
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
