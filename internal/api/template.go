package api

import (
	"bytes"
	"encoding/json"
)

// GeneratePayloadTemplate generates a preset JSON payload string according to API type, model, and reasoning effort
func GeneratePayloadTemplate(apiType, model, reasoningEffort string) string {
	if model == "" {
		switch apiType {
		case APITypeAnthropic:
			model = "claude-3-5-sonnet-20241022"
		case APITypeOpenAIResponses:
			model = "gpt-4o"
		default:
			model = "gpt-4o"
		}
	}

	var rawMap map[string]interface{}

	switch apiType {
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
