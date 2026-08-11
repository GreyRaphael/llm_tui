package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		u = "https://" + u
	}
	return u
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
func ProbeProviderWithModel(baseURL, apiKey, modelName string) (*ProbeResult, error) {
	norm := NormalizeURL(baseURL)
	result := &ProbeResult{
		BaseURL:         norm,
		APIKey:          apiKey,
		EndpointDetails: make(map[string]string),
	}

	if modelName == "" {
		modelName = "gpt-4o"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
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

		supported, msg := probeEndpoint(ctx, client, urls, "Authorization", "Bearer "+apiKey, payload)
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

		supported, msg := probeEndpoint(ctx, client, urls, "Authorization", "Bearer "+apiKey, payload)
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

// ProbeProvider is a backwards-compatible wrapper that fetches models first then probes
func ProbeProvider(baseURL, apiKey string) (*ProbeResult, error) {
	norm := NormalizeURL(baseURL)
	discoveredModels, _ := FetchModels(norm, apiKey)
	modelToUse := "gpt-4o"
	if len(discoveredModels) > 0 {
		modelToUse = discoveredModels[0]
	}
	res, err := ProbeProviderWithModel(baseURL, apiKey, modelToUse)
	if res != nil {
		res.DiscoveredModels = discoveredModels
	}
	return res, err
}

func probeEndpoint(ctx context.Context, client *http.Client, urls []string, authHeader, authVal string, payload []byte) (bool, string) {
	var lastStatus string
	for _, targetURL := range urls {
		req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(payload))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(authHeader, authVal)

		resp, err := client.Do(req)
		if err != nil {
			lastStatus = "Connection error: " + err.Error()
			continue
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

		lastStatus = errSnippet
	}
	return false, lastStatus
}

func probeEndpointAnthropic(ctx context.Context, client *http.Client, urls []string, apiKey string, payload []byte) (bool, string) {
	var lastStatus string
	for _, targetURL := range urls {
		req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(payload))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := client.Do(req)
		if err != nil {
			lastStatus = "Connection error: " + err.Error()
			continue
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

		lastStatus = errSnippet
	}
	return false, lastStatus
}

func extractErrorMessage(bodyStr string) string {
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(bodyStr), &parsed); err == nil {
		if errObj, ok := parsed["error"].(map[string]interface{}); ok {
			if msg, ok := errObj["message"].(string); ok && msg != "" {
				if len(msg) > 40 {
					return msg[:37] + "..."
				}
				return msg
			}
		}
		if msg, ok := parsed["error"].(string); ok && msg != "" {
			if len(msg) > 40 {
				return msg[:37] + "..."
			}
			return msg
		}
	}
	return ""
}

// FetchModels tries to query /models or /v1/models to retrieve available model IDs
func FetchModels(baseURL, apiKey string) ([]string, error) {
	norm := NormalizeURL(baseURL)
	if norm == "" {
		return nil, nil
	}

	urls := BuildEndpointURLs(norm, "/models")
	client := &http.Client{Timeout: 15 * time.Second}

	for _, targetURL := range urls {
		req, err := http.NewRequest("GET", targetURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}

		var parsed ModelsResponse
		if err := json.Unmarshal(body, &parsed); err == nil && len(parsed.Data) > 0 {
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
		}
	}
	return nil, nil
}
