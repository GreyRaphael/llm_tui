package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

	// 1b. OpenAI Chat with Reasoning Effort None (should omit reasoning_effort)
	payloadNone := GeneratePayloadTemplate(APITypeOpenAIChat, "gpt-4o", "none")
	var genericNone map[string]interface{}
	if err := json.Unmarshal([]byte(payloadNone), &genericNone); err != nil {
		t.Fatalf("failed to unmarshal none payload: %v", err)
	}
	if _, ok := genericNone["reasoning_effort"]; ok {
		t.Errorf("expected reasoning_effort to be omitted when effort is 'none', got %v", genericNone["reasoning_effort"])
	}

	// 2. OpenAI Responses with Reasoning Effort Max
	payloadResponses := GeneratePayloadTemplate(APITypeOpenAIResponses, "gpt-4o", "max")
	if err := json.Unmarshal([]byte(payloadResponses), &generic); err != nil {
		t.Fatalf("failed to unmarshal responses payload: %v", err)
	}
	reasoning, ok := generic["reasoning"].(map[string]interface{})
	if !ok || reasoning["effort"] != "max" {
		t.Errorf("expected reasoning.effort 'max', got %v", generic["reasoning"])
	}

	// 3. Anthropic with Output Config Effort High
	payloadAnthropic := GeneratePayloadTemplate(APITypeAnthropic, "claude-3-5-sonnet-20241022", "high")
	if err := json.Unmarshal([]byte(payloadAnthropic), &generic); err != nil {
		t.Fatalf("failed to unmarshal anthropic payload: %v", err)
	}
	outputConfig, ok := generic["output_config"].(map[string]interface{})
	if !ok || outputConfig["effort"] != "high" {
		t.Errorf("expected output_config.effort 'high', got %v", generic["output_config"])
	}
	if _, ok := generic["thinking"]; ok {
		t.Errorf("expected thinking parameter to be omitted in Anthropic payload, got %v", generic["thinking"])
	}

	// 3b. Anthropic with Reasoning Effort None
	payloadAnthropicNone := GeneratePayloadTemplate(APITypeAnthropic, "claude-3-5-sonnet-20241022", "none")
	var genericAnthropicNone map[string]interface{}
	if err := json.Unmarshal([]byte(payloadAnthropicNone), &genericAnthropicNone); err != nil {
		t.Fatalf("failed to unmarshal anthropic none payload: %v", err)
	}
	if _, ok := genericAnthropicNone["output_config"]; ok {
		t.Errorf("expected output_config to be omitted when effort is 'none', got %v", genericAnthropicNone["output_config"])
	}
}

func TestSSEResponseAssembly(t *testing.T) {
	sseChunkData := `data:{"model":"glm-5.2","id":"chatcmpl-123","choices":[{"delta":{"content":"Hello! I am ","reasoning_content":"Thinking... "}}]}
data:{"model":"glm-5.2","id":"chatcmpl-123","choices":[{"delta":{"content":"GLM model.","reasoning_content":"Done thinking."}}]}
data:{"model":"glm-5.2","id":"chatcmpl-123","choices":[],"usage":{"prompt_tokens":16,"completion_tokens":236,"total_tokens":252}}
data:[DONE]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseChunkData))
	}))
	defer server.Close()

	res := ExecuteTestRequest(server.URL, "sk-dummy-key", APITypeOpenAIChat, `{"model":"glm-5.2"}`)
	if res.StatusCode != 200 {
		t.Fatalf("expected HTTP 200, got %d", res.StatusCode)
	}
	if res.PromptTokens != 16 || res.CompletionTokens != 236 || res.TotalTokens != 252 {
		t.Errorf("unexpected SSE tokens assembly: %+v", res)
	}
	if !strings.Contains(res.FormattedBody, "Hello! I am GLM model.") {
		t.Errorf("expected assembled content text, got %s", res.FormattedBody)
	}
	if !strings.Contains(res.FormattedBody, "Thinking... Done thinking.") {
		t.Errorf("expected assembled reasoning_content text, got %s", res.FormattedBody)
	}
}

func TestExecuteRequestOmitsAuthHeaderWhenKeyEmpty(t *testing.T) {
	gotAuth := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"ok","choices":[],"usage":{}}`))
	}))
	defer server.Close()

	res := ExecuteTestRequest(server.URL, "", APITypeOpenAIChat, `{"model":"test"}`)
	if res.Error != "" {
		t.Fatalf("unexpected error: %s", res.Error)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", res.StatusCode)
	}
	if gotAuth != "" {
		t.Errorf("expected no Authorization header for empty key, got %q", gotAuth)
	}
}

func TestFetchModelsOmitsAuthHeaderWhenKeyEmpty(t *testing.T) {
	gotAuthorization := "sent"
	gotXAPIKey := "sent"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		gotXAPIKey = r.Header.Get("x-api-key")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"test-model"}]}`))
	}))
	defer server.Close()

	models, err := FetchModels(server.URL, "")
	if err != nil {
		t.Fatalf("FetchModels error: %v", err)
	}
	if len(models) != 1 || models[0] != "test-model" {
		t.Fatalf("unexpected models: %v", models)
	}
	if gotAuthorization != "" {
		t.Errorf("expected no Authorization header for empty key, got %q", gotAuthorization)
	}
	if gotXAPIKey != "" {
		t.Errorf("expected no x-api-key header for empty key, got %q", gotXAPIKey)
	}
}

func TestProbeWithEmptyKey(t *testing.T) {
	var authCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			atomic.AddInt32(&authCount, 1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","choices":[],"usage":{}}`))
	}))
	defer server.Close()

	res, err := ProbeProviderWithModel(server.URL, "", "test-model")
	if err != nil {
		t.Fatalf("ProbeProviderWithModel with empty key error: %v", err)
	}
	if len(res.SupportedAPITypes) == 0 {
		t.Fatal("expected supported API types for no-auth server")
	}
	if n := atomic.LoadInt32(&authCount); n != 0 {
		t.Errorf("expected no request to carry an Authorization header for empty key, got %d", n)
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
	payload := GeneratePayloadTemplate(APITypeOpenAIChat, "gpt-4o", "high")
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

func TestExecuteStreamRequest(t *testing.T) {
	sseData := `data: {"id":"chat-1","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-reasoner","choices":[{"index":0,"delta":{"reasoning_content":"Let me think step by step... "},"finish_reason":null}]}
data: {"id":"chat-1","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-reasoner","choices":[{"index":0,"delta":{"reasoning_content":"Done thinking."},"finish_reason":null}]}
data: {"id":"chat-1","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-reasoner","choices":[{"index":0,"delta":{"content":"The answer is "},"finish_reason":null}]}
data: {"id":"chat-1","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-reasoner","choices":[{"index":0,"delta":{"content":"42."},"finish_reason":null}]}
data: {"id":"chat-1","object":"chat.completion.chunk","created":1700000000,"model":"deepseek-reasoner","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":45,"total_tokens":57}}
data: [DONE]`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseData))
	}))
	defer server.Close()

	ch := make(chan StreamChunkMsg, 50)
	go ExecuteStreamRequest(server.URL, "sk-test", APITypeOpenAIChat, `{"model":"deepseek-reasoner","stream":true}`, ch)

	var fullContent strings.Builder
	var fullReasoning strings.Builder
	var finalStatusCode int
	var finalPromptTokens, finalCompletionTokens, finalTotalTokens int
	var isDone bool

	for msg := range ch {
		if msg.StatusCode != 0 {
			finalStatusCode = msg.StatusCode
		}
		if msg.ContentDelta != "" {
			fullContent.WriteString(msg.ContentDelta)
		}
		if msg.ReasoningDelta != "" {
			fullReasoning.WriteString(msg.ReasoningDelta)
		}
		if msg.PromptTokens > 0 {
			finalPromptTokens = msg.PromptTokens
		}
		if msg.CompletionTokens > 0 {
			finalCompletionTokens = msg.CompletionTokens
		}
		if msg.TotalTokens > 0 {
			finalTotalTokens = msg.TotalTokens
		}
		if msg.Done {
			isDone = true
		}
	}

	if finalStatusCode != http.StatusOK {
		t.Errorf("expected HTTP 200, got %d", finalStatusCode)
	}
	if !isDone {
		t.Errorf("expected stream to mark Done: true")
	}
	if fullReasoning.String() != "Let me think step by step... Done thinking." {
		t.Errorf("unexpected reasoning content: %q", fullReasoning.String())
	}
	if fullContent.String() != "The answer is 42." {
		t.Errorf("unexpected answer content: %q", fullContent.String())
	}
	if finalPromptTokens != 12 || finalCompletionTokens != 45 || finalTotalTokens != 57 {
		t.Errorf("unexpected token usage stats: P=%d, C=%d, T=%d", finalPromptTokens, finalCompletionTokens, finalTotalTokens)
	}
}

func TestExecuteStreamRequestOpenAIResponsesFormat(t *testing.T) {
	sseChunkData := `event: response.reasoning_text.delta
data: {"content_index":0,"delta":"Let me think. ","type":"response.reasoning_text.delta"}
event: response.reasoning_text.delta
data: {"content_index":0,"delta":"Done thinking.","type":"response.reasoning_text.delta"}
event: response.output_text.delta
data: {"content_index":0,"delta":"The answer is ","type":"response.output_text.delta"}
event: response.output_text.delta
data: {"content_index":0,"delta":"42.","type":"response.output_text.delta"}
event: response.completed
data: {"response":{"id":"resp-1","output":[],"status":"completed","usage":{"input_tokens":5,"output_tokens":10,"total_tokens":15}},"type":"response.completed"}
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseChunkData))
	}))
	defer server.Close()

	ch := make(chan StreamChunkMsg, 50)
	go ExecuteStreamRequest(server.URL, "", APITypeOpenAIResponses, `{"model":"deepseek-v4-flash","stream":true}`, ch)

	var fullContent strings.Builder
	var fullReasoning strings.Builder
	var finalPromptTokens, finalCompletionTokens, finalTotalTokens int
	var isDone bool

	for msg := range ch {
		if msg.ContentDelta != "" {
			fullContent.WriteString(msg.ContentDelta)
		}
		if msg.ReasoningDelta != "" {
			fullReasoning.WriteString(msg.ReasoningDelta)
		}
		if msg.PromptTokens > 0 {
			finalPromptTokens = msg.PromptTokens
		}
		if msg.CompletionTokens > 0 {
			finalCompletionTokens = msg.CompletionTokens
		}
		if msg.TotalTokens > 0 {
			finalTotalTokens = msg.TotalTokens
		}
		if msg.Done {
			isDone = true
		}
	}

	if fullReasoning.String() != "Let me think. Done thinking." {
		t.Errorf("unexpected reasoning content: %q", fullReasoning.String())
	}
	if fullContent.String() != "The answer is 42." {
		t.Errorf("unexpected answer content: %q", fullContent.String())
	}
	if finalPromptTokens != 5 || finalCompletionTokens != 10 || finalTotalTokens != 15 {
		t.Errorf("unexpected token usage stats: P=%d, C=%d, T=%d", finalPromptTokens, finalCompletionTokens, finalTotalTokens)
	}
	if !isDone {
		t.Errorf("expected stream to mark Done: true")
	}
}
