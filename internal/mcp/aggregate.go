package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// namespaceSep separates a binding namespace from a tool name in aggregate lists.
const namespaceSep = ":"

type aggregateSource struct {
	prefix string // "" = local (unprefixed); otherwise the binding id
	source ToolSource
}

// Aggregate merges several ToolSources into one. Upstream tools are namespaced
// `{prefix}:{tool}`; the local source (prefix "") stays bare. tools/list results
// are cached per source with a TTL and refreshed lazily; a failing upstream is
// skipped, never sinking the whole aggregate.
type Aggregate struct {
	sources []aggregateSource
	ttl     time.Duration

	mu    sync.Mutex
	cache map[string]cachedTools
	now   func() time.Time
}

type cachedTools struct {
	tools   []Tool
	expires time.Time
}

// NewAggregate builds an aggregate with a per-source tools/list cache TTL (0 =
// no caching).
func NewAggregate(ttl time.Duration) *Aggregate {
	return &Aggregate{ttl: ttl, cache: map[string]cachedTools{}, now: time.Now}
}

// Add registers a source under a namespace prefix ("" for the local source).
func (a *Aggregate) Add(prefix string, source ToolSource) {
	a.sources = append(a.sources, aggregateSource{prefix: prefix, source: source})
}

// ListTools merges every source's tools, namespacing prefixed sources.
func (a *Aggregate) ListTools(ctx context.Context, _ string) ([]Tool, string, error) {
	all := make([]Tool, 0)
	for _, s := range a.sources {
		tools, err := a.listCached(ctx, s)
		if err != nil {
			continue
		}
		for _, t := range tools {
			if s.prefix != "" {
				t.Name = s.prefix + namespaceSep + t.Name
			}
			all = append(all, t)
		}
	}
	return all, "", nil
}

func (a *Aggregate) listCached(ctx context.Context, s aggregateSource) ([]Tool, error) {
	if a.ttl > 0 {
		a.mu.Lock()
		if c, ok := a.cache[s.prefix]; ok && a.now().Before(c.expires) {
			tools := c.tools
			a.mu.Unlock()
			return tools, nil
		}
		a.mu.Unlock()
	}
	tools, _, err := s.source.ListTools(ctx, "")
	if err != nil {
		return nil, err
	}
	if a.ttl > 0 {
		a.mu.Lock()
		a.cache[s.prefix] = cachedTools{tools: tools, expires: a.now().Add(a.ttl)}
		a.mu.Unlock()
	}
	return tools, nil
}

// CallTool routes by the namespace prefix; a bare name goes to the local source.
func (a *Aggregate) CallTool(ctx context.Context, name string, arguments json.RawMessage) (ToolResult, error) {
	prefix, bare := splitNamespace(name)
	for _, s := range a.sources {
		if s.prefix == prefix {
			return s.source.CallTool(ctx, bare, arguments)
		}
	}
	return ToolResult{}, fmt.Errorf("unknown tool: %s", name)
}

func splitNamespace(name string) (prefix, bare string) {
	if i := strings.Index(name, namespaceSep); i > 0 {
		return name[:i], name[i+1:]
	}
	return "", name
}
