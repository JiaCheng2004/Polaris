package middleware

import (
	"strings"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/metrics"
	"github.com/gin-gonic/gin"
)

func Metrics(recorder *metrics.Recorder) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		if recorder == nil {
			return
		}

		outcome, _ := GetRequestOutcome(c)
		model := outcome.Model
		provider := outcome.Provider
		modality := string(outcome.Modality)
		statusCode := c.Writer.Status()
		if outcome.StatusCode != 0 {
			statusCode = outcome.StatusCode
		}
		cost := estimateOutcomeCost(c, outcome, nil)
		interfaceFamily := firstNonEmpty(outcome.InterfaceFamily, interfaceFamilyFromPath(c.FullPath()))
		tokenSource := usageTokenSource(outcome)

		recorder.ObserveRequest(
			interfaceFamily,
			model,
			modality,
			provider,
			statusCode,
			time.Since(start),
			outcome.ProviderLatencyMs,
			outcome.PromptTokens,
			outcome.CompletionTokens,
			tokenSource,
			cost.TotalUSD,
			cost.Source,
			outcome.ErrorType,
		)
		recorder.IncPricingLookup(model, cost.LookupStatus)
		if cacheStatus := strings.TrimSpace(c.Writer.Header().Get("X-Polaris-Cache")); cacheStatus != "" {
			recorder.IncCacheEvent(cacheStatus, model)
		}
	}
}
