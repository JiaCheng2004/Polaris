package middleware

import (
	"log/slog"
	"strings"
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
			ProjectID:         auth.ProjectID,
			Model:             outcome.Model,
			Modality:          outcome.Modality,
			InterfaceFamily:   firstNonEmpty(outcome.InterfaceFamily, interfaceFamilyFromPath(c.FullPath())),
			TokenSource:       usageTokenSource(outcome),
			CacheStatus:       firstNonEmpty(outcome.CacheStatus, c.Writer.Header().Get("X-Polaris-Cache")),
			FallbackModel:     firstNonEmpty(outcome.FallbackModel, c.Writer.Header().Get("X-Polaris-Fallback")),
			TraceID:           GetTraceID(c),
			Toolset:           outcome.Toolset,
			MCPBinding:        outcome.MCPBinding,
			ProviderLatencyMs: outcome.ProviderLatencyMs,
			TotalLatencyMs:    outcome.TotalLatencyMs,
			InputTokens:       outcome.PromptTokens,
			OutputTokens:      outcome.CompletionTokens,
			TotalTokens:       outcome.TotalTokens,
			EstimatedCost:     EstimateCostUSD(outcome.Model, outcome.PromptTokens, outcome.CompletionTokens),
			StatusCode:        outcome.StatusCode,
			ErrorType:         outcome.ErrorType,
			CreatedAt:         time.Now().UTC(),
		}

		if !requestLogger.Log(entry) {
			logger.Warn("usage entry dropped", "request_id", entry.RequestID, "model", entry.Model)
		}
	}
}

func usageTokenSource(outcome RequestOutcome) string {
	switch outcome.TokenSource {
	case "provider_reported", "estimated", "unavailable":
		return string(outcome.TokenSource)
	default:
		return "unavailable"
	}
}

func interfaceFamilyFromPath(path string) string {
	switch {
	case path == "/v1/chat/completions":
		return "chat_completions"
	case path == "/v1/responses":
		return "responses"
	case path == "/v1/messages":
		return "messages"
	case path == "/v1/embeddings":
		return "embeddings"
	case path == "/v1/translations":
		return "translations"
	case path == "/v1/voices" || strings.HasPrefix(path, "/v1/voices/"):
		return "voices"
	case strings.HasPrefix(path, "/v1/images/"):
		return "images"
	case strings.HasPrefix(path, "/v1/video/"):
		return "video"
	case path == "/v1/audio/speech" || path == "/v1/audio/transcriptions":
		return "voice"
	case strings.HasPrefix(path, "/v1/audio/transcriptions/stream"):
		return "voice_streaming"
	case strings.HasPrefix(path, "/v1/audio/notes"):
		return "audio_notes"
	case strings.HasPrefix(path, "/v1/audio/podcasts"):
		return "audio_podcasts"
	case strings.HasPrefix(path, "/v1/audio/sessions"):
		return "audio_sessions"
	case strings.HasPrefix(path, "/v1/audio/interpreting/sessions"):
		return "audio_interpreting"
	case strings.HasPrefix(path, "/v1/music/"):
		return "music"
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
