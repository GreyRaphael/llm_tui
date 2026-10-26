package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

const (
	maxSSELineBytes = 10 * 1024 * 1024

	// maxSSESniffBytes bounds how much of a response with a non-SSE
	// Content-Type is inspected before it is treated as a JSON document.
	maxSSESniffBytes = 4 * 1024
)

func endpointPathForAPIType(apiType string) string {
	switch apiType {
	case APITypeAnthropic:
		return "/messages"
	case APITypeOpenAIResponses:
		return "/responses"
	case APITypeOpenAIImages:
		return "/images/generations"
	default:
		return "/chat/completions"
	}
}

// sniffSSEStream classifies a response body whose Content-Type does not say
// "text/event-stream". It reads at most one line ahead and replays everything it
// consumed, so callers still stream incrementally.
//
// The decision is made from whole lines: SSE is recognised when a line starts
// with "data:", and the body is treated as a single JSON document as soon as a
// line starts with a JSON token or no data line shows up within
// maxSSESniffBytes. Newline-delimited JSON (multiple documents) is deliberately
// unsupported: such a body is classified as one JSON document and the decoder
// reports the trailing content as an error.
func sniffSSEStream(r io.Reader) (io.Reader, bool, error) {
	br := bufio.NewReaderSize(r, maxSSESniffBytes)
	var sniffed bytes.Buffer

	for sniffed.Len() < maxSSESniffBytes {
		line, err := br.ReadSlice('\n')
		sniffed.Write(line)
		if sse, decided := classifySniffLine(line); decided {
			return io.MultiReader(&sniffed, br), sse, nil
		}
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, bufio.ErrBufferFull) {
				// Either the body ended or a single line is longer than the
				// sniff window: fall back to treating it as a JSON document.
				return io.MultiReader(&sniffed, br), false, nil
			}
			return io.MultiReader(&sniffed, br), false, err
		}
	}
	return io.MultiReader(&sniffed, br), false, nil
}

// classifySniffLine inspects one line and reports whether the body has been
// classified. A line starting with a JSON token means a single JSON document, a
// line starting with "data:" means SSE. Other lines (comments, event names,
// blank separators) are inconclusive and are skipped.
func classifySniffLine(line []byte) (sse bool, decided bool) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return false, false
	}
	switch trimmed[0] {
	case '{', '[', '"':
		// JSON responses cannot start with an SSE data line.
		return false, true
	}
	if bytes.HasPrefix(trimmed, []byte("data:")) {
		return true, true
	}
	return false, false
}

func parseSSELine(line string) (map[string]interface{}, bool, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "data:") {
		return nil, false, false
	}
	data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
	if data == "" {
		return nil, false, false
	}
	if data == "[DONE]" {
		return nil, true, false
	}
	var chunk map[string]interface{}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil, false, false
	}
	return chunk, false, true
}

func consumeSSEStream(ctx context.Context, r io.Reader, handle func(map[string]interface{}) bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), maxSSELineBytes)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := decodeGBKIfNeeded(scanner.Bytes())
		chunk, done, ok := parseSSELine(line)
		if done {
			return nil
		}
		if ok && !handle(chunk) {
			return ctx.Err()
		}
	}
	return scanner.Err()
}

type streamUsage struct {
	prompt     int
	completion int
	total      int
}

func (u *streamUsage) addChunk(chunk map[string]interface{}) {
	if usage, ok := chunk["usage"].(map[string]interface{}); ok {
		extractTokenUsageFromMap(usage, &u.prompt, &u.completion, &u.total)
	}
	if response, ok := chunk["response"].(map[string]interface{}); ok {
		if usage, ok := response["usage"].(map[string]interface{}); ok {
			extractTokenUsageFromMap(usage, &u.prompt, &u.completion, &u.total)
		}
	}
	if message, ok := chunk["message"].(map[string]interface{}); ok {
		if usage, ok := message["usage"].(map[string]interface{}); ok {
			extractTokenUsageFromMap(usage, &u.prompt, &u.completion, &u.total)
		}
	}
	if combined := u.prompt + u.completion; u.total < combined {
		u.total = combined
	}
}

func extractStreamDelta(chunk map[string]interface{}) (string, string) {
	var content, reasoning string

	if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			if delta, ok := choice["delta"].(map[string]interface{}); ok {
				if text, ok := delta["content"].(string); ok {
					content += text
				}
				if text, ok := delta["reasoning_content"].(string); ok {
					reasoning += text
				} else if text, ok := delta["reasoning"].(string); ok {
					reasoning += text
				}
			}
		}
	}

	typeValue, _ := chunk["type"].(string)
	switch typeValue {
	case "content_block_delta":
		if delta, ok := chunk["delta"].(map[string]interface{}); ok {
			switch delta["type"] {
			case "text_delta":
				if text, ok := delta["text"].(string); ok {
					content += text
				}
			case "thinking_delta":
				if thinking, ok := delta["thinking"].(string); ok {
					reasoning += thinking
				}
			}
		}
	case "response.text.delta", "response.output_text.delta":
		if delta, ok := chunk["delta"].(string); ok {
			content += delta
		}
	case "response.reasoning.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta":
		if delta, ok := chunk["delta"].(string); ok {
			reasoning += delta
		}
	}
	return content, reasoning
}
