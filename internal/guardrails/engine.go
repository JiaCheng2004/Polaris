package guardrails

import "time"

// Detector inspects text and returns findings. Detectors must be pure, fast
// (local detectors target <1ms on 4KB), and safe for concurrent use.
type Detector interface {
	Name() string
	Detect(text string, spec DetectorSpec) []Finding
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
	return e
}

// Register adds or overrides a detector (used for remote detectors too).
func (e *Engine) Register(d Detector) { e.detectors[d.Name()] = d }

// Evaluate runs every policy covering phase over text and returns the strongest
// verdict (block > redact > observe). Redaction, when it wins, masks all
// findings. An empty text or no policies yields a no-op verdict.
func (e *Engine) Evaluate(phase Phase, text string, policies []Policy) Verdict {
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
		for _, spec := range p.Detectors {
			d, ok := e.detectors[spec.Name]
			if !ok {
				continue
			}
			fs := d.Detect(text, spec)
			if len(fs) > 0 {
				e.metrics.IncGuardrailEvaluation(spec.Name, string(p.Action), string(phase))
				policyFindings = append(policyFindings, fs...)
			}
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
