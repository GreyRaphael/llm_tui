package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
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

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

	client := &http.Client{Timeout: 90 * time.Second}
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

	// Auto-detect GBK/GB2312 vs UTF-8 encoding and decode to UTF-8
	bodyStr := decodeGBKIfNeeded(bodyBytes)
	res.RawBody = bodyStr

	// If response contains embedded SSE stream logs (e.g. LiteLLM/vLLM error wrapping SSE data), parse and assemble
	if isSSEResponse([]byte(bodyStr)) {
		res.RawBody = assembleSSEResponse([]byte(bodyStr), res)
	}

	res.FormattedBody = FormatJSON(res.RawBody)

	// Extract token usage metrics & error if present in JSON
	extractTokenUsage([]byte(res.RawBody), res)

	return res
}

func decodeGBKIfNeeded(data []byte) string {
	// If already valid UTF-8, return directly
	if json.Valid(data) || isUTF8(data) {
		return string(data)
	}
	// Try converting from GBK to UTF-8
	decoder := simplifiedchinese.GBK.NewDecoder()
	utf8Bytes, _, err := transform.Bytes(decoder, data)
	if err == nil {
		return string(utf8Bytes)
	}
	return string(data)
}

func isUTF8(data []byte) bool {
	return bytes.IndexByte(data, 0xff) == -1 && !bytes.Contains(data, []byte{0xc0, 0xaf})
}

func isSSEResponse(body []byte) bool {
	s := string(body)
	return strings.Contains(s, "data:") || strings.Contains(s, "Original Response: data:")
}

func assembleSSEResponse(body []byte, res *TestResult) string {
	bodyStr := string(body)

	// Extract SSE portion if wrapped inside error message string
	if idx := strings.Index(bodyStr, "Original Response:"); idx != -1 {
		bodyStr = bodyStr[idx+len("Original Response:"):]
	}

	lines := strings.Split(bodyStr, "\n")
	var contentBuilder strings.Builder
	var reasoningBuilder strings.Builder
	var lastModel string
	var lastID string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if dataStr == "" || dataStr == "[DONE]" {
			continue
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			continue
		}

		if m, ok := chunk["model"].(string); ok && m != "" {
			lastModel = m
		}
		if id, ok := chunk["id"].(string); ok && id != "" {
			lastID = id
		}

		// Extract usage if present in chunk
		if usageMap, ok := chunk["usage"].(map[string]interface{}); ok {
			if pt, ok := usageMap["prompt_tokens"].(float64); ok {
				res.PromptTokens = int(pt)
			}
			if ct, ok := usageMap["completion_tokens"].(float64); ok {
				res.CompletionTokens = int(ct)
			}
			if tt, ok := usageMap["total_tokens"].(float64); ok {
				res.TotalTokens = int(tt)
			}
		}

		// Extract delta text & reasoning_content
		if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if delta, ok := choice["delta"].(map[string]interface{}); ok {
					if c, ok := delta["content"].(string); ok {
						contentBuilder.WriteString(c)
					}
					if r, ok := delta["reasoning_content"].(string); ok {
						reasoningBuilder.WriteString(r)
					}
				}
			}
		}
	}

	assembledMap := map[string]interface{}{
		"id":      lastID,
		"model":   lastModel,
		"object":  "chat.completion",
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":              "assistant",
					"content":           contentBuilder.String(),
					"reasoning_content": reasoningBuilder.String(),
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     res.PromptTokens,
			"completion_tokens": res.CompletionTokens,
			"total_tokens":      res.TotalTokens,
		},
	}

	// Reset error status if SSE content was successfully assembled
	if contentBuilder.Len() > 0 || reasoningBuilder.Len() > 0 {
		res.Error = ""
	}

	assembledJSON, err := json.Marshal(assembledMap)
	if err != nil {
		return string(body)
	}
	return string(assembledJSON)
}

func extractTokenUsage(body []byte, res *TestResult) {
	var genericMap map[string]interface{}
	if err := json.Unmarshal(body, &genericMap); err != nil {
		return
	}

	// Check if top-level "error" key is present in JSON body
	if res.Error == "" {
		if errVal, ok := genericMap["error"]; ok {
			if errObj, ok := errVal.(map[string]interface{}); ok {
				if msg, ok := errObj["message"].(string); ok && msg != "" {
					res.Error = msg
				}
			} else if errMsg, ok := errVal.(string); ok && errMsg != "" {
				res.Error = errMsg
			}
		}
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
