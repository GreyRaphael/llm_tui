package api

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

var probeHTTPClient = &http.Client{Timeout: 30 * time.Second}

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
		EndpointDetails: make(map[string]string),
	}

	modelName = cmp.Or(modelName, "gpt-4o")

	ctx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()

	type probeSpec struct {
		apiType string
		path    string
		payload map[string]any
	}
	specs := []probeSpec{
		{
			apiType: APITypeOpenAIChat,
			path:    "/chat/completions",
			payload: map[string]any{
				"model": modelName,
				"messages": []map[string]string{
					{"role": "user", "content": "hi"},
				},
			},
		},
		{
			apiType: APITypeOpenAIResponses,
			path:    "/responses",
			payload: map[string]any{
				"model": modelName,
				"input": "hi",
			},
		},
		{
			apiType: APITypeAnthropic,
			path:    "/messages",
			payload: map[string]any{
				"model": modelName,
				"messages": []map[string]string{
					{"role": "user", "content": "hi"},
				},
				"max_tokens": 16,
			},
		},
	}

	type probeOutcome struct {
		supported bool
		msg       string
	}
	outcomes := make([]probeOutcome, len(specs))

	var wg sync.WaitGroup
	for i, spec := range specs {
		wg.Go(func() {
			urls := BuildEndpointURLs(norm, spec.path)
			payload, _ := json.Marshal(spec.payload)
			supported, msg := probeEndpoint(ctx, probeHTTPClient, urls, spec.apiType, apiKey, payload)
			outcomes[i] = probeOutcome{supported: supported, msg: msg}
		})
	}
	wg.Wait()

	for i, spec := range specs {
		result.EndpointDetails[spec.apiType] = outcomes[i].msg
		if outcomes[i].supported {
			result.SupportedAPITypes = append(result.SupportedAPITypes, spec.apiType)
		}
	}

	return result, nil
}

// statusAllowsEndpointFallback reports whether a response identifies an
// unsuitable candidate route rather than a completed provider request. Auth,
// quota, server, and transport failures must not be replayed.
func statusAllowsEndpointFallback(status int) bool {
	return status == http.StatusNotFound || status == http.StatusMethodNotAllowed
}

// readProbeStatus consumes a failed probe response and returns the status
// snippet to report. Draining is best-effort (see drainErrorBody): an oversized,
// truncated, or stalled error body must not abort the fallback chain, so the
// status line is reported instead and the next candidate URL is still tried.
func readProbeStatus(resp *http.Response) string {
	body, complete := drainErrorBody(resp.Body, maxErrorBodyBytes)
	// Closing unblocks a drain that timed out, releasing the connection.
	_ = resp.Body.Close()

	snippet := ""
	if complete {
		snippet = extractErrorMessage(string(body))
	}
	if snippet == "" {
		return resp.Status
	}
	return fmt.Sprintf("%d %s", resp.StatusCode, snippet)
}

func probeEndpoint(ctx context.Context, client *http.Client, urls []string, apiType, apiKey string, payload []byte) (bool, string) {
	var lastStatus string
	for _, targetURL := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(payload))
		if err != nil {
			return false, err.Error()
		}
		req.Header.Set("Content-Type", "application/json")
		setAuthHeaders(req.Header, apiType, apiKey)
		if apiType == APITypeAnthropic {
			req.Header.Set("anthropic-version", "2023-06-01")
		}

		resp, err := client.Do(req)
		if err != nil {
			return false, "Connection error: " + err.Error()
		}
		if resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return true, "200 OK (Supported)"
		}

		errSnippet := readProbeStatus(resp)
		if !statusAllowsEndpointFallback(resp.StatusCode) {
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

		resp, err := requestHTTPClient.Do(req)
		if err != nil {
			requestErrors = append(requestErrors, fmt.Errorf("%s: %w", targetURL, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			requestErrors = append(requestErrors, fmt.Errorf("%s: HTTP %d", targetURL, resp.StatusCode))
			continue
		}
		body, readErr := readAllLimited(resp.Body, maxModelsBodyBytes)
		_ = resp.Body.Close()
		if readErr != nil {
			requestErrors = append(requestErrors, fmt.Errorf("%s: read response: %w", targetURL, readErr))
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
			slices.Sort(models)
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
