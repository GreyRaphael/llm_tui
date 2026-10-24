package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// NormalizeURL cleans up base_url strings (removes trailing slashes, ensures http/https)
func NormalizeURL(baseURL string) string {
	u := strings.TrimSpace(baseURL)
	u = strings.TrimRight(u, "/")
	if u == "" {
		return ""
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		host := u
		if idx := strings.Index(u, "/"); idx != -1 {
			host = u[:idx]
		}
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		if isLocalHost(host) {
			u = "http://" + u
		} else {
			u = "https://" + u
		}
	}
	return u
}

func isLocalHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "localhost" || h == "127.0.0.1" || h == "0.0.0.0" || h == "::1" || strings.HasSuffix(h, ".local") {
		return true
	}
	if strings.HasPrefix(h, "127.") {
		return true
	}
	return false
}

// BuildEndpointURLs constructs candidate URLs given a base URL and an endpoint path (e.g. "/chat/completions")
func BuildEndpointURLs(baseURL, endpointPath string) []string {
	norm := NormalizeURL(baseURL)
	if norm == "" {
		return nil
	}

	endpointPath = "/" + strings.TrimLeft(endpointPath, "/")

	var urls []string
	if strings.HasSuffix(norm, "/v1") {
		urls = append(urls, norm+endpointPath)
		urls = append(urls, strings.TrimSuffix(norm, "/v1")+endpointPath)
	} else {
		urls = append(urls, norm+"/v1"+endpointPath)
		urls = append(urls, norm+endpointPath)
	}
	return urls
}

// ProbeProviderWithModel auto-detects supported API types for a base URL and API key using a specified model name
func ProbeProviderWithModel(ctx context.Context, baseURL, apiKey, modelName string) (*ProbeResult, error) {
	norm := NormalizeURL(baseURL)
	if norm == "" {
		return nil, fmt.Errorf("invalid base URL")
	}
	result := &ProbeResult{
		BaseURL:         norm,
		APIKey:          apiKey,
		EndpointDetails: make(map[string]string),
	}

	if modelName == "" {
		modelName = "gpt-4o"
	}

	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 30 * time.Second}

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Helper to add supported type
	addResult := func(apiType, statusMsg string, supported bool) {
		mu.Lock()
		defer mu.Unlock()
		result.EndpointDetails[apiType] = statusMsg
		if supported {
			result.SupportedAPITypes = append(result.SupportedAPITypes, apiType)
		}
	}

	// 1. Probe OpenAI Chat API
	wg.Add(1)
	go func() {
		defer wg.Done()
		urls := BuildEndpointURLs(norm, "/chat/completions")
		payload, _ := json.Marshal(map[string]interface{}{
			"model": modelName,
			"messages": []map[string]string{
				{"role": "user", "content": "hi"},
			},
		})

		authVal := ""
		if strings.TrimSpace(apiKey) != "" {
			authVal = "Bearer " + apiKey
		}
		supported, msg := probeEndpoint(ctx, client, urls, "Authorization", authVal, payload)
		addResult(APITypeOpenAIChat, msg, supported)
	}()

	// 2. Probe OpenAI Responses API
	wg.Add(1)
	go func() {
		defer wg.Done()
		urls := BuildEndpointURLs(norm, "/responses")
		payload, _ := json.Marshal(map[string]interface{}{
			"model": modelName,
			"input": "hi",
		})

		authVal := ""
		if strings.TrimSpace(apiKey) != "" {
			authVal = "Bearer " + apiKey
		}
		supported, msg := probeEndpoint(ctx, client, urls, "Authorization", authVal, payload)
		addResult(APITypeOpenAIResponses, msg, supported)
	}()

	// 3. Probe Anthropic Messages API
	wg.Add(1)
	go func() {
		defer wg.Done()
		urls := BuildEndpointURLs(norm, "/messages")
		payload, _ := json.Marshal(map[string]interface{}{
			"model": modelName,
			"messages": []map[string]string{
				{"role": "user", "content": "hi"},
			},
			"max_tokens": 16,
		})

		supported, msg := probeEndpointAnthropic(ctx, client, urls, apiKey, payload)
		addResult(APITypeAnthropic, msg, supported)
	}()

	wg.Wait()

	return result, nil
}

func probeEndpoint(ctx context.Context, client *http.Client, urls []string, authHeader, authVal string, payload []byte) (bool, string) {
	var lastStatus string
	for _, targetURL := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
		if err != nil {
			return false, err.Error()
		}
		req.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(authVal) != "" {
			req.Header.Set(authHeader, authVal)
		}

		resp, err := client.Do(req)
		if err != nil {
			return false, "Connection error: " + err.Error()
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return true, "200 OK (Supported)"
		}

		bodyStr := string(body)
		errSnippet := extractErrorMessage(bodyStr)
		if errSnippet == "" {
			errSnippet = resp.Status
		} else {
			errSnippet = fmt.Sprintf("%d %s", resp.StatusCode, errSnippet)
		}
		if resp.StatusCode != http.StatusNotFound {
			return false, errSnippet
		}
		lastStatus = errSnippet
	}
	return false, lastStatus
}

func probeEndpointAnthropic(ctx context.Context, client *http.Client, urls []string, apiKey string, payload []byte) (bool, string) {
	var lastStatus string
	for _, targetURL := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
		if err != nil {
			return false, err.Error()
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("anthropic-version", "2023-06-01")
		if strings.TrimSpace(apiKey) != "" {
			req.Header.Set("x-api-key", apiKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			return false, "Connection error: " + err.Error()
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return true, "200 OK (Supported)"
		}

		bodyStr := string(body)
		errSnippet := extractErrorMessage(bodyStr)
		if errSnippet == "" {
			errSnippet = resp.Status
		} else {
			errSnippet = fmt.Sprintf("%d %s", resp.StatusCode, errSnippet)
		}
		if resp.StatusCode != http.StatusNotFound {
			return false, errSnippet
		}
		lastStatus = errSnippet
	}
	return false, lastStatus
}

func extractErrorMessage(bodyStr string) string {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(bodyStr), &parsed); err == nil {
		if errObj, ok := parsed["error"].(map[string]interface{}); ok {
			if msg, ok := errObj["message"].(string); ok && msg != "" {
				return truncateRunes(msg, 40)
			}
		}
		if msg, ok := parsed["error"].(string); ok && msg != "" {
			return truncateRunes(msg, 40)
		}
	}
	return ""
}

func truncateRunes(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		if maxLen > 3 {
			return string(runes[:maxLen-3]) + "..."
		}
		return string(runes[:maxLen])
	}
	return s
}

// FetchModels tries to query /models or /v1/models to retrieve available model IDs.
func FetchModels(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	norm := NormalizeURL(baseURL)
	if norm == "" {
		return nil, fmt.Errorf("invalid base URL")
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	urls := BuildEndpointURLs(norm, "/models")
	// Some gateways only serve the model list at the root base rather than under
	// a path prefix. Try prefix-specific URLs first, then the root base.
	if root := rootBaseURL(norm); root != "" && root != norm {
		urls = append(urls, BuildEndpointURLs(root, "/models")...)
	}
	client := &http.Client{}
	var requestErrors []error

	for _, targetURL := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
		if err != nil {
			requestErrors = append(requestErrors, fmt.Errorf("%s: %w", targetURL, err))
			continue
		}
		if strings.TrimSpace(apiKey) != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
			req.Header.Set("x-api-key", apiKey)
		}
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := client.Do(req)
		if err != nil {
			requestErrors = append(requestErrors, fmt.Errorf("%s: %w", targetURL, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			requestErrors = append(requestErrors, fmt.Errorf("%s: read response: %w", targetURL, readErr))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			requestErrors = append(requestErrors, fmt.Errorf("%s: HTTP %d", targetURL, resp.StatusCode))
			continue
		}

		var parsed ModelsResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			requestErrors = append(requestErrors, fmt.Errorf("%s: decode model list: %w", targetURL, err))
			continue
		}
		var models []string
		for _, m := range parsed.Data {
			if m.ID != "" {
				models = append(models, m.ID)
			}
		}
		if len(models) > 0 {
			sort.Strings(models)
			return models, nil
		}
		requestErrors = append(requestErrors, fmt.Errorf("%s: empty model list", targetURL))
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(requestErrors) > 0 {
		return nil, errors.Join(requestErrors...)
	}
	return nil, fmt.Errorf("no model endpoints available for %s", norm)
}

// rootBaseURL returns the scheme://host portion of a normalized base URL.
func rootBaseURL(norm string) string {
	u, err := url.Parse(norm)
	if err != nil {
		return ""
	}
	u.Path = ""
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
