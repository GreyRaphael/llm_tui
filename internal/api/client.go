package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ExecuteStreamRequest dispatches the test payload to target API endpoint and streams chunks over msgChan
func ExecuteStreamRequest(baseURL, apiKey, apiType, payloadJSON string, msgChan chan<- StreamChunkMsg) {
	defer close(msgChan)

	norm := NormalizeURL(baseURL)
	if norm == "" {
		msgChan <- StreamChunkMsg{Err: fmt.Errorf("Invalid Base URL"), Done: true}
		return
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
		msgChan <- StreamChunkMsg{Err: fmt.Errorf("Failed to construct endpoint URL"), Done: true}
		return
	}
	targetURL := urls[0]

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewBufferString(payloadJSON))
	if err != nil {
		msgChan <- StreamChunkMsg{Err: fmt.Errorf("Failed to create HTTP request: %v", err), Done: true}
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if apiType == APITypeAnthropic {
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{}
	startTime := time.Now()
	resp, err := client.Do(req)
	initialLatency := time.Since(startTime)

	if err != nil {
		msgChan <- StreamChunkMsg{Err: fmt.Errorf("HTTP execution error: %v", err), Latency: initialLatency, Done: true}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		bodyStr := decodeGBKIfNeeded(bodyBytes)
		errSnippet := extractErrorMessage(bodyStr)
		if errSnippet == "" {
			errSnippet = bodyStr
		}
		msgChan <- StreamChunkMsg{
			StatusCode: resp.StatusCode,
			Latency:    time.Since(startTime),
			Err:        fmt.Errorf("HTTP %d: %s", resp.StatusCode, errSnippet),
			Done:       true,
		}
		return
	}

	// Send initial HTTP status msg
	msgChan <- StreamChunkMsg{
		StatusCode: resp.StatusCode,
		Latency:    initialLatency,
	}

	contentType := resp.Header.Get("Content-Type")
	isSSEHeader := strings.Contains(contentType, "text/event-stream")

	scanner := bufio.NewScanner(resp.Body)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)

	var promptTokens, completionTokens, totalTokens int
	var fullBodyBuilder strings.Builder

	for scanner.Scan() {
		lineBytes := scanner.Bytes()
		lineStr := decodeGBKIfNeeded(lineBytes)
		fullBodyBuilder.WriteString(lineStr)
		fullBodyBuilder.WriteString("\n")

		trimmed := strings.TrimSpace(lineStr)
		if !strings.HasPrefix(trimmed, "data:") {
			if !isSSEHeader && (strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")) {
				var generic map[string]interface{}
				if err := json.Unmarshal([]byte(trimmed), &generic); err == nil {
					res := &TestResult{}
					extractTokenUsage([]byte(trimmed), res)
					content, reasoning := extractContentAndReasoningFromJSON(generic)
					msgChan <- StreamChunkMsg{
						ContentDelta:     content,
						ReasoningDelta:   reasoning,
						PromptTokens:     res.PromptTokens,
						CompletionTokens: res.CompletionTokens,
						TotalTokens:      res.TotalTokens,
					}
				}
			}
			continue
		}

		dataStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
		if dataStr == "" {
			continue
		}
		if dataStr == "[DONE]" {
			break
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			continue
		}

		// Extract Usage
		if usageMap, ok := chunk["usage"].(map[string]interface{}); ok {
			if pt, ok := usageMap["prompt_tokens"].(float64); ok {
				promptTokens = int(pt)
			}
			if ct, ok := usageMap["completion_tokens"].(float64); ok {
				completionTokens = int(ct)
			}
			if tt, ok := usageMap["total_tokens"].(float64); ok {
				totalTokens = int(tt)
			}
			if pt, ok := usageMap["input_tokens"].(float64); ok {
				promptTokens = int(pt)
			}
			if ct, ok := usageMap["output_tokens"].(float64); ok {
				completionTokens = int(ct)
			}
		}

		var contentDelta, reasoningDelta string

		// 1. OpenAI Chat API format
		if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if delta, ok := choice["delta"].(map[string]interface{}); ok {
					if c, ok := delta["content"].(string); ok {
						contentDelta += c
					}
					if r, ok := delta["reasoning_content"].(string); ok {
						reasoningDelta += r
					} else if r, ok := delta["reasoning"].(string); ok {
						reasoningDelta += r
					}
				}
			}
		}

		// 2. Anthropic Messages API format
		if typeVal, ok := chunk["type"].(string); ok {
			switch typeVal {
			case "content_block_delta":
				if delta, ok := chunk["delta"].(map[string]interface{}); ok {
					if deltaType, ok := delta["type"].(string); ok {
						if deltaType == "text_delta" {
							if text, ok := delta["text"].(string); ok {
								contentDelta += text
							}
						} else if deltaType == "thinking_delta" {
							if thinking, ok := delta["thinking"].(string); ok {
								reasoningDelta += thinking
							}
						}
					}
				}
			case "message_start":
				if msgMap, ok := chunk["message"].(map[string]interface{}); ok {
					if usageMap, ok := msgMap["usage"].(map[string]interface{}); ok {
						if it, ok := usageMap["input_tokens"].(float64); ok {
							promptTokens = int(it)
						}
					}
				}
			case "message_delta":
				if usageMap, ok := chunk["usage"].(map[string]interface{}); ok {
					if ot, ok := usageMap["output_tokens"].(float64); ok {
						completionTokens = int(ot)
					}
				}
			}
		}

		// 3. OpenAI Responses API Stream format
		if typeVal, ok := chunk["type"].(string); ok {
			switch typeVal {
			case "response.text.delta":
				if delta, ok := chunk["delta"].(string); ok {
					contentDelta += delta
				}
			case "response.reasoning.delta":
				if delta, ok := chunk["delta"].(string); ok {
					reasoningDelta += delta
				}
			}
		}

		if totalTokens == 0 {
			totalTokens = promptTokens + completionTokens
		}

		if contentDelta != "" || reasoningDelta != "" || promptTokens > 0 || completionTokens > 0 {
			msgChan <- StreamChunkMsg{
				ContentDelta:     contentDelta,
				ReasoningDelta:   reasoningDelta,
				PromptTokens:     promptTokens,
				CompletionTokens: completionTokens,
				TotalTokens:      totalTokens,
				Latency:          time.Since(startTime),
			}
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		msgChan <- StreamChunkMsg{Err: fmt.Errorf("Stream scanner error: %v", err)}
	}

	msgChan <- StreamChunkMsg{
		Done:             true,
		Latency:          time.Since(startTime),
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
}

func extractContentAndReasoningFromJSON(genericMap map[string]interface{}) (string, string) {
	var content, reasoning string

	// OpenAI style
	if choices, ok := genericMap["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if msg, ok := choice["message"].(map[string]interface{}); ok {
				if c, ok := msg["content"].(string); ok {
					content = c
				}
				if r, ok := msg["reasoning_content"].(string); ok {
					reasoning = r
				} else if r, ok := msg["reasoning"].(string); ok {
					reasoning = r
				}
			}
		}
	}

	// Anthropic style
	if contentArr, ok := genericMap["content"].([]interface{}); ok {
		for _, item := range contentArr {
			if itemMap, ok := item.(map[string]interface{}); ok {
				if t, ok := itemMap["type"].(string); ok {
					if t == "text" {
						if text, ok := itemMap["text"].(string); ok {
							content += text
						}
					} else if t == "thinking" {
						if thinking, ok := itemMap["thinking"].(string); ok {
							reasoning += thinking
						}
					}
				}
			}
		}
	}

	return content, reasoning
}

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

	client := &http.Client{}
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
	if json.Valid(data) || utf8.Valid(data) {
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
