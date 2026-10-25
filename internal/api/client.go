package api

import (
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

// setAuthHeaders configures provider auth headers. When apiKey is empty, no
// auth header is sent, allowing providers and local servers that require no key.
func setAuthHeaders(h http.Header, apiType, apiKey string) {
	if strings.TrimSpace(apiKey) == "" {
		return
	}
	if apiType == APITypeAnthropic {
		h.Set("x-api-key", apiKey)
	} else {
		h.Set("Authorization", "Bearer "+apiKey)
	}
}

// doPostWithFallback tries the next candidate URL only when the current endpoint
// does not exist. Other responses and transport errors are returned immediately:
// replaying a non-idempotent POST could duplicate a completion or image request.
func doPostWithFallback(ctx context.Context, client *http.Client, urls []string, apiType, apiKey, payloadJSON string) (*http.Response, error) {
	var notFoundResp *http.Response
	closeNotFound := func() {
		if notFoundResp != nil {
			_ = notFoundResp.Body.Close()
			notFoundResp = nil
		}
	}

	for _, targetURL := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewBufferString(payloadJSON))
		if err != nil {
			closeNotFound()
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		setAuthHeaders(req.Header, apiType, apiKey)
		if apiType == APITypeAnthropic {
			req.Header.Set("anthropic-version", "2023-06-01")
		}

		resp, err := client.Do(req)
		if err != nil {
			closeNotFound()
			return nil, err
		}
		if resp.StatusCode == http.StatusOK {
			closeNotFound()
			return resp, nil
		}
		if resp.StatusCode == http.StatusNotFound {
			closeNotFound()
			notFoundResp = resp
			continue
		}

		closeNotFound()
		return resp, nil
	}
	return notFoundResp, nil
}

// ExecuteStreamRequest dispatches a payload and emits SSE deltas over msgChan.
// Providers that answer a streaming request with a complete JSON body are handled
// as one final delta, including pretty-printed JSON documents.
func ExecuteStreamRequest(ctx context.Context, baseURL, apiKey, apiType, payloadJSON string, msgChan chan<- StreamChunkMsg) {
	defer close(msgChan)

	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	if apiType == APITypeOpenAIImages {
		sendChunk(ctx, msgChan, StreamChunkMsg{Err: fmt.Errorf("streaming is not supported for image generation"), Done: true})
		return
	}
	norm := NormalizeURL(baseURL)
	if norm == "" {
		sendChunk(ctx, msgChan, StreamChunkMsg{Err: fmt.Errorf("invalid base URL"), Done: true})
		return
	}

	urls := BuildEndpointURLs(norm, endpointPathForAPIType(apiType))
	if len(urls) == 0 {
		sendChunk(ctx, msgChan, StreamChunkMsg{Err: fmt.Errorf("failed to construct endpoint URL"), Done: true})
		return
	}

	client := &http.Client{}
	startTime := time.Now()
	resp, err := doPostWithFallback(ctx, client, urls, apiType, apiKey, payloadJSON)
	initialLatency := time.Since(startTime)
	if resp == nil {
		if err == nil {
			err = fmt.Errorf("no reachable endpoint")
		}
		sendChunk(ctx, msgChan, StreamChunkMsg{Err: fmt.Errorf("HTTP execution error: %v", err), Latency: initialLatency, Done: true})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := readAllLimited(resp.Body, maxErrorBodyBytes)
		errSnippet := ""
		if readErr != nil {
			errSnippet = readErr.Error()
		} else {
			bodyStr := decodeGBKIfNeeded(body)
			errSnippet = extractErrorMessage(bodyStr)
			if errSnippet == "" {
				errSnippet = bodyStr
			}
		}
		if errSnippet == "" {
			errSnippet = resp.Status
		}
		sendChunk(ctx, msgChan, StreamChunkMsg{
			StatusCode: resp.StatusCode,
			Latency:    time.Since(startTime),
			Err:        fmt.Errorf("HTTP %d: %s", resp.StatusCode, errSnippet),
			Done:       true,
		})
		return
	}

	if !sendChunk(ctx, msgChan, StreamChunkMsg{StatusCode: resp.StatusCode, Latency: initialLatency}) {
		return
	}

	var usage streamUsage
	handleChunk := func(chunk map[string]interface{}) bool {
		usage.addChunk(chunk)
		content, reasoning := extractStreamDelta(chunk)
		if content == "" && reasoning == "" && usage.prompt == 0 && usage.completion == 0 {
			return true
		}
		return sendChunk(ctx, msgChan, StreamChunkMsg{
			ContentDelta:     content,
			ReasoningDelta:   reasoning,
			PromptTokens:     usage.prompt,
			CompletionTokens: usage.completion,
			TotalTokens:      usage.total,
			Latency:          time.Since(startTime),
		})
	}

	var reader io.Reader = resp.Body
	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")
	if !isSSE {
		// Some proxies stream SSE while advertising a JSON content type. Sniff a
		// small prefix instead of buffering the whole body so such streams are
		// still consumed incrementally.
		reader, isSSE, err = sniffSSEStream(resp.Body)
	}
	if err == nil {
		if isSSE {
			err = consumeSSEStream(ctx, reader, handleChunk)
		} else {
			var body []byte
			body, err = readAllLimited(reader, maxResponseBodyBytes)
			if err == nil {
				err = sendCompleteJSONChunk(ctx, body, msgChan, &usage, time.Since(startTime))
			}
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		sendChunk(ctx, msgChan, StreamChunkMsg{Err: fmt.Errorf("stream response error: %v", err), Done: true})
		return
	}

	sendChunk(ctx, msgChan, StreamChunkMsg{
		Done:             true,
		Latency:          time.Since(startTime),
		PromptTokens:     usage.prompt,
		CompletionTokens: usage.completion,
		TotalTokens:      usage.total,
	})
}

func sendCompleteJSONChunk(ctx context.Context, body []byte, msgChan chan<- StreamChunkMsg, usage *streamUsage, latency time.Duration) error {
	bodyStr := decodeGBKIfNeeded(body)
	var response map[string]interface{}
	if err := json.Unmarshal([]byte(bodyStr), &response); err != nil {
		return fmt.Errorf("decode JSON response: %w", err)
	}
	usage.addChunk(response)
	content, reasoning := extractContentAndReasoningFromJSON(response)
	if content == "" && reasoning == "" {
		content = FormatJSON(bodyStr)
	}
	if !sendChunk(ctx, msgChan, StreamChunkMsg{
		ContentDelta:     content,
		ReasoningDelta:   reasoning,
		PromptTokens:     usage.prompt,
		CompletionTokens: usage.completion,
		TotalTokens:      usage.total,
		Latency:          latency,
	}) {
		return ctx.Err()
	}
	return nil
}

// sendChunk delivers a chunk to the TUI without ever blocking forever: once the
// context is cancelled (e.g. the user navigated away and stopped reading the
// channel, or a new request superseded this one) it drops the message and returns
// false so the goroutine can exit instead of leaking on a full channel.
func sendChunk(ctx context.Context, ch chan<- StreamChunkMsg, msg StreamChunkMsg) bool {
	select {
	case ch <- msg:
		return true
	case <-ctx.Done():
		return false
	}
}

// ExecuteTestRequest dispatches the single-turn test payload and returns response metrics.
func ExecuteTestRequest(ctx context.Context, baseURL, apiKey, apiType, payloadJSON string) *TestResult {
	norm := NormalizeURL(baseURL)
	res := &TestResult{}

	if norm == "" {
		res.Error = "Invalid Base URL"
		return res
	}

	urls := BuildEndpointURLs(norm, endpointPathForAPIType(apiType))
	if len(urls) == 0 {
		res.Error = "Failed to construct endpoint URL"
		return res
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	client := &http.Client{}
	startTime := time.Now()
	resp, err := doPostWithFallback(ctx, client, urls, apiType, apiKey, payloadJSON)
	res.Latency = time.Since(startTime)

	if resp == nil {
		if err == nil {
			err = fmt.Errorf("no reachable endpoint")
		}
		res.Error = "HTTP execution error: " + err.Error()
		return res
	}
	defer resp.Body.Close()

	res.StatusCode = resp.StatusCode
	bodyBytes, err := readAllLimited(resp.Body, maxResponseBodyBytes)
	if err != nil {
		res.Error = "Failed to read response body: " + err.Error()
		return res
	}

	// Auto-detect GBK/GB2312 vs UTF-8 encoding and decode to UTF-8
	bodyStr := decodeGBKIfNeeded(bodyBytes)
	res.RawBody = bodyStr

	// Check if response is a JSON error object (including gateways like LiteLLM
	// returning nested error structures). A present but null "error" key (common
	// in OpenAI Responses non-stream payloads) must NOT trigger the error path.
	isErrorJSON := false
	var genericCheck map[string]interface{}
	if err := json.Unmarshal([]byte(bodyStr), &genericCheck); err == nil {
		if errVal, ok := genericCheck["error"]; ok && errVal != nil {
			isErrorJSON = true
		}
	}

	if isErrorJSON {
		// Unescape nested error messages for clean, human-readable display
		readable := unescapeForDisplay(bodyStr)
		res.FormattedBody = readable
	} else if isSSEResponse([]byte(bodyStr)) {
		// If response contains raw SSE stream data (e.g. gateway returned stream logs), parse and assemble
		res.RawBody = assembleSSEResponse([]byte(bodyStr), res)
		res.FormattedBody = FormatJSON(res.RawBody)
	} else {
		res.FormattedBody = FormatJSON(res.RawBody)
	}

	// For OpenAI Images API, automatically decode Base64 and write image files to ./generated_images/.
	if apiType == APITypeOpenAIImages && res.StatusCode == http.StatusOK && !isErrorJSON {
		prompt := ExtractPromptFromPayload(payloadJSON)
		savedPaths, sanitizedBody, saveErr := SaveImagesFromResponse(res.RawBody, prompt)
		res.SavedImages = savedPaths
		switch {
		case saveErr != nil && sanitizedBody != res.RawBody:
			res.FormattedBody = sanitizedBody
			res.Error = fmt.Sprintf("failed to process image response: %v", saveErr)
		case saveErr != nil:
			res.FormattedBody = fmt.Sprintf("Image processing failed: %v", saveErr)
			res.Error = fmt.Sprintf("failed to process image response: %v", saveErr)
		default:
			res.FormattedBody = sanitizedBody
		}
	}

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

// unescapeForDisplay cleans up nested JSON escape sequences for human-readable display
func unescapeForDisplay(s string) string {
	// First, try to pretty-print the JSON structure
	var genericMap map[string]interface{}
	if err := json.Unmarshal([]byte(s), &genericMap); err == nil {
		// Extract the error message string and unescape it
		if errObj, ok := genericMap["error"].(map[string]interface{}); ok {
			if msg, ok := errObj["message"].(string); ok {
				// The message string is already unescaped by json.Unmarshal,
				// but may contain further escaped sequences from nested JSON
				msg = strings.ReplaceAll(msg, `\"`, `"`)
				msg = strings.ReplaceAll(msg, `\\n`, "\n")
				msg = strings.ReplaceAll(msg, `\n`, "\n")
				msg = strings.ReplaceAll(msg, `\\`, `\`)
				return msg
			}
		}
	}
	// Fallback: basic unescape
	result := s
	result = strings.ReplaceAll(result, `\\\"`, `"`)
	result = strings.ReplaceAll(result, `\\n`, "\n")
	result = strings.ReplaceAll(result, `\\\\`, `\`)
	return result
}

func isSSEResponse(body []byte) bool {
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "data:") ||
			(strings.HasPrefix(trimmed, "Original Response:") && strings.Contains(trimmed, "data:")) {
			return true
		}
	}
	return false
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
	var usage streamUsage
	var lastModel string
	var lastID string

	for _, line := range lines {
		chunk, done, ok := parseSSELine(line)
		if done {
			break
		}
		if !ok {
			continue
		}
		if m, ok := chunk["model"].(string); ok && m != "" {
			lastModel = m
		}
		if id, ok := chunk["id"].(string); ok && id != "" {
			lastID = id
		}
		if response, ok := chunk["response"].(map[string]interface{}); ok {
			if m, ok := response["model"].(string); ok && m != "" {
				lastModel = m
			}
			if id, ok := response["id"].(string); ok && id != "" {
				lastID = id
			}
		}
		usage.addChunk(chunk)
		content, reasoning := extractStreamDelta(chunk)
		contentBuilder.WriteString(content)
		reasoningBuilder.WriteString(reasoning)
	}
	res.PromptTokens = usage.prompt
	res.CompletionTokens = usage.completion
	res.TotalTokens = usage.total

	assembledMap := map[string]interface{}{
		"id":     lastID,
		"model":  lastModel,
		"object": "chat.completion",
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

func extractTokenUsageFromMap(usageMap map[string]interface{}, promptTokens, completionTokens, totalTokens *int) {
	if pt, ok := usageMap["prompt_tokens"].(float64); ok {
		*promptTokens = int(pt)
	} else if pt, ok := usageMap["input_tokens"].(float64); ok {
		*promptTokens = int(pt)
	}
	if ct, ok := usageMap["completion_tokens"].(float64); ok {
		*completionTokens = int(ct)
	} else if ct, ok := usageMap["output_tokens"].(float64); ok {
		*completionTokens = int(ct)
	}
	if tt, ok := usageMap["total_tokens"].(float64); ok {
		*totalTokens = int(tt)
	}
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

	usage := streamUsage{
		prompt:     res.PromptTokens,
		completion: res.CompletionTokens,
		total:      res.TotalTokens,
	}
	usage.addChunk(genericMap)
	res.PromptTokens = usage.prompt
	res.CompletionTokens = usage.completion
	res.TotalTokens = usage.total
}
