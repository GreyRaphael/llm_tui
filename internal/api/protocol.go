package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
)

const maxSSELineBytes = 10 * 1024 * 1024

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
