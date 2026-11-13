package api

import (
	"time"
)

// Supported API Types
const (
	APITypeOpenAIChat      = "openai_chat"
	APITypeOpenAIResponses = "openai_responses"
	APITypeAnthropic       = "anthropic_messages"
	APITypeOpenAIImages    = "openai_images"
)

// ReasoningEffort constants define supported reasoning effort levels.
const (
	ReasoningEffortNone   = "none"
	ReasoningEffortLow    = "low"
	ReasoningEffortMedium = "medium"
	ReasoningEffortHigh   = "high"
	ReasoningEffortXHigh  = "xhigh"
	ReasoningEffortMax    = "max"
)

// SupportedReasoningEfforts lists reasoning effort levels in Alt+1..Alt+6 order.
var SupportedReasoningEfforts = [...]string{
	ReasoningEffortNone,
	ReasoningEffortLow,
	ReasoningEffortMedium,
	ReasoningEffortHigh,
	ReasoningEffortXHigh,
	ReasoningEffortMax,
}

// ProbeResult holds the auto-detection results for a provider endpoint.
type ProbeResult struct {
	SupportedAPITypes []string
	EndpointDetails   map[string]string
}

// TestResult holds response metrics and body from sending a request to an LLM provider
type TestResult struct {
	StatusCode       int
	Latency          time.Duration
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	FormattedBody    string
	RawBody          string
	SavedImages      []string
	Error            string
}

// ModelsResponse is used for parsing GET /models or GET /v1/models
type ModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// StreamChunkMsg represents a real-time SSE stream delta sent to the TUI update loop
type StreamChunkMsg struct {
	ContentDelta     string
	ReasoningDelta   string
	StatusCode       int
	Latency          time.Duration
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Err              error
	Done             bool
}
