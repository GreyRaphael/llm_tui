package api

import (
	"strings"
	"testing"
)

func TestReadAllLimited(t *testing.T) {
	body, err := readAllLimited(strings.NewReader("1234"), 4)
	if err != nil || string(body) != "1234" {
		t.Fatalf("expected body at limit, got %q, %v", body, err)
	}
	if _, err := readAllLimited(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("expected oversized body error")
	}
}

func TestSSEResponseDetectionRequiresDataLine(t *testing.T) {
	jsonBody := []byte(`{"choices":[{"message":{"content":"mention data: without being SSE"}}]}`)
	if isSSEResponse(jsonBody) {
		t.Fatal("JSON content containing data: must not be classified as SSE")
	}
	if !isSSEResponse([]byte("event: message\ndata: {\"delta\":\"ok\"}\n")) {
		t.Fatal("expected SSE data line to be detected")
	}
}
