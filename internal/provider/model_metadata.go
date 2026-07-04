package provider

import (
	"slices"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/provider/core"
)

type ModelCapabilityFlags struct {
	Chat              bool `json:"chat"`
	Vision            bool `json:"vision"`
	FileUnderstanding bool `json:"file_understanding"`
	ImageGeneration   bool `json:"image_generation"`
	ImageEdit         bool `json:"image_edit"`
	MusicGeneration   bool `json:"music_generation"`
	LyricsGeneration  bool `json:"lyrics_generation"`
	VideoGeneration   bool `json:"video_generation"`
	ToolCalling       bool `json:"tool_calling"`
	Streaming         bool `json:"streaming"`
}

type ModelLifecycle struct {
	Enabled           bool   `json:"enabled"`
	Status            string `json:"status,omitempty"`
	Stability         string `json:"stability,omitempty"`
	VerificationClass string `json:"verification_class,omitempty"`
}

type ModelHostedTool struct {
	Name               string `json:"name"`
	Capability         string `json:"capability"`
	ResolvedServerSide bool   `json:"resolved_server_side"`
}

type ModelBillingMetadata struct {
	BillingMode     string                        `json:"billing_mode,omitempty"`
	Unit            string                        `json:"unit,omitempty"`
	Currency        string                        `json:"currency,omitempty"`
	Source          string                        `json:"source,omitempty"`
	EffectiveFrom   string                        `json:"effective_from,omitempty"`
	EffectiveUntil  string                        `json:"effective_until,omitempty"`
	Notes           string                        `json:"notes,omitempty"`
	Rates           map[string]float64            `json:"rates,omitempty"`
	Tiers           map[string]map[string]float64 `json:"tiers,omitempty"`
	TieredRates     []ModelTieredRate             `json:"tiered_rates,omitempty"`
	AdditionalUnits map[string]float64            `json:"additional_units,omitempty"`
}

type ModelTieredRate struct {
	ID    string             `json:"id,omitempty"`
	Range [2]int             `json:"range"`
	Rates map[string]float64 `json:"rates,omitempty"`
}

type ModelQuotaMetadata struct {
	QuotaBucket      string   `json:"quota_bucket,omitempty"`
	Unit             string   `json:"unit,omitempty"`
	DailyLimit       *float64 `json:"daily_limit,omitempty"`
	ConcurrencyLimit *int     `json:"concurrency_limit,omitempty"`
}

func (r *Registry) refreshModelMetadata() {
	for id, model := range r.models {
		model.Aliases = r.aliasesForModel(id)
		model.CapabilityFlags = capabilityFlags(model)
		model.Lifecycle = lifecycleMetadata(model)
		model.HostedTools = hostedTools(model.Capabilities)
		r.models[id] = model
	}
}

func (r *Registry) aliasesForModel(modelID string) []string {
	aliases := make([]string, 0)
	for alias, target := range r.aliases {
		if target == modelID {
			aliases = append(aliases, alias)
		}
	}
	slices.Sort(aliases)
	return aliases
}

func withModelMetadata(model Model) Model {
	model.CapabilityFlags = capabilityFlags(model)
	model.Lifecycle = lifecycleMetadata(model)
	model.HostedTools = hostedTools(model.Capabilities)
	return model
}

func capabilityFlags(model Model) ModelCapabilityFlags {
	return ModelCapabilityFlags{
		Chat:              model.Modality == modality.ModalityChat,
		Vision:            core.ContainsCapability(model.Capabilities, modality.CapabilityVision),
		FileUnderstanding: nativeFileUnderstanding(model.Capabilities),
		ImageGeneration: model.Modality == modality.ModalityImage &&
			core.ContainsCapability(model.Capabilities, modality.CapabilityGeneration),
		ImageEdit: model.Modality == modality.ModalityImage &&
			core.ContainsCapability(model.Capabilities, modality.CapabilityEditing),
		MusicGeneration: model.Modality == modality.ModalityMusic &&
			core.ContainsCapability(model.Capabilities, modality.CapabilityMusicGeneration),
		LyricsGeneration: model.Modality == modality.ModalityMusic &&
			core.ContainsCapability(model.Capabilities, modality.CapabilityLyricsGeneration),
		VideoGeneration: model.Modality == modality.ModalityVideo &&
			(core.ContainsCapability(model.Capabilities, modality.CapabilityTextToVideo) ||
				core.ContainsCapability(model.Capabilities, modality.CapabilityImageToVideo) ||
				core.ContainsCapability(model.Capabilities, modality.CapabilityGeneration)),
		ToolCalling: core.ContainsCapability(model.Capabilities, modality.CapabilityFunctionCalling),
		Streaming:   core.ContainsCapability(model.Capabilities, modality.CapabilityStreaming),
	}
}

func nativeFileUnderstanding(capabilities []modality.Capability) bool {
	for _, capability := range capabilities {
		switch capability {
		case modality.CapabilityPDF,
			modality.CapabilityPDFInput,
			modality.CapabilityDocumentInput,
			modality.CapabilityFileReference:
			return true
		}
	}
	return false
}

func lifecycleMetadata(model Model) ModelLifecycle {
	return ModelLifecycle{
		Enabled:           true,
		Status:            model.Status,
		Stability:         stabilityForStatus(model.Status),
		VerificationClass: model.VerificationClass,
	}
}

func stabilityForStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "ga":
		return "stable"
	case "experimental", "preview":
		return "experimental"
	case "deprecated":
		return "deprecated"
	default:
		return "unknown"
	}
}

func hostedTools(capabilities []modality.Capability) []ModelHostedTool {
	tools := make([]ModelHostedTool, 0, 7)
	for _, tool := range []struct {
		capability modality.Capability
		name       string
	}{
		{capability: modality.CapabilityHostedToolWebSearch, name: "web_search"},
		{capability: modality.CapabilityHostedToolCodeInterpreter, name: "code_interpreter"},
		{capability: modality.CapabilityHostedToolComputerUse, name: "computer_use"},
		{capability: modality.CapabilityHostedToolFileSearch, name: "file_search"},
		{capability: modality.CapabilityHostedToolImageGeneration, name: "image_generation"},
		{capability: modality.CapabilityHostedToolMCP, name: "mcp"},
		{capability: modality.CapabilityHostedToolURLContext, name: "url_context"},
	} {
		if core.ContainsCapability(capabilities, tool.capability) {
			tools = append(tools, ModelHostedTool{
				Name:               tool.name,
				Capability:         string(tool.capability),
				ResolvedServerSide: true,
			})
		}
	}
	return tools
}
