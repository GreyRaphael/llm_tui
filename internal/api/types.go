package api

import (
	"time"
)

// Supported API Types
const (
	APITypeOpenAIChat      = "openai_chat"
	APITypeOpenAIResponses = "openai_responses"
	APITypeAnthropic       = "anthropic_messages"
)

// ProbeResult holds the auto-detection results for a provider base_url
type ProbeResult struct {
	BaseURL           string            `json:"base_url"`
	APIKey            string            `json:"api_key"`
	SupportedAPITypes []string          `json:"supported_api_types"`
	DiscoveredModels  []string          `json:"discovered_models"`
	EndpointDetails   map[string]string `json:"endpoint_details"`
}

// TestResult holds response metrics and body from sending a request to an LLM provider
type TestResult struct {
	StatusCode       int           `json:"status_code"`
	Latency          time.Duration `json:"latency"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	TotalTokens      int           `json:"total_tokens"`
	FormattedBody    string        `json:"formatted_body"`
	RawBody          string        `json:"raw_body"`
	Error            string        `json:"error,omitempty"`
}

// ModelsResponse is used for parsing GET /models or GET /v1/models
type ModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}
