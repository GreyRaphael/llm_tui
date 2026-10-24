package api

func extractContentAndReasoningFromJSON(response map[string]interface{}) (string, string) {
	content, reasoning := extractChatContent(response)
	anthropicContent, anthropicReasoning := extractAnthropicContent(response)
	content += anthropicContent
	reasoning += anthropicReasoning
	responsesContent, responsesReasoning := extractResponsesContent(response)
	content += responsesContent
	reasoning += responsesReasoning
	return content, reasoning
}

func extractChatContent(response map[string]interface{}) (string, string) {
	choices, ok := response["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return "", ""
	}
	choice, ok := choices[0].(map[string]interface{})
	if !ok {
		return "", ""
	}
	message, ok := choice["message"].(map[string]interface{})
	if !ok {
		return "", ""
	}
	content, _ := message["content"].(string)
	reasoning, _ := message["reasoning_content"].(string)
	if reasoning == "" {
		reasoning, _ = message["reasoning"].(string)
	}
	return content, reasoning
}

func extractAnthropicContent(response map[string]interface{}) (string, string) {
	parts, ok := response["content"].([]interface{})
	if !ok {
		return "", ""
	}
	var content, reasoning string
	for _, item := range parts {
		part, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		switch part["type"] {
		case "text":
			text, _ := part["text"].(string)
			content += text
		case "thinking":
			thinking, _ := part["thinking"].(string)
			reasoning += thinking
		}
	}
	return content, reasoning
}

func extractResponsesContent(response map[string]interface{}) (string, string) {
	output, _ := response["output"].([]interface{})
	var content, reasoning string
	for _, item := range output {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "reasoning" {
			summary, _ := itemMap["summary"].([]interface{})
			reasoning += textFromParts(summary)
			continue
		}
		parts, _ := itemMap["content"].([]interface{})
		partContent, partReasoning := contentFromResponsesParts(parts)
		content += partContent
		reasoning += partReasoning
	}
	if content == "" {
		content, _ = response["output_text"].(string)
	}
	return content, reasoning
}

func textFromParts(parts []interface{}) string {
	var text string
	for _, item := range parts {
		part, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		value, _ := part["text"].(string)
		text += value
	}
	return text
}

func contentFromResponsesParts(parts []interface{}) (string, string) {
	var content, reasoning string
	for _, item := range parts {
		part, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		text, _ := part["text"].(string)
		switch part["type"] {
		case "output_text", "text":
			content += text
		case "reasoning_text":
			reasoning += text
		}
	}
	return content, reasoning
}
