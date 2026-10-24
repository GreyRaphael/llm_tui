package api

import "testing"

func TestStreamUsageUpdatesComputedTotal(t *testing.T) {
	var usage streamUsage
	usage.addChunk(map[string]interface{}{
		"message": map[string]interface{}{
			"usage": map[string]interface{}{"input_tokens": 2.0},
		},
	})
	usage.addChunk(map[string]interface{}{
		"usage": map[string]interface{}{"output_tokens": 3.0},
	})
	if usage.prompt != 2 || usage.completion != 3 || usage.total != 5 {
		t.Fatalf("unexpected cumulative usage: %+v", usage)
	}
}
