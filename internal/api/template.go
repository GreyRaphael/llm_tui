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
					"content": "Hello, please explain how quantum computing works in 2 sentences.",
				},
			},
			"max_tokens": 1024,
		}
		if reasoningEffort != "" && reasoningEffort != "none" {
			budget := 1024
			switch reasoningEffort {
			case "low":
				budget = 512
			case "medium":
				budget = 1024
			case "high":
				budget = 2048
			}
			rawMap["thinking"] = map[string]interface{}{
				"type":          "enabled",
				"budget_tokens": budget,
			}
		}

	case APITypeOpenAIResponses:
		rawMap = map[string]interface{}{
			"model": model,
			"input": "Hello, please explain how quantum computing works in 2 sentences.",
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
					"content": "Hello, please explain how quantum computing works in 2 sentences.",
				},
			},
			"temperature": 0.7,
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
