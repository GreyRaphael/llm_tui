package api

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestSanitizeTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "generated_image"},
		{"a cute cat on grass", "a_cute_cat_on_grass"},
		{"Hello, World! 3D-Style??", "Hello_World_3D-Style"},
		{"一只有趣的小猫咪在草地上玩耍，很可爱", "一只有趣的小猫咪在草地上玩耍_很可爱"},
		{"   leading and trailing spaces   ", "leading_and_trailing_spaces"},
	}

	for _, tt := range tests {
		got := SanitizeTitle(tt.input)
		if got != tt.want {
			t.Errorf("SanitizeTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractPromptFromPayload(t *testing.T) {
	tests := []struct {
		payload string
		want    string
	}{
		{`{"prompt": "beautiful landscape", "n": 1}`, "beautiful landscape"},
		{`{"prompt": "   ", "model": "gemini-3.1-flash-image"}`, "generated_image"},
		{`invalid json`, "generated_image"},
	}

	for _, tt := range tests {
		got := ExtractPromptFromPayload(tt.payload)
		if got != tt.want {
			t.Errorf("ExtractPromptFromPayload(%q) = %q, want %q", tt.payload, got, tt.want)
		}
	}
}

func TestSaveImagesFromResponse(t *testing.T) {
	defer os.RemoveAll("./generated_images")

	// 1x1 1-pixel red PNG in base64
	samplePNG := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	rawJSON := fmt.Sprintf(`{
		"created": 1787725102,
		"data": [
			{"b64_json": "%s"}
		]
	}`, samplePNG)

	paths, sanitized, err := SaveImagesFromResponse(rawJSON, "red dot test")
	if err != nil {
		t.Fatalf("SaveImagesFromResponse failed: %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("expected 1 saved image, got %d", len(paths))
	}

	if !strings.HasSuffix(paths[0], ".png") {
		t.Errorf("expected file ending with .png, got %s", paths[0])
	}

	if _, err := os.Stat(paths[0]); os.IsNotExist(err) {
		t.Errorf("expected saved file to exist on disk at %s", paths[0])
	}

	if !strings.Contains(sanitized, "Successfully saved 1 image(s)") {
		t.Errorf("expected sanitized summary to indicate success, got: %s", sanitized)
	}
	if strings.Contains(sanitized, samplePNG) {
		t.Errorf("sanitized JSON should not contain raw base64 string")
	}

	// Test resilience against newlines/spaces in b64_json
	rawJSONWithNewlines := fmt.Sprintf(`{
		"created": 1787725102,
		"data": [
			{"b64_json": "  \n%s\r\n  "}
		]
	}`, samplePNG)
	pathsWithNewlines, _, err := SaveImagesFromResponse(rawJSONWithNewlines, "newline test")
	if err != nil {
		t.Fatalf("SaveImagesFromResponse failed on input with newlines: %v", err)
	}
	if len(pathsWithNewlines) != 1 {
		t.Fatalf("expected 1 saved image for input with newlines, got %d", len(pathsWithNewlines))
	}
}

func TestOpenImageFileHeadless(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping Linux headless test on non-Linux platform")
	}

	origDisplay := os.Getenv("DISPLAY")
	origWayland := os.Getenv("WAYLAND_DISPLAY")
	defer func() {
		_ = os.Setenv("DISPLAY", origDisplay)
		_ = os.Setenv("WAYLAND_DISPLAY", origWayland)
	}()

	_ = os.Unsetenv("DISPLAY")
	_ = os.Unsetenv("WAYLAND_DISPLAY")

	err := OpenImageFile("./test.png")
	if err == nil {
		t.Fatal("expected error in headless environment with no DISPLAY/WAYLAND_DISPLAY")
	}

	if !errors.Is(err, ErrHeadlessEnvironment) {
		t.Errorf("expected ErrHeadlessEnvironment, got: %v", err)
	}

	if !strings.Contains(err.Error(), "test.png") {
		t.Errorf("expected error message to mention saved file path, got: %v", err)
	}
}


