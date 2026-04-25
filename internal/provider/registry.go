package provider

import (
	"errors"
	"fmt"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/modality"
)

var (
	ErrUnknownAlias      = errors.New("unknown alias")
	ErrUnknownModel      = errors.New("unknown model")
	ErrRouteNotResolved  = errors.New("route not resolved")
	ErrModalityMismatch  = errors.New("modality mismatch")
	ErrCapabilityMissing = errors.New("capability missing")
	ErrAdapterMissing    = errors.New("adapter missing")
)

type Registry struct {
	models                         map[string]Model
	chatAdapters                   map[string]modality.ChatAdapter
	embedAdapters                  map[string]modality.EmbedAdapter
	imageAdapters                  map[string]modality.ImageAdapter
	videoAdapters                  map[string]modality.VideoAdapter
	voiceAdapters                  map[string]modality.VoiceAdapter
	voiceCatalogAdapters           map[string]modality.VoiceCatalogAdapter
	voiceAssetAdapters             map[string]modality.VoiceAssetAdapter
	translationAdapters            map[string]modality.TranslationAdapter
	audioNotesAdapters             map[string]modality.AudioNotesAdapter
	podcastAdapters                map[string]modality.PodcastAdapter
	streamingTranscriptionAdapters map[string]modality.StreamingTranscriptionAdapter
	audioAdapters                  map[string]modality.AudioAdapter
	interpretingAdapters           map[string]modality.InterpretingAdapter
	musicAdapters                  map[string]modality.MusicAdapter
	aliases                        map[string]string
	familyAliases                  map[string]string
	families                       map[string]modelFamily
	selectors                      map[string]config.RoutingSelector
	fallbacks                      map[string][]string
}

type modelFamily struct {
	ID          string
	DisplayName string
	Modality    modality.Modality
	Aliases     []string
	Variants    []string
}

type Resolution struct {
	Model            Model
	RequestedModel   string
	ResolvedModel    string
	ResolvedProvider string
	Mode             string
}

type Model struct {
	ID                string                `json:"id"`
	Object            string                `json:"object"`
	Kind              string                `json:"kind,omitempty"`
	Provider          string                `json:"provider,omitempty"`
	ProviderVariant   string                `json:"provider_variant,omitempty"`
	Name              string                `json:"-"`
	DisplayName       string                `json:"display_name,omitempty"`
	FamilyID          string                `json:"family_id,omitempty"`
	FamilyDisplayName string                `json:"family_display_name,omitempty"`
	Status            string                `json:"status,omitempty"`
	VerificationClass string                `json:"verification_class,omitempty"`
	CostTier          string                `json:"cost_tier,omitempty"`
	LatencyTier       string                `json:"latency_tier,omitempty"`
	DocURL            string                `json:"doc_url,omitempty"`
	LastVerified      string                `json:"last_verified,omitempty"`
	Modality          modality.Modality     `json:"modality"`
	Capabilities      []modality.Capability `json:"capabilities,omitempty"`
	ContextWindow     int                   `json:"context_window,omitempty"`
	MaxOutputTokens   int                   `json:"max_output_tokens,omitempty"`
	MaxDuration       int                   `json:"max_duration,omitempty"`
	AllowedDurations  []int                 `json:"allowed_durations,omitempty"`
	AspectRatios      []string              `json:"aspect_ratios,omitempty"`
	Resolutions       []string              `json:"resolutions,omitempty"`
	Cancelable        bool                  `json:"cancelable,omitempty"`
	Voices            []string              `json:"voices,omitempty"`
	Formats           []string              `json:"formats,omitempty"`
	OutputFormats     []string              `json:"output_formats,omitempty"`
	MinDurationMs     int                   `json:"min_duration_ms,omitempty"`
	MaxDurationMs     int                   `json:"max_duration_ms,omitempty"`
	SampleRatesHz     []int                 `json:"sample_rates_hz,omitempty"`
	Dimensions        int                   `json:"dimensions,omitempty"`
	SessionTTL        int64                 `json:"session_ttl,omitempty"`
	ResolvesTo        string                `json:"resolves_to,omitempty"`
	FamilyPriority    int                   `json:"-"`
}

func New(cfg *config.Config) (*Registry, []string, error) {
	if _, err := prepareProviderCatalog(); err != nil {
		return nil, nil, fmt.Errorf("load provider catalog: %w", err)
	}

	registry := &Registry{
		models:                         map[string]Model{},
		chatAdapters:                   map[string]modality.ChatAdapter{},
		embedAdapters:                  map[string]modality.EmbedAdapter{},
		imageAdapters:                  map[string]modality.ImageAdapter{},
		videoAdapters:                  map[string]modality.VideoAdapter{},
		voiceAdapters:                  map[string]modality.VoiceAdapter{},
		voiceCatalogAdapters:           map[string]modality.VoiceCatalogAdapter{},
		voiceAssetAdapters:             map[string]modality.VoiceAssetAdapter{},
		translationAdapters:            map[string]modality.TranslationAdapter{},
		audioNotesAdapters:             map[string]modality.AudioNotesAdapter{},
		podcastAdapters:                map[string]modality.PodcastAdapter{},
		streamingTranscriptionAdapters: map[string]modality.StreamingTranscriptionAdapter{},
		audioAdapters:                  map[string]modality.AudioAdapter{},
		interpretingAdapters:           map[string]modality.InterpretingAdapter{},
		musicAdapters:                  map[string]modality.MusicAdapter{},
		aliases:                        map[string]string{},
		familyAliases:                  map[string]string{},
		families:                       map[string]modelFamily{},
		selectors:                      map[string]config.RoutingSelector{},
		fallbacks:                      map[string][]string{},
	}

	var warnings []string
	for providerName, providerCfg := range cfg.Providers {
		if !providerEnabled(providerName, providerCfg) {
			warnings = append(warnings, fmt.Sprintf("provider %s is disabled because required credentials are missing", providerName))
			continue
		}
		if registerProviderFamily(registry, &warnings, providerName, providerCfg) {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("provider %s is configured but not supported by this runtime build", providerName))
	}

	registerConfiguredAliases(registry, cfg.Routing, &warnings)
	registerCatalogAliases(registry)
	registerCatalogFamilies(registry)
	registerSelectors(registry, cfg.Routing, &warnings)
	registerFallbacks(registry, cfg.Routing)

	return registry, warnings, nil
}
