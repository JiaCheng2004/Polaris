package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/JiaCheng2004/Polaris/internal/reliability"
	"github.com/gin-gonic/gin"
)

type hedgeResult struct {
	resp    *modality.ChatResponse
	outcome middleware.RequestOutcome
	target  chatTarget
	index   int
}

// hedgeConfigFor returns the hedge config for a model+modality, or the zero
// (disabled) config when no policy matches.
func (h *ChatHandler) hedgeConfigFor(c *gin.Context, modelID string, mod modality.Modality) config.HedgeConfig {
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil || snapshot.Config == nil {
		return config.HedgeConfig{}
	}
	if p := matchRoutePolicy(snapshot.Config.Routing.Policies, modelID, mod); p != nil {
		return p.Hedge
	}
	return config.HedgeConfig{}
}

// completeHedged races idempotent unary completions across the top candidates to
// cut tail latency. Only the winner's usage is recorded (winner-only billing);
// losers are cancelled before their usage would be counted. Used only when a
// route policy enables hedging — never for streaming or job submits.
func (h *ChatHandler) completeHedged(c *gin.Context, targets []chatTarget, req *modality.ChatRequest, interfaceFamily string, hedge config.HedgeConfig) (*modality.ChatResponse, middleware.RequestOutcome, string, error) {
	n := 1 + hedge.MaxExtra
	if n > len(targets) {
		n = len(targets)
	}
	if n < 1 {
		n = 1
	}

	attempts := make([]func(context.Context) (*hedgeResult, error), n)
	for i := 0; i < n; i++ {
		idx := i
		target := targets[i]
		attempts[i] = func(ctx context.Context) (*hedgeResult, error) {
			attemptReq := *req
			attemptReq.Model = target.model.ID
			resolvedReq, err := h.resolveFilesForTarget(c, &attemptReq, target.model)
			if err != nil {
				return nil, err
			}
			release, admit := h.admit(target.model.Provider)
			if admit != reliability.AdmitOK {
				release()
				return nil, noAvailableProviderError()
			}
			resp, err := target.adapter.Complete(ctx, resolvedReq)
			release()
			if err != nil {
				return nil, err
			}
			outcome := middleware.RequestOutcome{
				Model:           target.model.ID,
				Provider:        target.model.Provider,
				Modality:        modality.ModalityChat,
				InterfaceFamily: interfaceFamily,
				StatusCode:      http.StatusOK,
				Hedged:          idx > 0,
			}
			onCompleteSuccess(resp, &outcome, resolvedReq, target)
			return &hedgeResult{resp: resp, outcome: outcome, target: target, index: idx}, nil
		}
	}

	res, _, err := reliability.Hedge(c.Request.Context(), time.Duration(hedge.DelayMs)*time.Millisecond, attempts)
	if err != nil {
		apiErr := apiErrorFrom(err)
		return nil, middleware.RequestOutcome{
			Model:           targets[0].model.ID,
			Provider:        targets[0].model.Provider,
			Modality:        modality.ModalityChat,
			InterfaceFamily: interfaceFamily,
			StatusCode:      apiErr.Status,
			ErrorType:       apiErr.Type,
		}, "", apiErr
	}

	fallbackModel := ""
	if res.index > 0 {
		fallbackModel = res.target.model.ID
		res.outcome.FallbackModel = fallbackModel
	}
	return res.resp, res.outcome, fallbackModel, nil
}
