package api

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"unicode"
)

// IsImageModel checks if a model name indicates an image generation model.
func IsImageModel(modelName string) bool {
	lower := strings.ToLower(modelName)
	for _, kw := range []string{"image", "dall-e", "flux", "imagen", "stable-diffusion", "midjourney"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	// Match standalone "sd" tokens and versioned/XL variants (e.g. "sd-1.5", "sdxl", "sd3")
	// without matching unrelated substrings like "usd" or "sdpa".
	tokens := strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return slices.ContainsFunc(tokens, func(tok string) bool {
		if tok == "sd" || strings.HasPrefix(tok, "sdxl") {
			return true
		}
		return strings.HasPrefix(tok, "sd") && len(tok) > 2 && tok[2] >= '0' && tok[2] <= '9'
	})
}

// GeneratePayloadTemplate generates a preset JSON payload string according to API type, model, and reasoning effort
func GeneratePayloadTemplate(apiType, model, reasoningEffort string) string {
	if model == "" {
		switch apiType {
		case APITypeAnthropic:
			model = "claude-3-5-sonnet-20241022"
		case APITypeOpenAIImages:
			model = "gemini-3.1-flash-image"
		default:
			model = "gpt-4o"
		}
	}

	var rawMap map[string]any

	switch apiType {
	case APITypeOpenAIImages:
		rawMap = map[string]any{
			"model":           model,
			"prompt":          "a cute little cat sitting on grass, cartoon 3d style",
			"size":            "512x512",
			"n":               1,
			"response_format": "b64_json",
		}

	case APITypeAnthropic:
		rawMap = map[string]any{
			"model": model,
			"messages": []map[string]any{
				{
					"role":    "user",
					"content": "hi,  list your abilities.",
				},
			},
			"max_tokens": 1024,
			"stream":     true,
		}

	case APITypeOpenAIResponses:
		rawMap = map[string]any{
			"model":  model,
			"input":  "hi,  list your abilities.",
			"stream": true,
		}

	default: // APITypeOpenAIChat
		rawMap = map[string]any{
			"model": model,
			"messages": []map[string]any{
				{
					"role":    "user",
					"content": "hi,  list your abilities.",
				},
			},
			"stream": true,
		}
	}
	SetReasoningEffortField(rawMap, apiType, reasoningEffort)

	buf, err := json.MarshalIndent(rawMap, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(buf)
}

// SetReasoningEffortField updates or clears the provider-specific reasoning effort
// field inside a parsed request payload map.
func SetReasoningEffortField(payload map[string]any, apiType, reasoningEffort string) {
	hasReasoning := reasoningEffort != "" && reasoningEffort != ReasoningEffortNone
	switch apiType {
	case APITypeOpenAIImages:
		return
	case APITypeAnthropic:
		if hasReasoning {
			payload["output_config"] = map[string]any{"effort": reasoningEffort}
		} else {
			delete(payload, "output_config")
		}
	case APITypeOpenAIResponses:
		if hasReasoning {
			payload["reasoning"] = map[string]any{"effort": reasoningEffort}
		} else {
			delete(payload, "reasoning")
		}
	default: // APITypeOpenAIChat
		if hasReasoning {
			payload["reasoning_effort"] = reasoningEffort
		} else {
			delete(payload, "reasoning_effort")
		}
	}
}

// PatchPayloadJSON parses payloadJSON as a JSON object, applies mutate, and
// returns the formatted JSON string.
func PatchPayloadJSON(payloadJSON string, mutate func(map[string]any)) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return "", err
	}
	if payload == nil {
		payload = make(map[string]any)
	}
	mutate(payload)
	buf, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// FormatJSON pretty-prints a JSON string with 2 spaces indent
func FormatJSON(raw string) string {
	var buf bytes.Buffer
	err := json.Indent(&buf, []byte(raw), "", "  ")
	if err != nil {
		return raw
	}
	return buf.String()
}
