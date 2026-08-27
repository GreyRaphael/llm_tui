package api

import (
	"bytes"
	"encoding/json"
	"strings"
)

// IsImageModel checks if a model name indicates an image generation model
func IsImageModel(modelName string) bool {
	lower := strings.ToLower(modelName)
	return strings.Contains(lower, "image") ||
		strings.Contains(lower, "dall-e") ||
		strings.Contains(lower, "flux") ||
		strings.Contains(lower, "imagen") ||
		strings.Contains(lower, "sd") ||
		strings.Contains(lower, "stable-diffusion") ||
		strings.Contains(lower, "midjourney")
}

// GeneratePayloadTemplate generates a preset JSON payload string according to API type, model, and reasoning effort
func GeneratePayloadTemplate(apiType, model, reasoningEffort string) string {
	if model == "" {
		switch apiType {
		case APITypeAnthropic:
			model = "claude-3-5-sonnet-20241022"
		case APITypeOpenAIResponses:
			model = "gpt-4o"
		case APITypeOpenAIImages:
			model = "gemini-3.1-flash-image"
		default:
			model = "gpt-4o"
		}
	}

	var rawMap map[string]interface{}

	switch apiType {
	case APITypeOpenAIImages:
		rawMap = map[string]interface{}{
			"model":           model,
			"prompt":          "a cute little cat sitting on grass, cartoon 3d style",
			"size":            "512x512",
			"n":               1,
			"response_format": "b64_json",
		}

	case APITypeAnthropic:
		rawMap = map[string]interface{}{
			"model": model,
			"messages": []map[string]interface{}{
				{
					"role":    "user",
					"content": "hello, introduce yourself",
				},
			},
			"max_tokens": 1024,
			"stream":     true,
		}
		if reasoningEffort != "" && reasoningEffort != "none" {
			rawMap["output_config"] = map[string]interface{}{
				"effort": reasoningEffort,
			}
		}

	case APITypeOpenAIResponses:
		rawMap = map[string]interface{}{
			"model":  model,
			"input":  "hello, introduce yourself",
			"stream": true,
		}
		if reasoningEffort != "" && reasoningEffort != "none" {
			rawMap["reasoning"] = map[string]interface{}{
				"effort": reasoningEffort,
			}
		}

	default: // APITypeOpenAIChat
		rawMap = map[string]interface{}{
			"model": model,
			"messages": []map[string]interface{}{
				{
					"role":    "user",
					"content": "hello, introduce yourself",
				},
			},
			"stream": true,
		}
		if reasoningEffort != "" && reasoningEffort != "none" {
			rawMap["reasoning_effort"] = reasoningEffort
		}
	}

	buf, err := json.MarshalIndent(rawMap, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(buf)
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
