package guardrails

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Judge runs a single completion for the llm_judge detector. The gateway wires
// an implementation that routes through Polaris's provider layer directly (not
// the HTTP handler), so the judge call never re-enters guardrails or the
// semantic cache — recursion is structurally impossible.
type Judge interface {
	Complete(ctx context.Context, model, prompt string) (string, error)
}

type llmJudgeDetector struct {
	judge Judge
}

func newLLMJudgeDetector(judge Judge) *llmJudgeDetector { return &llmJudgeDetector{judge: judge} }

func (d *llmJudgeDetector) Name() string { return "llm_judge" }

const judgePromptTemplate = `You are a strict content-safety classifier. Decide whether the TEXT below violates the POLICY. Respond with ONLY a compact JSON object: {"flagged": <true|false>, "category": "<short label>", "reason": "<one sentence>"}. Output no other text.

POLICY: %s

TEXT:
%s`

type judgeVerdict struct {
	Flagged  bool   `json:"flagged"`
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

func (d *llmJudgeDetector) Detect(ctx context.Context, text string, spec DetectorSpec) ([]Finding, error) {
	if d.judge == nil || spec.Model == "" {
		return nil, nil
	}
	callCtx := ctx
	if spec.TimeoutMs > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, time.Duration(spec.TimeoutMs)*time.Millisecond)
		defer cancel()
	}
	policy := spec.Prompt
	if policy == "" {
		policy = "unsafe, harmful, or policy-violating content"
	}

	out, err := d.judge.Complete(callCtx, spec.Model, fmt.Sprintf(judgePromptTemplate, policy, text))
	if err != nil {
		return nil, err
	}
	verdict, ok := parseJudgeVerdict(out)
	if !ok {
		return nil, fmt.Errorf("llm_judge: unparseable verdict")
	}
	if !verdict.Flagged {
		return nil, nil
	}
	category := verdict.Category
	if category == "" {
		category = "flagged"
	}
	return []Finding{{Detector: "llm_judge", Type: category, Start: 0, End: len(text), Severity: SeverityHigh}}, nil
}

// parseJudgeVerdict extracts the JSON verdict from model output that may include
// surrounding prose or code fences.
func parseJudgeVerdict(out string) (judgeVerdict, bool) {
	start := strings.Index(out, "{")
	end := strings.LastIndex(out, "}")
	if start < 0 || end <= start {
		return judgeVerdict{}, false
	}
	var v judgeVerdict
	if err := json.Unmarshal([]byte(out[start:end+1]), &v); err != nil {
		return judgeVerdict{}, false
	}
	return v, true
}
