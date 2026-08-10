package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestURLNormalization(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"api.openai.com/v1/", "https://api.openai.com/v1"},
		{"http://localhost:8080/v1", "http://localhost:8080/v1"},
		{"  https://api.anthropic.com  ", "https://api.anthropic.com"},
	}

	for _, tt := range tests {
		got := NormalizeURL(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeURL(%q) = %q; want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBuildEndpointURLs(t *testing.T) {
	urls := BuildEndpointURLs("https://api.openai.com", "/chat/completions")
	if len(urls) != 2 {
		t.Fatalf("expected 2 candidate URLs, got %d", len(urls))
	}
	if urls[0] != "https://api.openai.com/v1/chat/completions" {
		t.Errorf("unexpected primary URL: %s", urls[0])
	}
}

func TestGeneratePayloadTemplate(t *testing.T) {
	// 1. OpenAI Chat with Reasoning Effort High
	payloadStr := GeneratePayloadTemplate(APITypeOpenAIChat, "gpt-4o", "high")
	var generic map[string]interface{}
	if err := json.Unmarshal([]byte(payloadStr), &generic); err != nil {
		t.Fatalf("failed to unmarshal generated JSON: %v", err)
	}
	if generic["reasoning_effort"] != "high" {
		t.Errorf("expected reasoning_effort 'high', got %v", generic["reasoning_effort"])
	}

	// 2. OpenAI Responses with Reasoning Effort Medium
	payloadResponses := GeneratePayloadTemplate(APITypeOpenAIResponses, "gpt-4o", "medium")
	if err := json.Unmarshal([]byte(payloadResponses), &generic); err != nil {
		t.Fatalf("failed to unmarshal responses payload: %v", err)
	}
	reasoning, ok := generic["reasoning"].(map[string]interface{})
	if !ok || reasoning["effort"] != "medium" {
		t.Errorf("expected reasoning.effort 'medium', got %v", generic["reasoning"])
	}

	// 3. Anthropic with Thinking High
	payloadAnthropic := GeneratePayloadTemplate(APITypeAnthropic, "claude-3-5-sonnet-20241022", "high")
	if err := json.Unmarshal([]byte(payloadAnthropic), &generic); err != nil {
		t.Fatalf("failed to unmarshal anthropic payload: %v", err)
	}
	thinking, ok := generic["thinking"].(map[string]interface{})
	if !ok || thinking["budget_tokens"] != float64(2048) {
		t.Errorf("expected thinking budget_tokens 2048, got %v", generic["thinking"])
	}
}

func TestProbeProviderAndExecute(t *testing.T) {
	// Mock HTTP Server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions", "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "chatcmpl-123",
				"choices": []map[string]interface{}{
					{"message": map[string]string{"role": "assistant", "content": "Quantum computing uses qubits."}},
				},
				"usage": map[string]interface{}{
					"prompt_tokens":     15,
					"completion_tokens": 25,
					"total_tokens":      40,
				},
			})
		case "/v1/models", "/models":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]string{
					{"id": "gpt-4o"},
					{"id": "gpt-4o-mini"},
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	// Probe provider
	res, err := ProbeProvider(server.URL, "sk-test-key")
	if err != nil {
		t.Fatalf("ProbeProvider error: %v", err)
	}

	if len(res.SupportedAPITypes) == 0 {
		t.Fatalf("expected at least 1 supported API type, got 0")
	}

	foundChat := false
	for _, st := range res.SupportedAPITypes {
		if st == APITypeOpenAIChat {
			foundChat = true
		}
	}
	if !foundChat {
		t.Errorf("expected openai_chat in supported types, got %v", res.SupportedAPITypes)
	}

	if len(res.DiscoveredModels) != 2 {
		t.Errorf("expected 2 discovered models, got %d", len(res.DiscoveredModels))
	}

	// Execute Test Request
	payload := GeneratePayloadTemplate(APITypeOpenAIChat, "gpt-4o", "medium")
	testRes := ExecuteTestRequest(server.URL, "sk-test-key", APITypeOpenAIChat, payload)
	if testRes.Error != "" {
		t.Fatalf("ExecuteTestRequest error: %s", testRes.Error)
	}
	if testRes.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", testRes.StatusCode)
	}
	if testRes.PromptTokens != 15 || testRes.CompletionTokens != 25 || testRes.TotalTokens != 40 {
		t.Errorf("unexpected token stats: %+v", testRes)
	}
	if !strings.Contains(testRes.FormattedBody, "Quantum computing uses qubits.") {
		t.Errorf("expected response text in formatted body: %s", testRes.FormattedBody)
	}
}
