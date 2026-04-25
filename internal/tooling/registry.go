package tooling

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/JiaCheng2004/Polaris/internal/gateway/telemetry"
	"go.opentelemetry.io/otel/attribute"
)

type Result struct {
	Text       string
	Structured any
}

type LocalTool interface {
	Execute(ctx context.Context, arguments json.RawMessage) (Result, error)
	Schema() string
}

type Registry struct {
	tools map[string]LocalTool
}

func NewRegistry() *Registry {
	r := &Registry{tools: map[string]LocalTool{}}
	r.Register("echo", echoTool{})
	r.Register("time.now", timeNowTool{})
	r.Register("math.add", addTool{})
	return r
}

func (r *Registry) Register(name string, tool LocalTool) {
	if r == nil || name == "" || tool == nil {
		return
	}
	r.tools[name] = tool
}

func (r *Registry) Execute(ctx context.Context, implementation string, arguments json.RawMessage) (Result, error) {
	ctx, span := telemetry.StartInternalSpan(ctx, "tool.execute",
		attribute.String("polaris.tool_name", implementation),
	)
	defer span.End()

	if r == nil {
		err := fmt.Errorf("tool registry is nil")
		telemetry.RecordSpanError(span, err)
		return Result{}, err
	}
	tool, ok := r.tools[implementation]
	if !ok {
		err := fmt.Errorf("tool implementation %q is not registered", implementation)
		telemetry.RecordSpanError(span, err)
		return Result{}, err
	}
	result, err := tool.Execute(ctx, arguments)
	if err != nil {
		telemetry.RecordSpanError(span, err)
		return Result{}, err
	}
	return result, nil
}

func (r *Registry) Schema(implementation string) string {
	if r == nil {
		return "{}"
	}
	tool, ok := r.tools[implementation]
	if !ok {
		return "{}"
	}
	return tool.Schema()
}

func (r *Registry) Has(implementation string) bool {
	if r == nil {
		return false
	}
	_, ok := r.tools[implementation]
	return ok
}

type echoTool struct{}

func (echoTool) Execute(_ context.Context, arguments json.RawMessage) (Result, error) {
	var payload struct {
		Text string `json:"text"`
	}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &payload); err != nil {
			return Result{}, err
		}
	}
	return Result{
		Text: payload.Text,
		Structured: map[string]any{
			"text": payload.Text,
		},
	}, nil
}

func (echoTool) Schema() string {
	return `{"type":"object","properties":{"text":{"type":"string"}},"required":["text"]}`
}

type timeNowTool struct{}

func (timeNowTool) Execute(_ context.Context, _ json.RawMessage) (Result, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	return Result{
		Text: now,
		Structured: map[string]any{
			"now": now,
		},
	}, nil
}

func (timeNowTool) Schema() string {
	return `{"type":"object","properties":{}}`
}

type addTool struct{}

func (addTool) Execute(_ context.Context, arguments json.RawMessage) (Result, error) {
	var payload struct {
		A float64 `json:"a"`
		B float64 `json:"b"`
	}
	if len(arguments) > 0 {
		if err := json.Unmarshal(arguments, &payload); err != nil {
			return Result{}, err
		}
	}
	sum := payload.A + payload.B
	return Result{
		Text: fmt.Sprintf("%g", sum),
		Structured: map[string]any{
			"sum": sum,
		},
	}, nil
}

func (addTool) Schema() string {
	return `{"type":"object","properties":{"a":{"type":"number"},"b":{"type":"number"}},"required":["a","b"]}`
}
