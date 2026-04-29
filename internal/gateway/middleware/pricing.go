package middleware

import (
	"log/slog"
	"strings"
	"sync"

	pricingpkg "github.com/JiaCheng2004/Polaris/internal/pricing"
	"github.com/gin-gonic/gin"
)

var missingPricingLogged sync.Map

func EstimateCostUSD(model string, promptTokens int, completionTokens int) float64 {
	result := pricingpkg.DefaultEstimator().Estimate(pricingpkg.EstimateRequest{
		Model:        model,
		InputTokens:  promptTokens,
		OutputTokens: completionTokens,
	})
	return result.TotalUSD
}

func estimateOutcomeCost(c *gin.Context, outcome RequestOutcome, logger *slog.Logger) pricingpkg.EstimateResult {
	estimator := pricingpkg.DefaultEstimator()
	if snapshot, ok := GetRuntimeSnapshot(c); ok && snapshot != nil && snapshot.Pricing != nil {
		estimator = snapshot.Pricing
	}
	result := estimator.Estimate(pricingRequestFromOutcome(outcome))
	if result.Source == pricingpkg.SourceMissing {
		logMissingPricingOnce(logger, outcome.Model)
	}
	return result
}

func pricingRequestFromOutcome(outcome RequestOutcome) pricingpkg.EstimateRequest {
	return pricingpkg.EstimateRequest{
		Model:              outcome.Model,
		Tier:               outcome.Tier,
		Deployment:         outcome.Deployment,
		InputTokens:        outcome.PromptTokens,
		CachedInputTokens:  outcome.CachedInputTokens,
		CacheWrite5mTokens: outcome.CacheWrite5mTokens,
		CacheWrite1hTokens: outcome.CacheWrite1hTokens,
		OutputTokens:       outcome.CompletionTokens,
		ReasoningTokens:    outcome.ReasoningTokens,
		InputImageTokens:   outcome.InputImageTokens,
		OutputImageTokens:  outcome.OutputImageTokens,
		AudioSeconds:       outcome.AudioSeconds,
		VideoSeconds:       outcome.VideoSeconds,
		Characters:         outcome.Characters,
		Images:             outcome.Images,
		UnitCounts:         outcome.UnitCounts,
	}
}

func logMissingPricingOnce(logger *slog.Logger, model string) {
	model = strings.TrimSpace(model)
	if model == "" {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	if _, loaded := missingPricingLogged.LoadOrStore(model, struct{}{}); loaded {
		return
	}
	logger.Warn("missing pricing entry; estimated cost defaults to zero", "model", model)
}
