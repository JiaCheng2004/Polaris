package pricing

import (
	"fmt"
	"math"
	"strings"
)

const perMTok = 1_000_000.0

func (c *Catalog) Estimate(req EstimateRequest) EstimateResult {
	entry, ok, status := c.lookup(req.Model)
	if !ok {
		return missingResult(status)
	}

	result := EstimateResult{
		Currency:     defaultCurrency(entry.Currency),
		Source:       SourceTable,
		BreakdownUSD: map[string]float64{},
		LookupStatus: status,
	}
	rates, band := resolveRates(entry, req)
	result.EffectiveBand = band
	result.EffectiveTier = strings.TrimSpace(req.Tier)
	if !hasBillableActivity(req) && rates.PerCall == 0 {
		result.Source = SourceFallbackZero
		return result
	}

	addTokenCost(result.BreakdownUSD, "input", billableInputTokens(req), rates.InputPerMTok)
	cacheReadRate := rates.CacheReadPerMTok
	if rates.InputCacheHitPerMTok != 0 {
		cacheReadRate = rates.InputCacheHitPerMTok
	}
	addTokenCost(result.BreakdownUSD, "cache_read", req.CachedInputTokens, cacheReadRate)
	addTokenCost(result.BreakdownUSD, "cache_write_5m", req.CacheWrite5mTokens, rates.CacheWrite5mPerMTok)
	addTokenCost(result.BreakdownUSD, "cache_write_1h", req.CacheWrite1hTokens, rates.CacheWrite1hPerMTok)
	addTokenCost(result.BreakdownUSD, "output", req.OutputTokens, rates.OutputPerMTok)

	reasoningRate := rates.OutputReasoningPerMTok
	if reasoningRate == 0 {
		reasoningRate = rates.OutputPerMTok
	}
	addTokenCost(result.BreakdownUSD, "reasoning", req.ReasoningTokens, reasoningRate)
	addTokenCost(result.BreakdownUSD, "input_image_tokens", req.InputImageTokens, rates.InputImageTokenPerMTok)
	addTokenCost(result.BreakdownUSD, "output_image_tokens", req.OutputImageTokens, rates.OutputImageTokenPerMTok)

	addUnitCost(result.BreakdownUSD, "audio_seconds", req.AudioSeconds, rates.InputPerAudioSecond)
	addUnitCost(result.BreakdownUSD, "video_seconds", req.VideoSeconds, rates.InputPerVideoSecond)
	addUnitCost(result.BreakdownUSD, "characters", float64(req.Characters), rates.InputPerCharacter)
	addImageCost(result.BreakdownUSD, req.Images, rates)
	if rates.PerCall > 0 {
		result.BreakdownUSD["call"] = rates.PerCall
	}
	for name, count := range req.UnitCounts {
		additionalCost(result.BreakdownUSD, name, count, entry.AdditionalUnits)
	}

	for _, cost := range result.BreakdownUSD {
		result.TotalUSD += cost
	}
	result.TotalUSD = roundUSD(result.TotalUSD)
	for key, cost := range result.BreakdownUSD {
		result.BreakdownUSD[key] = roundUSD(cost)
	}
	return result
}

func resolveRates(entry Entry, req EstimateRequest) (Rates, string) {
	rates := Rates{}
	band := ""
	if len(entry.TieredPricing) > 0 {
		for _, candidate := range entry.TieredPricing {
			if inRange(req.InputTokens, candidate.Range) {
				rates = candidate.Rates
				band = candidate.ID
				if band == "" {
					band = fmt.Sprintf("%d-%d", candidate.Range[0], candidate.Range[1])
				}
				break
			}
		}
		if band == "" {
			last := entry.TieredPricing[len(entry.TieredPricing)-1]
			rates = last.Rates
			band = last.ID
			if band == "" {
				band = fmt.Sprintf("%d-%d", last.Range[0], last.Range[1])
			}
		}
	} else if entry.Pricing != nil {
		rates = *entry.Pricing
	}

	if tier := strings.TrimSpace(req.Tier); tier != "" {
		if override, ok := entry.Tiers[tier]; ok {
			rates = applyRateOverride(rates, override)
		}
	}
	if deployment := strings.TrimSpace(req.Deployment); deployment != "" {
		if override, ok := entry.Deployments[deployment]; ok {
			rates = applyRateOverride(rates, override)
		}
	}
	return rates, band
}

func applyRateOverride(base Rates, override Rates) Rates {
	if override.Multiplier > 0 {
		base = multiplyRates(base, override.Multiplier)
	}
	if override.InputPerMTok != 0 {
		base.InputPerMTok = override.InputPerMTok
	}
	if override.OutputPerMTok != 0 {
		base.OutputPerMTok = override.OutputPerMTok
	}
	if override.OutputReasoningPerMTok != 0 {
		base.OutputReasoningPerMTok = override.OutputReasoningPerMTok
	}
	if override.CacheReadPerMTok != 0 {
		base.CacheReadPerMTok = override.CacheReadPerMTok
	}
	if override.CacheWrite5mPerMTok != 0 {
		base.CacheWrite5mPerMTok = override.CacheWrite5mPerMTok
	}
	if override.CacheWrite1hPerMTok != 0 {
		base.CacheWrite1hPerMTok = override.CacheWrite1hPerMTok
	}
	if override.InputCacheHitPerMTok != 0 {
		base.InputCacheHitPerMTok = override.InputCacheHitPerMTok
	}
	if override.InputImageTokenPerMTok != 0 {
		base.InputImageTokenPerMTok = override.InputImageTokenPerMTok
	}
	if override.OutputImageTokenPerMTok != 0 {
		base.OutputImageTokenPerMTok = override.OutputImageTokenPerMTok
	}
	if override.InputPerAudioSecond != 0 {
		base.InputPerAudioSecond = override.InputPerAudioSecond
	}
	if override.OutputPerAudioSecond != 0 {
		base.OutputPerAudioSecond = override.OutputPerAudioSecond
	}
	if override.InputPerVideoSecond != 0 {
		base.InputPerVideoSecond = override.InputPerVideoSecond
	}
	if override.OutputPerVideoSecond != 0 {
		base.OutputPerVideoSecond = override.OutputPerVideoSecond
	}
	if override.InputPerCharacter != 0 {
		base.InputPerCharacter = override.InputPerCharacter
	}
	if override.InputPerImage != 0 {
		base.InputPerImage = override.InputPerImage
	}
	if override.OutputPerImage != 0 {
		base.OutputPerImage = override.OutputPerImage
	}
	if override.OutputPerPixel != 0 {
		base.OutputPerPixel = override.OutputPerPixel
	}
	if override.PerCall != 0 {
		base.PerCall = override.PerCall
	}
	return base
}

func multiplyRates(rates Rates, multiplier float64) Rates {
	rates.InputPerMTok *= multiplier
	rates.OutputPerMTok *= multiplier
	rates.OutputReasoningPerMTok *= multiplier
	rates.CacheReadPerMTok *= multiplier
	rates.CacheWrite5mPerMTok *= multiplier
	rates.CacheWrite1hPerMTok *= multiplier
	rates.InputCacheHitPerMTok *= multiplier
	rates.InputImageTokenPerMTok *= multiplier
	rates.OutputImageTokenPerMTok *= multiplier
	rates.InputPerAudioSecond *= multiplier
	rates.OutputPerAudioSecond *= multiplier
	rates.InputPerVideoSecond *= multiplier
	rates.OutputPerVideoSecond *= multiplier
	rates.InputPerCharacter *= multiplier
	rates.InputPerImage *= multiplier
	rates.OutputPerImage *= multiplier
	rates.OutputPerPixel *= multiplier
	rates.PerCall *= multiplier
	return rates
}

func billableInputTokens(req EstimateRequest) int {
	return max(0, req.InputTokens-req.CachedInputTokens-req.CacheWrite5mTokens-req.CacheWrite1hTokens)
}

func addTokenCost(breakdown map[string]float64, key string, tokens int, ratePerMTok float64) {
	if tokens <= 0 || ratePerMTok == 0 {
		return
	}
	breakdown[key] += float64(tokens) * ratePerMTok / perMTok
}

func addUnitCost(breakdown map[string]float64, key string, units float64, rate float64) {
	if units <= 0 || rate == 0 {
		return
	}
	breakdown[key] += units * rate
}

func addImageCost(breakdown map[string]float64, images int, rates Rates) {
	if images <= 0 {
		return
	}
	if rates.OutputPerImage != 0 {
		breakdown["output_images"] += float64(images) * rates.OutputPerImage
		return
	}
	if rates.InputPerImage != 0 {
		breakdown["input_images"] += float64(images) * rates.InputPerImage
	}
}

func additionalCost(breakdown map[string]float64, name string, count int, rates map[string]float64) {
	if count <= 0 || len(rates) == 0 {
		return
	}
	canonical := strings.TrimSpace(name)
	if canonical == "" {
		return
	}
	if rate, ok := rates[canonical]; ok {
		breakdown[canonical] += float64(count) * rate
		return
	}
	if rate, ok := rates[canonical+"_per_call"]; ok {
		breakdown[canonical] += float64(count) * rate
		return
	}
	if rate, ok := rates[canonical+"_per_1k_calls"]; ok {
		breakdown[canonical] += float64(count) * rate / 1000
		return
	}
	if rate, ok := rates[canonical+"_per_container_hour"]; ok {
		breakdown[canonical] += float64(count) * rate
	}
}

func inRange(tokens int, bounds [2]int) bool {
	lo := bounds[0]
	hi := bounds[1]
	if tokens < lo {
		return false
	}
	if hi <= 0 {
		return true
	}
	return tokens < hi
}

func defaultCurrency(currency string) string {
	if strings.TrimSpace(currency) == "" {
		return "USD"
	}
	return strings.TrimSpace(currency)
}

func missingResult(status string) EstimateResult {
	if status == "" {
		status = LookupMiss
	}
	return EstimateResult{
		Currency:     "USD",
		Source:       SourceMissing,
		BreakdownUSD: map[string]float64{},
		LookupStatus: status,
	}
}

func hasBillableActivity(req EstimateRequest) bool {
	return req.InputTokens > 0 ||
		req.CachedInputTokens > 0 ||
		req.CacheWrite5mTokens > 0 ||
		req.CacheWrite1hTokens > 0 ||
		req.OutputTokens > 0 ||
		req.ReasoningTokens > 0 ||
		req.InputImageTokens > 0 ||
		req.OutputImageTokens > 0 ||
		req.AudioSeconds > 0 ||
		req.VideoSeconds > 0 ||
		req.Characters > 0 ||
		req.Images > 0 ||
		len(req.UnitCounts) > 0
}

func roundUSD(value float64) float64 {
	return math.Round(value*1_000_000_000) / 1_000_000_000
}
