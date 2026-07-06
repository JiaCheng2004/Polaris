package guardrails

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookDetector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"findings":[{"detector":"custom","type":"toxicity","start":0,"end":4,"severity":"high"}]}`))
	}))
	defer srv.Close()

	d := newWebhookDetector()
	fs, err := d.Detect(context.Background(), "some text here", DetectorSpec{Name: "webhook", URL: srv.URL})
	if err != nil {
		t.Fatalf("webhook: %v", err)
	}
	if len(fs) != 1 || fs[0].Type != "toxicity" || fs[0].Severity != SeverityHigh {
		t.Fatalf("webhook findings = %+v", fs)
	}
}

func TestWebhookDetectorErrors(t *testing.T) {
	d := newWebhookDetector()
	// Unreachable endpoint → error (handled by fail_mode).
	if _, err := d.Detect(context.Background(), "x", DetectorSpec{Name: "webhook", URL: "http://127.0.0.1:1/nope", TimeoutMs: 50}); err == nil {
		t.Fatal("expected error from unreachable webhook")
	}
	// 5xx → error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := d.Detect(context.Background(), "x", DetectorSpec{Name: "webhook", URL: srv.URL}); err == nil {
		t.Fatal("expected error from webhook 5xx")
	}
	// No URL → no-op, no error.
	if fs, err := d.Detect(context.Background(), "x", DetectorSpec{Name: "webhook"}); err != nil || fs != nil {
		t.Fatalf("no-URL webhook = %v,%v", fs, err)
	}
}

func TestEngineFailModeOnDetectorError(t *testing.T) {
	e := NewEngine(nil)
	badWebhook := DetectorSpec{Name: "webhook", URL: "http://127.0.0.1:1/x", TimeoutMs: 50}

	// fail_mode closed → block when the detector errors.
	v := e.Evaluate(context.Background(), PhaseRequest, "text", []Policy{
		{Name: "p", Phase: PhaseRequest, Action: ActionObserve, FailMode: "closed", Detectors: []DetectorSpec{badWebhook}},
	})
	if !v.Blocked {
		t.Fatal("fail_mode=closed should block when a remote detector errors")
	}

	// fail_mode open (default) → skip, no block.
	v = e.Evaluate(context.Background(), PhaseRequest, "text", []Policy{
		{Name: "p", Phase: PhaseRequest, Action: ActionObserve, FailMode: "open", Detectors: []DetectorSpec{badWebhook}},
	})
	if v.Blocked {
		t.Fatal("fail_mode=open should not block on a remote detector error")
	}
}

type fakeJudge struct {
	calls    int
	response string
	err      error
}

func (j *fakeJudge) Complete(_ context.Context, _, _ string) (string, error) {
	j.calls++
	return j.response, j.err
}

func TestLLMJudgeDetector(t *testing.T) {
	// Flagged verdict → finding.
	d := newLLMJudgeDetector(&fakeJudge{response: `here: {"flagged": true, "category": "toxicity", "reason": "x"}`})
	fs, err := d.Detect(context.Background(), "bad text", DetectorSpec{Name: "llm_judge", Model: "openai/gpt-4o"})
	if err != nil || len(fs) != 1 || fs[0].Type != "toxicity" {
		t.Fatalf("flagged judge = %+v, %v", fs, err)
	}

	// Not flagged → no finding.
	d2 := newLLMJudgeDetector(&fakeJudge{response: `{"flagged": false}`})
	if fs, _ := d2.Detect(context.Background(), "ok", DetectorSpec{Name: "llm_judge", Model: "openai/gpt-4o"}); len(fs) != 0 {
		t.Fatalf("not-flagged judge = %+v", fs)
	}

	// No judge wired → no-op.
	d3 := newLLMJudgeDetector(nil)
	if fs, err := d3.Detect(context.Background(), "x", DetectorSpec{Name: "llm_judge", Model: "openai/gpt-4o"}); err != nil || len(fs) != 0 {
		t.Fatalf("no-judge = %+v, %v", fs, err)
	}

	// Unparseable verdict → error.
	d4 := newLLMJudgeDetector(&fakeJudge{response: "not json at all"})
	if _, err := d4.Detect(context.Background(), "x", DetectorSpec{Name: "llm_judge", Model: "openai/gpt-4o"}); err == nil {
		t.Fatal("expected error on unparseable judge verdict")
	}
}

func TestLLMJudgeNoRecursion(t *testing.T) {
	// Evaluating an llm_judge policy calls the judge exactly once — the judge
	// routes around guardrails, so it never re-enters the engine.
	j := &fakeJudge{response: `{"flagged": false}`}
	e := NewEngine(nil)
	e.SetJudge(j)
	e.Evaluate(context.Background(), PhaseRequest, "some text", []Policy{
		{Name: "p", Phase: PhaseRequest, Action: ActionBlock, Detectors: []DetectorSpec{{Name: "llm_judge", Model: "openai/gpt-4o"}}},
	})
	if j.calls != 1 {
		t.Fatalf("judge called %d times, want exactly 1 (no recursion)", j.calls)
	}
}
