package guardrails

import (
	"context"
	"time"
)

// Detector inspects text and returns findings. Local detectors must be pure,
// fast (<1ms on 4KB), and safe for concurrent use; remote detectors honor the
// context deadline and may return an error (handled per the policy fail_mode).
type Detector interface {
	Name() string
	Detect(ctx context.Context, text string, spec DetectorSpec) ([]Finding, error)
}

// MetricsSink receives guardrail metric updates. The gateway metrics.Recorder
// satisfies it structurally, keeping this package below the gateway layer.
type MetricsSink interface {
	IncGuardrailEvaluation(detector, action, phase string)
	ObserveGuardrailLatency(seconds float64)
}

type noopMetrics struct{}

func (noopMetrics) IncGuardrailEvaluation(string, string, string) {}
func (noopMetrics) ObserveGuardrailLatency(float64)               {}

// Engine holds the detector registry and evaluates text against policies.
type Engine struct {
	detectors map[string]Detector
	metrics   MetricsSink
}

// NewEngine builds an Engine with the built-in local detectors registered.
func NewEngine(metrics MetricsSink) *Engine {
	if metrics == nil {
		metrics = noopMetrics{}
	}
	e := &Engine{detectors: make(map[string]Detector), metrics: metrics}
	e.Register(newPIIDetector())
	e.Register(newSecretsDetector())
	e.Register(newPromptInjectionDetector())
	e.Register(newContentDetector())
	e.Register(newWebhookDetector())
	e.Register(newLLMJudgeDetector(nil)) // no judge until SetJudge
	return e
}

// Register adds or overrides a detector (used for remote detectors too).
func (e *Engine) Register(d Detector) { e.detectors[d.Name()] = d }

// SetJudge wires the llm_judge completion backend (the gateway routes it through
// the provider layer). Until set, an llm_judge detector is a no-op.
func (e *Engine) SetJudge(judge Judge) { e.Register(newLLMJudgeDetector(judge)) }

// Evaluate runs every policy covering phase over text and returns the strongest
// verdict (block > redact > observe). A detector error is handled by the policy
// fail_mode: "closed" blocks, "open" (default) skips that detector.
func (e *Engine) Evaluate(ctx context.Context, phase Phase, text string, policies []Policy) Verdict {
	verdict := Verdict{}
	if text == "" || len(policies) == 0 {
		return verdict
	}
	start := time.Now()
	defer func() { e.metrics.ObserveGuardrailLatency(time.Since(start).Seconds()) }()

	var all []Finding
	for _, p := range policies {
		if !p.Phase.covers(phase) {
			continue
		}
		var policyFindings []Finding
		failClosed := false
		for _, spec := range p.Detectors {
			d, ok := e.detectors[spec.Name]
			if !ok {
				continue
			}
			fs, err := d.Detect(ctx, text, spec)
			if err != nil {
				e.metrics.IncGuardrailEvaluation(spec.Name, "error", string(phase))
				if p.FailMode == "closed" {
					failClosed = true
				}
				continue
			}
			if len(fs) > 0 {
				e.metrics.IncGuardrailEvaluation(spec.Name, string(p.Action), string(phase))
				policyFindings = append(policyFindings, fs...)
			}
		}
		if failClosed {
			if actionRank(ActionBlock) > actionRank(verdict.Action) {
				verdict.Action = ActionBlock
				verdict.PolicyHit = p.Name
			}
			continue
		}
		if len(policyFindings) == 0 {
			continue
		}
		all = append(all, policyFindings...)
		if actionRank(p.Action) > actionRank(verdict.Action) {
			verdict.Action = p.Action
			verdict.PolicyHit = p.Name
		}
	}

	verdict.Findings = all
	switch verdict.Action {
	case ActionBlock:
		verdict.Blocked = true
	case ActionRedact:
		verdict.Redacted = redact(text, all)
	}
	return verdict
}
