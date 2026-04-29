package pricing

import (
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

type Catalog struct {
	entries map[string]Entry
	globs   []globEntry
	loaded  time.Time
	sources []string
}

type globEntry struct {
	pattern string
	prefix  string
	entry   Entry
}

type Holder struct {
	ptr atomic.Pointer[Catalog]
}

func NewHolder(catalog *Catalog) *Holder {
	holder := &Holder{}
	if catalog == nil {
		catalog = emptyCatalog()
	}
	holder.Swap(catalog)
	return holder
}

func (h *Holder) Current() *Catalog {
	if h == nil {
		return nil
	}
	return h.ptr.Load()
}

func (h *Holder) Swap(catalog *Catalog) {
	if h == nil {
		return
	}
	if catalog == nil {
		catalog = emptyCatalog()
	}
	h.ptr.Store(catalog)
}

func (h *Holder) Lookup(modelID string) (Entry, bool) {
	catalog := h.Current()
	if catalog == nil {
		return Entry{}, false
	}
	return catalog.Lookup(modelID)
}

func (h *Holder) Estimate(req EstimateRequest) EstimateResult {
	catalog := h.Current()
	if catalog == nil {
		return missingResult(LookupMiss)
	}
	return catalog.Estimate(req)
}

func (c *Catalog) Lookup(modelID string) (Entry, bool) {
	entry, ok, _ := c.lookup(modelID)
	return entry, ok
}

func (c *Catalog) lookup(modelID string) (Entry, bool, string) {
	if c == nil {
		return Entry{}, false, LookupMiss
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return Entry{}, false, LookupMiss
	}
	if entry, ok := c.entries[modelID]; ok {
		return entry, true, LookupHit
	}
	for _, candidate := range c.globs {
		if strings.HasPrefix(modelID, candidate.prefix) {
			return candidate.entry, true, LookupGlobHit
		}
	}
	return Entry{}, false, LookupMiss
}

func (c *Catalog) Sources() []string {
	if c == nil {
		return nil
	}
	out := make([]string, len(c.sources))
	copy(out, c.sources)
	return out
}

func (c *Catalog) LoadedAt() time.Time {
	if c == nil {
		return time.Time{}
	}
	return c.loaded
}

func (c *Catalog) Entries() map[string]Entry {
	if c == nil {
		return nil
	}
	out := make(map[string]Entry, len(c.entries)+len(c.globs))
	for key, entry := range c.entries {
		out[key] = entry
	}
	for _, glob := range c.globs {
		out[glob.pattern] = glob.entry
	}
	return out
}

func newCatalog(entries map[string]Entry, loaded time.Time, sources []string) *Catalog {
	if loaded.IsZero() {
		loaded = time.Now().UTC()
	}
	catalog := &Catalog{
		entries: make(map[string]Entry),
		loaded:  loaded,
		sources: append([]string(nil), sources...),
	}

	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		entry := entries[key]
		if strings.HasSuffix(key, "*") {
			catalog.globs = append(catalog.globs, globEntry{
				pattern: key,
				prefix:  strings.TrimSuffix(key, "*"),
				entry:   entry,
			})
			continue
		}
		catalog.entries[key] = entry
	}
	sort.Slice(catalog.globs, func(i, j int) bool {
		if len(catalog.globs[i].prefix) != len(catalog.globs[j].prefix) {
			return len(catalog.globs[i].prefix) > len(catalog.globs[j].prefix)
		}
		return catalog.globs[i].pattern < catalog.globs[j].pattern
	})
	return catalog
}

func emptyCatalog() *Catalog {
	return &Catalog{
		entries: map[string]Entry{},
		loaded:  time.Now().UTC(),
	}
}
