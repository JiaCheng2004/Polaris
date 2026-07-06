package handler

import (
	"net/http"
	"path"

	"github.com/JiaCheng2004/Polaris/internal/config"
	"github.com/JiaCheng2004/Polaris/internal/gateway/httputil"
	"github.com/JiaCheng2004/Polaris/internal/gateway/middleware"
	"github.com/JiaCheng2004/Polaris/internal/guardrails"
	"github.com/JiaCheng2004/Polaris/internal/modality"
	"github.com/gin-gonic/gin"
)

// internalRequestKey marks a request as Polaris-internal (llm_judge, semcache
// embedder) so guardrails and the semantic cache are skipped, preventing
// recursion.
const internalRequestKey = "polaris.internal_request"

// MarkInternalRequest flags the gin context as an internal request.
func MarkInternalRequest(c *gin.Context) { c.Set(internalRequestKey, true) }

func isInternalRequest(c *gin.Context) bool {
	v, ok := c.Get(internalRequestKey)
	return ok && v == true
}

// guardrailPolicies compiles the enabled config policies that apply to modelID.
func guardrailPolicies(cfg *config.Config, modelID string) []guardrails.Policy {
	if cfg == nil || !cfg.Guardrails.Enabled {
		return nil
	}
	var out []guardrails.Policy
	for _, pc := range cfg.Guardrails.Policies {
		if pc.Match.Model != "" {
			if ok, err := path.Match(pc.Match.Model, modelID); err != nil || !ok {
				continue
			}
		}
		specs := make([]guardrails.DetectorSpec, 0, len(pc.Detectors))
		for name, dc := range pc.Detectors {
			specs = append(specs, guardrails.DetectorSpec{Name: name, Types: dc.Types, Terms: dc.Terms, Threshold: dc.Threshold})
		}
		out = append(out, guardrails.Policy{
			Name:      pc.Name,
			Phase:     guardrails.Phase(orDefaultStr(pc.Phase, string(guardrails.PhaseBoth))),
			Action:    guardrails.Action(orDefaultStr(pc.Action, string(guardrails.ActionObserve))),
			FailMode:  pc.FailMode,
			Detectors: specs,
		})
	}
	return out
}

func orDefaultStr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// checkRequestGuardrails evaluates every editable text field of the request. A
// block writes a 4xx and returns true; a redact masks the field in place.
func (h *ChatHandler) checkRequestGuardrails(c *gin.Context, req *modality.ChatRequest) bool {
	if h.guardrails == nil || isInternalRequest(c) {
		return false
	}
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return false
	}
	policies := guardrailPolicies(snapshot.Config, req.Model)
	if len(policies) == 0 {
		return false
	}
	return h.guardTextFields(c, guardrails.PhaseRequest, policies, requestTextFields(req), req.Model)
}

// applyResponseGuardrails evaluates the assistant text of a unary response.
func (h *ChatHandler) applyResponseGuardrails(c *gin.Context, resp *modality.ChatResponse, modelID string) bool {
	if h.guardrails == nil || isInternalRequest(c) || resp == nil {
		return false
	}
	snapshot := middleware.RuntimeSnapshot(c, h.runtime)
	if snapshot == nil {
		return false
	}
	policies := guardrailPolicies(snapshot.Config, modelID)
	if len(policies) == 0 {
		return false
	}
	return h.guardTextFields(c, guardrails.PhaseResponse, policies, responseTextFields(resp), modelID)
}

func (h *ChatHandler) guardTextFields(c *gin.Context, phase guardrails.Phase, policies []guardrails.Policy, fields []*string, modelID string) bool {
	for _, f := range fields {
		if f == nil || *f == "" {
			continue
		}
		verdict := h.guardrails.Evaluate(phase, *f, policies)
		if verdict.Blocked {
			writeGuardrailBlocked(c, modelID, phase)
			return true
		}
		if verdict.Action == guardrails.ActionRedact {
			*f = verdict.Redacted
		}
	}
	return false
}

func writeGuardrailBlocked(c *gin.Context, modelID string, phase guardrails.Phase) {
	middleware.SetRequestOutcome(c, middleware.RequestOutcome{
		Model:      modelID,
		Modality:   modality.ModalityChat,
		StatusCode: http.StatusBadRequest,
		ErrorType:  "guardrail_blocked",
	})
	httputil.WriteError(c, httputil.NewError(http.StatusBadRequest, "invalid_request_error", "guardrail_blocked", "", "Content was blocked by a guardrail policy ("+string(phase)+" phase)."))
}

// requestTextFields returns pointers to every editable text field in a chat
// request (message string content and text content-parts), so redaction can be
// applied in place.
func requestTextFields(req *modality.ChatRequest) []*string {
	var fields []*string
	for i := range req.Messages {
		content := &req.Messages[i].Content
		if content.Text != nil {
			fields = append(fields, content.Text)
		}
		for j := range content.Parts {
			if content.Parts[j].Type == "text" {
				fields = append(fields, &content.Parts[j].Text)
			}
		}
	}
	return fields
}

func responseTextFields(resp *modality.ChatResponse) []*string {
	var fields []*string
	for i := range resp.Choices {
		content := &resp.Choices[i].Message.Content
		if content.Text != nil {
			fields = append(fields, content.Text)
		}
		for j := range content.Parts {
			if content.Parts[j].Type == "text" {
				fields = append(fields, &content.Parts[j].Text)
			}
		}
	}
	return fields
}
