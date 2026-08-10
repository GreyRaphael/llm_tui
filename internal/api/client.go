package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// ExecuteTestRequest dispatches the single-turn test payload to the target API endpoint and returns latency & token stats
func ExecuteTestRequest(baseURL, apiKey, apiType, payloadJSON string) *TestResult {
	norm := NormalizeURL(baseURL)
	res := &TestResult{}

	if norm == "" {
		res.Error = "Invalid Base URL"
		return res
	}

	var endpointPath string
	switch apiType {
	case APITypeAnthropic:
		endpointPath = "/messages"
	case APITypeOpenAIResponses:
		endpointPath = "/responses"
	default:
		endpointPath = "/chat/completions"
	}

	urls := BuildEndpointURLs(norm, endpointPath)
	if len(urls) == 0 {
		res.Error = "Failed to construct endpoint URL"
		return res
	}
	targetURL := urls[0] // Primary normalized endpoint URL

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewBufferString(payloadJSON))
	if err != nil {
		res.Error = "Failed to create HTTP request: " + err.Error()
		return res
	}

	req.Header.Set("Content-Type", "application/json")
	if apiType == APITypeAnthropic {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	startTime := time.Now()
	resp, err := client.Do(req)
	res.Latency = time.Since(startTime)

	if err != nil {
		res.Error = "HTTP execution error: " + err.Error()
		return res
	}
	defer resp.Body.Close()

	res.StatusCode = resp.StatusCode
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		res.Error = "Failed to read response body: " + err.Error()
		return res
	}

	res.RawBody = string(bodyBytes)
	res.FormattedBody = FormatJSON(res.RawBody)

	// Extract token usage metrics if present in JSON
	extractTokenUsage(bodyBytes, res)

	return res
}

func extractTokenUsage(body []byte, res *TestResult) {
	var genericMap map[string]interface{}
	if err := json.Unmarshal(body, &genericMap); err != nil {
		return
	}

	// Check standard OpenAI style: "usage": { "prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30 }
	if usage, ok := genericMap["usage"].(map[string]interface{}); ok {
		if pt, ok := usage["prompt_tokens"].(float64); ok {
			res.PromptTokens = int(pt)
		}
		if ct, ok := usage["completion_tokens"].(float64); ok {
			res.CompletionTokens = int(ct)
		}
		if tt, ok := usage["total_tokens"].(float64); ok {
			res.TotalTokens = int(tt)
		}

		// Check Anthropic style inside usage: "input_tokens": 10, "output_tokens": 20
		if res.PromptTokens == 0 {
			if it, ok := usage["input_tokens"].(float64); ok {
				res.PromptTokens = int(it)
			}
		}
		if res.CompletionTokens == 0 {
			if ot, ok := usage["output_tokens"].(float64); ok {
				res.CompletionTokens = int(ot)
			}
		}
		if res.TotalTokens == 0 {
			res.TotalTokens = res.PromptTokens + res.CompletionTokens
		}
	}
}
