package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// OpenAIImagesResponse is the schema for POST /v1/images/generations responses
type OpenAIImagesResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		B64JSON string `json:"b64_json"`
		URL     string `json:"url"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

// SanitizeTitle transforms a prompt into a safe, clean file slug
func SanitizeTitle(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "generated_image"
	}
	// Strip special punctuation
	reg := regexp.MustCompile(`[^\p{L}\p{N}_-]+`)
	cleaned := reg.ReplaceAllString(prompt, "_")
	cleaned = strings.Trim(cleaned, "_")
	// Clamp max length to 32 runes
	runes := []rune(cleaned)
	if len(runes) > 32 {
		cleaned = string(runes[:32])
	}
	if cleaned == "" {
		return "generated_image"
	}
	return cleaned
}

// ExtractPromptFromPayload extracts the prompt field from request payload JSON
func ExtractPromptFromPayload(payloadJSON string) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(payloadJSON), &m); err == nil {
		if p, ok := m["prompt"].(string); ok && strings.TrimSpace(p) != "" {
			return strings.TrimSpace(p)
		}
	}
	return "generated_image"
}

// SaveImagesFromResponse extracts base64 images, writes them to ./generated_images/,
// and returns the saved file paths and a truncated JSON representation for TUI display.
func SaveImagesFromResponse(rawJSON string, prompt string) ([]string, string, error) {
	var resp OpenAIImagesResponse
	if err := json.Unmarshal([]byte(rawJSON), &resp); err != nil {
		return nil, rawJSON, err
	}

	if len(resp.Data) == 0 {
		return nil, rawJSON, nil
	}

	outputDir := "./generated_images"
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, rawJSON, fmt.Errorf("failed to create image directory: %w", err)
	}

	slug := SanitizeTitle(prompt)
	timestamp := time.Now().Format("20060102_150405")

	var savedPaths []string
	type sanitizedItem struct {
		SavedPath string `json:"saved_path,omitempty"`
		Size      string `json:"size,omitempty"`
		URL       string `json:"url,omitempty"`
	}
	var sanitizedData []sanitizedItem

	for i, item := range resp.Data {
		b64Str := strings.TrimSpace(item.B64JSON)
		if b64Str != "" {
			// Strip any embedded newlines or carriage returns that proxies/gateways might introduce
			b64Str = strings.ReplaceAll(b64Str, "\r", "")
			b64Str = strings.ReplaceAll(b64Str, "\n", "")

			imgBytes, err := base64.StdEncoding.DecodeString(b64Str)
			if err != nil {
				// Try raw standard or URL decoding
				imgBytes, err = base64.RawStdEncoding.DecodeString(b64Str)
				if err != nil {
					imgBytes, err = base64.URLEncoding.DecodeString(b64Str)
					if err != nil {
						imgBytes, err = base64.RawURLEncoding.DecodeString(b64Str)
						if err != nil {
							continue
						}
					}
				}
			}

			// Detect image format from magic bytes
			ext := ".png"
			if len(imgBytes) > 3 && imgBytes[0] == 0xFF && imgBytes[1] == 0xD8 && imgBytes[2] == 0xFF {
				ext = ".jpg"
			} else if len(imgBytes) > 3 && string(imgBytes[:4]) == "RIFF" {
				ext = ".webp"
			}

			var fileName string
			if len(resp.Data) == 1 {
				fileName = fmt.Sprintf("%s_%s%s", timestamp, slug, ext)
			} else {
				fileName = fmt.Sprintf("%s_%s_%d%s", timestamp, slug, i+1, ext)
			}

			targetPath := filepath.Join(outputDir, fileName)
			if err := os.WriteFile(targetPath, imgBytes, 0644); err != nil {
				return savedPaths, rawJSON, fmt.Errorf("failed to save image to %s: %w", targetPath, err)
			}

			savedPaths = append(savedPaths, targetPath)
			sizeKB := fmt.Sprintf("%.2f KB", float64(len(imgBytes))/1024.0)
			if len(imgBytes) > 1024*1024 {
				sizeKB = fmt.Sprintf("%.2f MB", float64(len(imgBytes))/(1024.0*1024.0))
			}
			sanitizedData = append(sanitizedData, sanitizedItem{
				SavedPath: targetPath,
				Size:      sizeKB,
			})
		} else if item.URL != "" {
			sanitizedData = append(sanitizedData, sanitizedItem{
				URL: item.URL,
			})
		}
	}

	sanitizedMap := map[string]interface{}{
		"created": resp.Created,
		"data":    sanitizedData,
		"summary": fmt.Sprintf("✅ Successfully saved %d image(s) to %s (Press Ctrl+O to open)", len(savedPaths), outputDir),
	}
	sanitizedJSON, err := json.MarshalIndent(sanitizedMap, "", "  ")
	if err != nil {
		return savedPaths, rawJSON, nil
	}

	return savedPaths, string(sanitizedJSON), nil
}

// ErrHeadlessEnvironment is returned when trying to open a graphical viewer in a headless terminal (e.g. over remote SSH)
var ErrHeadlessEnvironment = errors.New("remote headless SSH session: cannot open graphical viewer (DISPLAY or WAYLAND_DISPLAY not set)")

// OpenImageFile opens an image file using the host system's default image viewer
func OpenImageFile(filePath string) error {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return fmt.Errorf("%w. File saved at: %s", ErrHeadlessEnvironment, absPath)
		}
		cmd = exec.Command("xdg-open", absPath)
	case "darwin":
		cmd = exec.Command("open", absPath)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", absPath)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	return cmd.Start()
}
