package api

import (
	"io"
	"strings"
	"testing"
)

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

func TestSniffSSEStream(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantSSE bool
	}{
		{
			name:    "sse",
			body:    "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n",
			wantSSE: true,
		},
		{
			name:    "sse with preamble",
			body:    ": keep-alive\nevent: message\ndata: {\"choices\":[]}\n\n",
			wantSSE: true,
		},
		{
			name:    "pretty json",
			body:    "{\n  \"choices\": [{\"message\": {\"content\": \"hi\"}}]\n}\n",
			wantSSE: false,
		},
		{
			name:    "compact json containing data prefix",
			body:    `{"data":"data: not sse"}`,
			wantSSE: false,
		},
		{
			name:    "plain text",
			body:    "upstream connect error",
			wantSSE: false,
		},
		{
			name:    "sse with long keep-alive preamble",
			body:    strings.Repeat(": keep-alive\n", 300) + "data: {\"choices\":[]}\n",
			wantSSE: true,
		},
		{
			name:    "preamble exceeds sniff window",
			body:    strings.Repeat(": keep-alive\n", 400) + "data: {\"choices\":[]}\n",
			wantSSE: false,
		},
		{
			name:    "ndjson is one unsupported document",
			body:    "{\"a\":1}\n{\"b\":2}\n",
			wantSSE: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, gotSSE, err := sniffSSEStream(strings.NewReader(tt.body))
			if err != nil {
				t.Fatalf("sniffSSEStream failed: %v", err)
			}
			if gotSSE != tt.wantSSE {
				t.Fatalf("expected isSSE=%v, got %v", tt.wantSSE, gotSSE)
			}
			replayed, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("reading sniffed body failed: %v", err)
			}
			if string(replayed) != tt.body {
				t.Fatalf("sniffing lost body data: got %q want %q", replayed, tt.body)
			}
		})
	}
}
