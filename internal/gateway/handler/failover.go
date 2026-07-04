package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/obs"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
)

// runFailover is the single attempt-and-failover loop shared by the four
// conversation execution paths (unary/streaming × from-primary/fallback-only).
// It resolves files for each target, spans the attempt, invokes the adapter,
// and on a retryable provider error advances to the next target. It is generic
// on the result type (a *ChatResponse or a chunk channel); onSuccess enriches
// the outcome (and result) for the unary path. This is intentionally shaped as
// a drop-in for the Phase-3 routing.Executor.
//
//   - targets: ordered candidates to try.
//   - primaryInTargets: whether targets[0] is the primary (so index 0 is not a
//     fallback) vs. a fallback-only list (every target is a fallback).
//   - attemptOffset: the fallback-attempt number of targets[0] (1 when the
//     primary leads the list, 2 when the primary was already attempted).
//   - noProviderErr: returned when targets is empty or all attempts fail.
func runFailover[T any](
	h *ChatHandler,
	c *gin.Context,
	targets []chatTarget,
	primary chatTarget,
	req *modality.ChatRequest,
	interfaceFamily string,
	attemptOffset int,
	primaryInTargets bool,
	noProviderErr error,
	invoke func(ctx context.Context, resolvedReq *modality.ChatRequest, target chatTarget) (T, error),
	onSuccess func(value T, outcome *middleware.RequestOutcome, resolvedReq *modality.ChatRequest, target chatTarget),
) (T, chatTarget, middleware.RequestOutcome, string, error) {
	var zero T
	if len(targets) == 0 {
		return zero, chatTarget{}, middleware.RequestOutcome{}, "", noProviderErr
	}

	var lastOutcome middleware.RequestOutcome
	for index, target := range targets {
		attemptReq := *req
		attemptReq.Model = target.model.ID
		resolvedReq, err := h.resolveFilesForTarget(c, &attemptReq, target.model)
		if err != nil {
			return zero, chatTarget{}, lastOutcome, "", err
		}

		start := time.Now()
		attemptCtx, attemptSpan := obs.StartInternalSpan(c.Request.Context(), "fallback.attempt",
			attribute.Int("polaris.fallback_attempt", index+attemptOffset),
			attribute.String("polaris.provider", target.model.Provider),
			attribute.String("polaris.model", target.model.ID),
			attribute.String("polaris.fallback_from", primary.model.ID),
		)
		value, err := invoke(attemptCtx, resolvedReq, target)
		if err != nil {
			obs.RecordSpanError(attemptSpan, err)
		}
		attemptSpan.End()
		providerLatencyMs := int(time.Since(start).Milliseconds())

		if err != nil {
			apiErr := apiErrorFrom(err)
			h.metrics.IncProviderError(target.model.Provider, apiErr.Type)
			lastOutcome = middleware.RequestOutcome{
				Model:             target.model.ID,
				Provider:          target.model.Provider,
				Modality:          modality.ModalityChat,
				InterfaceFamily:   interfaceFamily,
				StatusCode:        apiErr.Status,
				ErrorType:         apiErr.Type,
				ProviderLatencyMs: providerLatencyMs,
			}
			if index < len(targets)-1 && shouldRetryWithFallback(apiErr) {
				continue
			}
			return zero, chatTarget{}, lastOutcome, "", apiErr
		}

		isFallback := !primaryInTargets || index > 0
		fallbackModel := ""
		if isFallback {
			fallbackModel = target.model.ID
		}
		outcome := middleware.RequestOutcome{
			Model:             target.model.ID,
			Provider:          target.model.Provider,
			Modality:          modality.ModalityChat,
			InterfaceFamily:   interfaceFamily,
			StatusCode:        http.StatusOK,
			ProviderLatencyMs: providerLatencyMs,
			FallbackModel:     fallbackModel,
		}
		if onSuccess != nil {
			onSuccess(value, &outcome, resolvedReq, target)
		}
		if isFallback {
			obs.AnnotateCurrentSpan(c.Request.Context(),
				attribute.String("polaris.fallback_from", primary.model.ID),
				attribute.String("polaris.fallback_to", fallbackModel),
			)
		}
		return value, target, outcome, fallbackModel, nil
	}

	return zero, chatTarget{}, lastOutcome, "", noProviderErr
}

func invokeComplete(ctx context.Context, req *modality.ChatRequest, target chatTarget) (*modality.ChatResponse, error) {
	return target.adapter.Complete(ctx, req)
}

func invokeStream(ctx context.Context, req *modality.ChatRequest, target chatTarget) (<-chan modality.ChatChunk, error) {
	return target.adapter.Stream(ctx, req)
}

// onCompleteSuccess normalizes the unary response and populates usage on the
// outcome. Streaming has no equivalent (usage arrives in the final chunk).
func onCompleteSuccess(response *modality.ChatResponse, outcome *middleware.RequestOutcome, resolvedReq *modality.ChatRequest, target chatTarget) {
	response.Model = target.model.ID
	response.Usage = normalizeUsage(response.Usage)
	attachFileUnderstandingMetadata(response, resolvedReq)
	outcome.PromptTokens = response.Usage.PromptTokens
	outcome.CompletionTokens = response.Usage.CompletionTokens
	outcome.TotalTokens = response.Usage.TotalTokens
	outcome.TokenSource = providerUsageSource(response.Usage)
}

func noAvailableProviderError() error {
	return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_unavailable", "model", "No available provider could serve this request.")
}

func noConfiguredFallbackError() error {
	return httputil.NewError(http.StatusBadGateway, "provider_error", "provider_unavailable", "model", "No configured fallback provider could serve this request.")
}
