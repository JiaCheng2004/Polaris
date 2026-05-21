package handler

import (
	"net/http"
	"strings"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	gwruntime "github.com/JiaCheng2004/Polaris/internal/gateway/runtime"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/pricing"
	"github.com/JiaCheng2004/Polaris/internal/provider"
	"github.com/gin-gonic/gin"
)

type ModelsHandler struct {
	runtime *gwruntime.Holder
}

func NewModelsHandler(runtime *gwruntime.Holder) *ModelsHandler {
	return &ModelsHandler{runtime: runtime}
}

func (h *ModelsHandler) List(c *gin.Context) {
	h.list(c, "list")
}

func (h *ModelsHandler) Capabilities(c *gin.Context) {
	h.list(c, "model_capabilities.list")
}

func (h *ModelsHandler) list(c *gin.Context, object string) {
	includeAliases := c.Query("include_aliases") == "true"
	snapshot := h.snapshot(c)
	if snapshot == nil || snapshot.Registry == nil {
		c.JSON(http.StatusOK, gin.H{"object": object, "data": []provider.Model{}})
		return
	}

	models := snapshot.Registry.ListModels(includeAliases)
	enrichModelList(snapshot, models)
	routing := snapshot.Registry.RoutingMetadata()
	enrichRoutingMetadata(snapshot, &routing)
	c.JSON(http.StatusOK, gin.H{
		"object":  object,
		"data":    models,
		"routing": routing,
	})
}

func (h *ModelsHandler) snapshot(c *gin.Context) *gwruntime.Snapshot {
	return middleware.RuntimeSnapshot(c, h.runtime)
}

func enrichModelList(snapshot *gwruntime.Snapshot, models []provider.Model) {
	for i := range models {
		if derivedFileUnderstandingEnabled(snapshot.Config) && models[i].Modality == modality.ModalityChat {
			models[i].CapabilityFlags.FileUnderstanding = true
		}
		if snapshot.Pricing == nil {
			continue
		}
		modelID := models[i].ID
		if strings.TrimSpace(models[i].ResolvesTo) != "" {
			modelID = models[i].ResolvesTo
		}
		entry, ok := snapshot.Pricing.Lookup(modelID)
		if !ok {
			continue
		}
		models[i].Billing = billingMetadata(entry)
		models[i].Quota = quotaMetadata(entry)
	}
}

func derivedFileUnderstandingEnabled(cfg *config.Config) bool {
	return cfg != nil && cfg.Files.Enabled && cfg.Files.Understanding.Enabled
}

func enrichRoutingMetadata(snapshot *gwruntime.Snapshot, routing *provider.ModelRoutingMetadata) {
	if routing == nil || !derivedFileUnderstandingEnabled(snapshot.Config) {
		return
	}
	chatDefault, ok := routing.Defaults["chat"]
	if !ok {
		return
	}
	if routing.Defaults == nil {
		routing.Defaults = map[string]provider.ModelExecutorDefault{}
	}
	routing.Defaults["file_understanding"] = chatDefault
}

func billingMetadata(entry pricing.Entry) *provider.ModelBillingMetadata {
	unit := pricingUnit(entry)
	metadata := &provider.ModelBillingMetadata{
		BillingMode:     firstNonEmptyModelMetadata(strings.TrimSpace(entry.BillingMode), "usage_based"),
		Unit:            unit,
		Currency:        firstNonEmptyModelMetadata(strings.TrimSpace(entry.Currency), "USD"),
		Source:          strings.TrimSpace(entry.Source),
		EffectiveFrom:   strings.TrimSpace(entry.EffectiveFrom),
		EffectiveUntil:  strings.TrimSpace(entry.EffectiveUntil),
		Notes:           strings.TrimSpace(entry.Notes),
		AdditionalUnits: copyFloatMap(entry.AdditionalUnits),
	}
	if entry.Pricing != nil {
		metadata.Rates = ratesMap(*entry.Pricing, unit, true)
	}
	if len(entry.Tiers) > 0 {
		metadata.Tiers = make(map[string]map[string]float64, len(entry.Tiers))
		for name, rates := range entry.Tiers {
			metadata.Tiers[name] = ratesMap(rates, unit, false)
		}
	}
	if len(entry.TieredPricing) > 0 {
		metadata.TieredRates = make([]provider.ModelTieredRate, 0, len(entry.TieredPricing))
		for _, tier := range entry.TieredPricing {
			metadata.TieredRates = append(metadata.TieredRates, provider.ModelTieredRate{
				ID:    tier.ID,
				Range: tier.Range,
				Rates: ratesMap(tier.Rates, unit, false),
			})
		}
	}
	return metadata
}

func quotaMetadata(entry pricing.Entry) *provider.ModelQuotaMetadata {
	if strings.TrimSpace(entry.QuotaBucket) == "" && entry.DailyLimit == nil && entry.ConcurrencyLimit == nil {
		return nil
	}
	return &provider.ModelQuotaMetadata{
		QuotaBucket:      strings.TrimSpace(entry.QuotaBucket),
		Unit:             strings.TrimSpace(entry.Unit),
		DailyLimit:       entry.DailyLimit,
		ConcurrencyLimit: entry.ConcurrencyLimit,
	}
}

func pricingUnit(entry pricing.Entry) string {
	if strings.TrimSpace(entry.Unit) != "" {
		return strings.TrimSpace(entry.Unit)
	}
	if entry.Pricing != nil {
		if unit := inferRatesUnit(*entry.Pricing); unit != "" {
			return unit
		}
	}
	for _, tier := range entry.TieredPricing {
		if unit := inferRatesUnit(tier.Rates); unit != "" {
			return unit
		}
	}
	for _, tier := range entry.Tiers {
		if unit := inferRatesUnit(tier); unit != "" {
			return unit
		}
	}
	return "unknown"
}

func inferRatesUnit(rates pricing.Rates) string {
	switch {
	case rates.InputPerMTok != 0,
		rates.OutputPerMTok != 0,
		rates.OutputReasoningPerMTok != 0,
		rates.CacheReadPerMTok != 0,
		rates.CacheWrite5mPerMTok != 0,
		rates.CacheWrite1hPerMTok != 0,
		rates.InputCacheHitPerMTok != 0,
		rates.InputImageTokenPerMTok != 0,
		rates.OutputImageTokenPerMTok != 0:
		return "token"
	case rates.PerCall != 0:
		return "call"
	case rates.InputPerAudioSecond != 0,
		rates.OutputPerAudioSecond != 0,
		rates.InputPerVideoSecond != 0,
		rates.OutputPerVideoSecond != 0,
		rates.InputPerAudioFileSecond != 0:
		return "second"
	case rates.InputPerCharacter != 0:
		return "character"
	case rates.InputPerImage != 0, rates.OutputPerImage != 0:
		return "image"
	case rates.OutputPerPixel != 0:
		return "pixel"
	default:
		return ""
	}
}

func ratesMap(rates pricing.Rates, unit string, includeZeroUnitRates bool) map[string]float64 {
	out := make(map[string]float64)
	addRate(out, "multiplier", rates.Multiplier)
	addRate(out, "input_per_mtok", rates.InputPerMTok)
	addRate(out, "output_per_mtok", rates.OutputPerMTok)
	addRate(out, "output_reasoning_per_mtok", rates.OutputReasoningPerMTok)
	addRate(out, "cache_read_per_mtok", rates.CacheReadPerMTok)
	addRate(out, "cache_write_5m_per_mtok", rates.CacheWrite5mPerMTok)
	addRate(out, "cache_write_1h_per_mtok", rates.CacheWrite1hPerMTok)
	addRate(out, "input_cache_hit_per_mtok", rates.InputCacheHitPerMTok)
	addRate(out, "input_image_token_per_mtok", rates.InputImageTokenPerMTok)
	addRate(out, "output_image_token_per_mtok", rates.OutputImageTokenPerMTok)
	addRate(out, "input_per_audio_second", rates.InputPerAudioSecond)
	addRate(out, "output_per_audio_second", rates.OutputPerAudioSecond)
	addRate(out, "input_per_video_second", rates.InputPerVideoSecond)
	addRate(out, "output_per_video_second", rates.OutputPerVideoSecond)
	addRate(out, "input_per_pdf_page", rates.InputPerPDFPage)
	addRate(out, "input_per_image_tile", rates.InputPerImageTile)
	addRate(out, "input_per_audio_file_second", rates.InputPerAudioFileSecond)
	addRate(out, "per_file_storage_gb_hour", rates.PerFileStorageGBHour)
	addRate(out, "per_file_bytes_ingested_gb", rates.PerFileBytesIngestedGB)
	addRate(out, "input_per_character", rates.InputPerCharacter)
	addRate(out, "input_per_image", rates.InputPerImage)
	addRate(out, "output_per_image", rates.OutputPerImage)
	addRate(out, "output_per_pixel", rates.OutputPerPixel)
	addRate(out, "per_call", rates.PerCall)
	if includeZeroUnitRates && unit == "token" {
		out["input_per_mtok"] = rates.InputPerMTok
		out["output_per_mtok"] = rates.OutputPerMTok
	}
	if includeZeroUnitRates && unit == "call" {
		out["per_call"] = rates.PerCall
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addRate(out map[string]float64, name string, value float64) {
	if value != 0 {
		out[name] = value
	}
}

func copyFloatMap(input map[string]float64) map[string]float64 {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]float64, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func firstNonEmptyModelMetadata(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
