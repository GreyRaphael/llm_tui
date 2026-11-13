package api

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// requestHTTPClient is shared across non-fixed-timeout streaming and test calls
// (per-request deadlines are governed via context timeouts).
var requestHTTPClient = &http.Client{}

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

// doPostWithFallback tries the next candidate URL only when the current route is
// missing or rejects the POST method. Other responses and transport errors are
// returned immediately to avoid replaying a request that may have done work.
func doPostWithFallback(ctx context.Context, client *http.Client, urls []string, apiType, apiKey, payloadJSON string) (*http.Response, error) {
	var fallbackResp *http.Response
	closeFallback := func() {
		if fallbackResp != nil {
			_ = fallbackResp.Body.Close()
			fallbackResp = nil
		}
	}

	for _, targetURL := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewBufferString(payloadJSON))
		if err != nil {
			closeFallback()
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
			closeFallback()
			return nil, err
		}
		if resp.StatusCode == http.StatusOK {
			closeFallback()
			return resp, nil
		}
		if statusAllowsEndpointFallback(resp.StatusCode) {
			closeFallback()
			fallbackResp = bufferFallbackResponse(resp)
			continue
		}

		closeFallback()
		return resp, nil
	}
	return fallbackResp, nil
}

// bufferFallbackResponse closes the network body of a fallback (404/405)
// response and replaces it with a replay of the bytes read so far. Draining the
// small error body lets the connection be reused while still letting the caller
// report the provider's error message. Draining is best-effort: oversized,
// truncated, or stalled bodies are abandoned instead of aborting the fallback
// chain, in which case the status line is kept as the body.
func bufferFallbackResponse(resp *http.Response) *http.Response {
	var body []byte
	// ContentLength is -1 when the length is unknown (e.g. chunked errors); only
	// skip draining when the provider declares a body that is already too large.
	if resp.ContentLength <= maxErrorBodyBytes {
		if drained, complete := drainErrorBody(resp.Body, maxErrorBodyBytes); complete {
			body = drained
		}
	}
	// Closing unblocks a drain that timed out, releasing the connection.
	_ = resp.Body.Close()
	if body == nil {
		body = []byte(resp.Status)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp
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

	startTime := time.Now()
	resp, err := doPostWithFallback(ctx, requestHTTPClient, urls, apiType, apiKey, payloadJSON)
	initialLatency := time.Since(startTime)
	if resp == nil {
		err = cmp.Or(err, fmt.Errorf("no reachable endpoint"))
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
			errSnippet = cmp.Or(extractErrorMessage(bodyStr), bodyStr)
		}
		errSnippet = cmp.Or(errSnippet, resp.Status)
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
			// A user-initiated cancellation (leaving the screen, superseding the
			// request) ends silently. An internal deadline is a real failure the
			// user must see, so report it even though ctx can no longer be used
			// for delivery.
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				sendTerminalChunk(msgChan, StreamChunkMsg{
					Latency: time.Since(startTime),
					Err:     fmt.Errorf("stream response error: %w", ctx.Err()),
					Done:    true,
				})
			}
			return
		}
		sendChunk(ctx, msgChan, StreamChunkMsg{
			Latency: time.Since(startTime),
			Err:     fmt.Errorf("stream response error: %v", err),
			Done:    true,
		})
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

// sendTerminalChunk delivers a final message after the request context has
// already expired. Delivery is bounded by terminalChunkGrace so the goroutine
// still cannot leak when nobody reads the channel; on expiry the message is
// dropped, which is equivalent to the previous silent behaviour.
func sendTerminalChunk(ch chan<- StreamChunkMsg, msg StreamChunkMsg) bool {
	timer := time.NewTimer(terminalChunkGrace)
	defer timer.Stop()

	select {
	case ch <- msg:
		return true
	case <-timer.C:
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

	startTime := time.Now()
	resp, err := doPostWithFallback(ctx, requestHTTPClient, urls, apiType, apiKey, payloadJSON)
	res.Latency = time.Since(startTime)

	if resp == nil {
		err = cmp.Or(err, fmt.Errorf("no reachable endpoint"))
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
	var parsedBody map[string]any
	hasParsedJSON := json.Unmarshal([]byte(bodyStr), &parsedBody) == nil
	isErrorJSON := hasParsedJSON && parsedBody["error"] != nil

	switch {
	case isErrorJSON:
		res.FormattedBody = unescapeForDisplay(bodyStr, parsedBody)
	case isSSEResponse([]byte(bodyStr)):
		// If response contains raw SSE stream data (e.g. gateway returned stream logs), parse and assemble
		res.RawBody = assembleSSEResponse([]byte(bodyStr), res)
		res.FormattedBody = FormatJSON(res.RawBody)
	default:
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

	if hasParsedJSON {
		applyResponseMetadata(parsedBody, res)
	}

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
func unescapeForDisplay(s string, parsed map[string]any) string {
	if errObj, ok := parsed["error"].(map[string]any); ok {
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
	// Fallback: basic unescape
	result := s
	result = strings.ReplaceAll(result, `\\\"`, `"`)
	result = strings.ReplaceAll(result, `\\n`, "\n")
	result = strings.ReplaceAll(result, `\\\\`, `\`)
	return result
}

func isSSEResponse(body []byte) bool {
	for line := range bytes.SplitSeq(body, []byte("\n")) {
		trimmed := bytes.TrimSpace(line)
		if bytes.HasPrefix(trimmed, []byte("data:")) ||
			(bytes.HasPrefix(trimmed, []byte("Original Response:")) && bytes.Contains(trimmed, []byte("data:"))) {
			return true
		}
	}
	return false
}

// BuildAssembledResponseJSON constructs a normalized chat-completion JSON string
// from accumulated content, reasoning, and token usage metrics.
func BuildAssembledResponseJSON(id, model, content, reasoning string, promptTokens, completionTokens, totalTokens int) (string, error) {
	assembledMap := map[string]any{
		"id":     id,
		"model":  model,
		"object": "chat.completion",
		"choices": []map[string]any{
			{
				"index": 0,
				"message": map[string]any{
					"role":              "assistant",
					"content":           content,
					"reasoning_content": reasoning,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]int{
			"prompt_tokens":     promptTokens,
			"completion_tokens": completionTokens,
			"total_tokens":      totalTokens,
		},
	}
	buf, err := json.Marshal(assembledMap)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func assembleSSEResponse(body []byte, res *TestResult) string {
	bodyStr := string(body)

	// Extract SSE portion if wrapped inside error message string
	if idx := strings.Index(bodyStr, "Original Response:"); idx != -1 {
		bodyStr = bodyStr[idx+len("Original Response:"):]
	}

	var contentBuilder strings.Builder
	var reasoningBuilder strings.Builder
	var usage streamUsage
	var lastModel string
	var lastID string

	for line := range strings.Lines(bodyStr) {
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
		if response, ok := chunk["response"].(map[string]any); ok {
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

	// Reset error status if SSE content was successfully assembled
	if contentBuilder.Len() > 0 || reasoningBuilder.Len() > 0 {
		res.Error = ""
	}

	assembledJSON, err := BuildAssembledResponseJSON(
		lastID, lastModel,
		contentBuilder.String(), reasoningBuilder.String(),
		res.PromptTokens, res.CompletionTokens, res.TotalTokens,
	)
	if err != nil {
		return string(body)
	}
	return assembledJSON
}

func extractTokenUsageFromMap(usageMap map[string]any, promptTokens, completionTokens, totalTokens *int) {
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

func applyResponseMetadata(parsedBody map[string]any, res *TestResult) {
	if res.Error == "" {
		if errVal, ok := parsedBody["error"]; ok {
			switch v := errVal.(type) {
			case map[string]any:
				if msg, ok := v["message"].(string); ok && msg != "" {
					res.Error = msg
				}
			case string:
				if v != "" {
					res.Error = v
				}
			}
		}
	}

	usage := streamUsage{
		prompt:     res.PromptTokens,
		completion: res.CompletionTokens,
		total:      res.TotalTokens,
	}
	usage.addChunk(parsedBody)
	res.PromptTokens = usage.prompt
	res.CompletionTokens = usage.completion
	res.TotalTokens = usage.total
}
