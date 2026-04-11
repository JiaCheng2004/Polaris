package middleware

import (
	"log/slog"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/store"
	"github.com/gin-gonic/gin"
)

func Usage(requestLogger *store.AsyncRequestLogger, logger *slog.Logger) gin.HandlerFunc {
	if logger == nil {
		logger = slog.Default()
	}

	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		if requestLogger == nil {
			return
		}

		outcome, ok := GetRequestOutcome(c)
		if !ok || outcome.Modality == "" || outcome.Model == "" {
			return
		}

		auth := GetAuthContext(c)
		if outcome.StatusCode == 0 {
			outcome.StatusCode = c.Writer.Status()
		}
		if outcome.TotalLatencyMs == 0 {
			outcome.TotalLatencyMs = int(time.Since(start).Milliseconds())
		}
		if outcome.TotalTokens == 0 {
			outcome.TotalTokens = outcome.PromptTokens + outcome.CompletionTokens
		}

		entry := store.RequestLog{
			RequestID:         GetRequestID(c),
			KeyID:             auth.KeyID,
			Model:             outcome.Model,
			Modality:          outcome.Modality,
			ProviderLatencyMs: outcome.ProviderLatencyMs,
			TotalLatencyMs:    outcome.TotalLatencyMs,
			InputTokens:       outcome.PromptTokens,
			OutputTokens:      outcome.CompletionTokens,
			TotalTokens:       outcome.TotalTokens,
			EstimatedCost:     estimatedCostUSD(outcome.Model, outcome.PromptTokens, outcome.CompletionTokens),
			StatusCode:        outcome.StatusCode,
			ErrorType:         outcome.ErrorType,
			CreatedAt:         time.Now().UTC(),
		}

		if !requestLogger.Log(entry) {
			logger.Warn("usage entry dropped", "request_id", entry.RequestID, "model", entry.Model)
		}
	}
}

type pricing struct {
	inputPerMillion  float64
	outputPerMillion float64
}

var phaseOnePricing = map[string]pricing{
	"openai/gpt-4o":               {inputPerMillion: 5.00, outputPerMillion: 15.00},
	"openai/gpt-4o-mini":          {inputPerMillion: 0.15, outputPerMillion: 0.60},
	"anthropic/claude-sonnet-4-6": {inputPerMillion: 3.00, outputPerMillion: 15.00},
	"anthropic/claude-opus-4-6":   {inputPerMillion: 15.00, outputPerMillion: 75.00},
}

func estimatedCostUSD(model string, promptTokens, completionTokens int) float64 {
	price, ok := phaseOnePricing[model]
	if !ok {
		return 0
	}
	return (float64(promptTokens)*price.inputPerMillion + float64(completionTokens)*price.outputPerMillion) / 1_000_000
}
