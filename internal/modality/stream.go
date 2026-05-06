package modality

import "encoding/json"

type StreamEventKind string

const (
	StreamEventTextDelta            StreamEventKind = "text_delta"
	StreamEventToolCallDelta        StreamEventKind = "tool_call_delta"
	StreamEventThinkingDelta        StreamEventKind = "thinking_delta"
	StreamEventThinkingSignature    StreamEventKind = "thinking_signature"
	StreamEventCitationsDelta       StreamEventKind = "citations_delta"
	StreamEventHostedToolInvocation StreamEventKind = "hosted_tool_invocation"
	StreamEventHostedToolResult     StreamEventKind = "hosted_tool_result"
	StreamEventUsage                StreamEventKind = "usage"
	StreamEventDone                 StreamEventKind = "done"
	StreamEventError                StreamEventKind = "error"
)

type StreamEvent struct {
	Kind      StreamEventKind `json:"type"`
	Text      string          `json:"text,omitempty"`
	ToolCalls []ToolCall      `json:"tool_calls,omitempty"`
	Citations []Citation      `json:"citations,omitempty"`
	Usage     *Usage          `json:"usage,omitempty"`
	Raw       json.RawMessage `json:"raw,omitempty"`
	Err       error           `json:"-"`
}
