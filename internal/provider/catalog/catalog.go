package catalog

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"gopkg.in/yaml.v3"
)

//go:embed models.yaml
var embeddedModelsYAML []byte

type Entry struct {
	Provider          string                `yaml:"provider"`
	ModelID           string                `yaml:"model_id"`
	DisplayName       string                `yaml:"display_name"`
	Aliases           []string              `yaml:"aliases"`
	FamilyID          string                `yaml:"family_id"`
	FamilyDisplayName string                `yaml:"family_display_name"`
	FamilyAliases     []string              `yaml:"family_aliases"`
	FamilyPriority    int                   `yaml:"family_priority"`
	Modality          modality.Modality     `yaml:"modality"`
	Capabilities      []modality.Capability `yaml:"capabilities"`
	Status            string                `yaml:"status"`
	VerificationClass string                `yaml:"verification_class"`
	CostTier          string                `yaml:"cost_tier"`
	LatencyTier       string                `yaml:"latency_tier"`
	DocURL            string                `yaml:"doc_url"`
	LastVerified      string                `yaml:"last_verified"`
}

func (e Entry) ID() string {
	return strings.TrimSpace(e.Provider) + "/" + strings.TrimSpace(e.ModelID)
}

type fileFormat struct {
	Models []Entry `yaml:"models"`
}

type Family struct {
	ID          string
	DisplayName string
	Aliases     []string
	Modality    modality.Modality
}

type Catalog struct {
	entriesByID       map[string]Entry
	aliasToID         map[string]string
	familyAliasToID   map[string]string
	familiesByID      map[string]Family
	entriesByFamilyID map[string][]Entry
	providers         map[string]struct{}
}

func (c *Catalog) Lookup(id string) (Entry, bool) {
	entry, ok := c.entriesByID[id]
	return entry, ok
}

func (c *Catalog) AliasTarget(alias string) (string, bool) {
	target, ok := c.aliasToID[alias]
	return target, ok
}

func (c *Catalog) Family(id string) (Family, bool) {
	family, ok := c.familiesByID[id]
	return family, ok
}

func (c *Catalog) FamilyAliasTarget(alias string) (string, bool) {
	target, ok := c.familyAliasToID[alias]
	return target, ok
}

func (c *Catalog) FamilyEntries(id string) []Entry {
	entries := c.entriesByFamilyID[id]
	out := make([]Entry, len(entries))
	copy(out, entries)
	return out
}

func (c *Catalog) Families() []Family {
	items := make([]Family, 0, len(c.familiesByID))
	for _, family := range c.familiesByID {
		items = append(items, family)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items
}

func (c *Catalog) Entries() []Entry {
	items := make([]Entry, 0, len(c.entriesByID))
	for _, entry := range c.entriesByID {
		items = append(items, entry)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID() < items[j].ID()
	})
	return items
}

func (c *Catalog) Providers() []string {
	items := make([]string, 0, len(c.providers))
	for provider := range c.providers {
		items = append(items, provider)
	}
	sort.Strings(items)
	return items
}

var (
	defaultOnce sync.Once
	defaultCat  *Catalog
	defaultErr  error
)

func Default() (*Catalog, error) {
	defaultOnce.Do(func() {
		defaultCat, defaultErr = parseCatalog(embeddedModelsYAML)
	})
	return defaultCat, defaultErr
}

func parseCatalog(data []byte) (*Catalog, error) {
	var parsed fileFormat
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("decode provider model catalog: %w", err)
	}
	if len(parsed.Models) == 0 {
		return nil, fmt.Errorf("provider model catalog is empty")
	}

	entriesByID := make(map[string]Entry, len(parsed.Models))
	aliasToID := make(map[string]string)
	familyAliasToID := make(map[string]string)
	familiesByID := make(map[string]Family)
	entriesByFamilyID := make(map[string][]Entry)
	providers := make(map[string]struct{})

	for index, entry := range parsed.Models {
		if err := validateEntry(entry); err != nil {
			return nil, fmt.Errorf("provider model catalog entry %d (%s/%s): %w", index, entry.Provider, entry.ModelID, err)
		}

		id := entry.ID()
		if _, exists := entriesByID[id]; exists {
			return nil, fmt.Errorf("duplicate model entry %q", id)
		}
		entriesByID[id] = entry
		providers[entry.Provider] = struct{}{}
		entriesByFamilyID[entry.FamilyID] = append(entriesByFamilyID[entry.FamilyID], entry)

		family, exists := familiesByID[entry.FamilyID]
		if !exists {
			family = Family{
				ID:          entry.FamilyID,
				DisplayName: entry.FamilyDisplayName,
				Modality:    entry.Modality,
			}
		} else {
			if family.DisplayName != entry.FamilyDisplayName {
				return nil, fmt.Errorf("family %q has inconsistent display_name %q vs %q", entry.FamilyID, family.DisplayName, entry.FamilyDisplayName)
			}
			if family.Modality != entry.Modality {
				return nil, fmt.Errorf("family %q has inconsistent modality %q vs %q", entry.FamilyID, family.Modality, entry.Modality)
			}
		}

		seenAliases := make(map[string]struct{}, len(entry.Aliases))
		for _, alias := range entry.Aliases {
			trimmed := strings.TrimSpace(alias)
			if _, exists := seenAliases[trimmed]; exists {
				return nil, fmt.Errorf("duplicate alias %q within model %q", trimmed, id)
			}
			seenAliases[trimmed] = struct{}{}
			if existing, exists := aliasToID[trimmed]; exists && existing != id {
				return nil, fmt.Errorf("alias %q collides between %q and %q", trimmed, existing, id)
			}
			aliasToID[trimmed] = id
		}

		seenFamilyAliases := make(map[string]struct{}, len(entry.FamilyAliases))
		for _, alias := range entry.FamilyAliases {
			trimmed := strings.TrimSpace(alias)
			if _, exists := seenFamilyAliases[trimmed]; exists {
				return nil, fmt.Errorf("duplicate family alias %q within family %q", trimmed, entry.FamilyID)
			}
			seenFamilyAliases[trimmed] = struct{}{}
			if existing, exists := familyAliasToID[trimmed]; exists && existing != entry.FamilyID {
				return nil, fmt.Errorf("family alias %q collides between %q and %q", trimmed, existing, entry.FamilyID)
			}
			familyAliasToID[trimmed] = entry.FamilyID
			if !slicesContainsString(family.Aliases, trimmed) {
				family.Aliases = append(family.Aliases, trimmed)
			}
		}
		sort.Strings(family.Aliases)
		familiesByID[entry.FamilyID] = family
	}

	for familyID, entries := range entriesByFamilyID {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].FamilyPriority != entries[j].FamilyPriority {
				return entries[i].FamilyPriority < entries[j].FamilyPriority
			}
			return entries[i].ID() < entries[j].ID()
		})
		entriesByFamilyID[familyID] = entries
	}

	return &Catalog{
		entriesByID:       entriesByID,
		aliasToID:         aliasToID,
		familyAliasToID:   familyAliasToID,
		familiesByID:      familiesByID,
		entriesByFamilyID: entriesByFamilyID,
		providers:         providers,
	}, nil
}

func validateEntry(entry Entry) error {
	if strings.TrimSpace(entry.Provider) == "" {
		return fmt.Errorf("provider is required")
	}
	if strings.TrimSpace(entry.ModelID) == "" {
		return fmt.Errorf("model_id is required")
	}
	if strings.TrimSpace(entry.DisplayName) == "" {
		return fmt.Errorf("display_name is required")
	}
	if strings.TrimSpace(entry.FamilyID) == "" {
		return fmt.Errorf("family_id is required")
	}
	if strings.TrimSpace(entry.FamilyDisplayName) == "" {
		return fmt.Errorf("family_display_name is required")
	}
	if entry.FamilyPriority < 0 {
		return fmt.Errorf("family_priority must be >= 0")
	}
	if !entry.Modality.Valid() {
		return fmt.Errorf("invalid modality %q", entry.Modality)
	}
	for _, capability := range entry.Capabilities {
		if !capability.Valid() {
			return fmt.Errorf("invalid capability %q", capability)
		}
	}
	switch entry.Status {
	case "ga", "preview", "experimental":
	default:
		return fmt.Errorf("invalid status %q", entry.Status)
	}
	switch entry.VerificationClass {
	case "strict", "opt_in", "skipped":
	default:
		return fmt.Errorf("invalid verification_class %q", entry.VerificationClass)
	}
	switch strings.TrimSpace(entry.CostTier) {
	case "", modality.CostTierLow, modality.CostTierBalanced, modality.CostTierPremium:
	default:
		return fmt.Errorf("invalid cost_tier %q", entry.CostTier)
	}
	switch strings.TrimSpace(entry.LatencyTier) {
	case "", modality.LatencyTierFast, modality.LatencyTierBalanced, modality.LatencyTierBestQuality:
	default:
		return fmt.Errorf("invalid latency_tier %q", entry.LatencyTier)
	}
	if strings.TrimSpace(entry.DocURL) == "" {
		return fmt.Errorf("doc_url is required")
	}
	if _, err := time.Parse("2006-01-02", entry.LastVerified); err != nil {
		return fmt.Errorf("invalid last_verified %q", entry.LastVerified)
	}
	for _, alias := range entry.Aliases {
		if strings.TrimSpace(alias) == "" {
			return fmt.Errorf("aliases must not contain empty values")
		}
	}
	for _, alias := range entry.FamilyAliases {
		if strings.TrimSpace(alias) == "" {
			return fmt.Errorf("family_aliases must not contain empty values")
		}
	}
	return nil
}

func slicesContainsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
