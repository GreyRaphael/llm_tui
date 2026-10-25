package views

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"llm_tui/internal/api"
	"llm_tui/internal/clipboard"
	"llm_tui/internal/db"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

// setStatus records a user-facing message together with its severity so the
// renderer can avoid showing failures in the success badge.
func (m *TesterModel) setStatus(message string, isError bool) {
	m.CopyStatusMsg = message
	m.StatusIsError = isError
}

func (m TesterModel) handleModelsFetched(msg testerModelsFetchedMsg) (TesterModel, tea.Cmd, string) {
	if !finishTrackedRequest(&m.cancelModels, &m.modelsRequestID, msg.id) {
		return m, nil, ""
	}
	if msg.err != nil {
		m.setStatus(fmt.Sprintf("Failed to discover models: %v", msg.err), true)
		return m, nil, ""
	}
	m.DiscoveredModels = msg.models
	for i, model := range m.DiscoveredModels {
		if model == m.Record.Model {
			m.ModelIndex = i
			break
		}
	}
	if len(m.DiscoveredModels) > 0 && m.CopyStatusMsg == "" {
		m.setStatus(fmt.Sprintf("Discovered %d models via /models (Model list length: %d)", len(m.DiscoveredModels), len(m.DiscoveredModels)), false)
	}
	return m, nil, ""
}

func (m TesterModel) handleExecuteFinished(msg executeFinishedMsg) (TesterModel, tea.Cmd, string) {
	if !finishTrackedRequest(&m.cancelExecution, &m.requestID, msg.id) {
		return m, nil, ""
	}
	m.IsExecuting = false
	m.LastResult = msg.result
	m.setViewportContent(msg.result.FormattedBody)
	m.Viewport.GotoTop()
	if len(msg.result.SavedImages) > 0 {
		m.LatestSavedImage = msg.result.SavedImages[0]
		m.setStatus(fmt.Sprintf("🖼️ Saved %d image(s) to ./generated_images! [Ctrl+O] to open.", len(msg.result.SavedImages)), false)
	} else {
		m.setStatus("", false)
	}
	return m, nil, ""
}

func (m TesterModel) handleSizeKey(msg tea.KeyMsg) (TesterModel, tea.Cmd, string) {
	switch msg.String() {
	case "esc":
		m.SelectingAspectRatio = false
	case "up", "k":
		if m.AspectRatioIndex > 0 {
			m.AspectRatioIndex--
		}
	case "down", "j":
		if m.AspectRatioIndex < len(SupportedImageSizes)-1 {
			m.AspectRatioIndex++
		}
	case "enter":
		return m.applyImageSize()
	}
	return m, nil, ""
}

func (m TesterModel) applyImageSize() (TesterModel, tea.Cmd, string) {
	if m.AspectRatioIndex < 0 || m.AspectRatioIndex >= len(SupportedImageSizes) {
		m.SelectingAspectRatio = false
		return m, nil, ""
	}
	preset := SupportedImageSizes[m.AspectRatioIndex]
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(m.Textarea.Value()), &payload); err != nil {
		m.setStatus(fmt.Sprintf("❌ Cannot change image size: invalid JSON payload: %v", err), true)
		return m, nil, ""
	}
	payload["size"] = preset.Size
	for _, key := range []string{"aspect_ratio", "aspectRatio", "image_size", "imageSize", "_imageSize"} {
		delete(payload, key)
	}
	buf, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		m.setStatus(fmt.Sprintf("❌ Cannot change image size: %v", err), true)
		return m, nil, ""
	}
	m.Textarea.SetValue(string(buf))
	m.SelectingAspectRatio = false
	m.Record.CustomPayload = m.Textarea.Value()
	if err := m.DB.UpdateRecord(&m.Record); err != nil {
		m.setStatus(fmt.Sprintf("⚠️ Failed to save image size to DB: %v", err), true)
	} else {
		m.setStatus(fmt.Sprintf("📐 Changed size to '%s' (%s %s)", preset.Size, preset.Ratio, preset.Tier), false)
	}
	return m, nil, ""
}

func (m TesterModel) handleModelKey(msg tea.KeyMsg) (TesterModel, tea.Cmd, string) {
	switch msg.String() {
	case "esc":
		m.SelectingModel = false
	case "up", "k":
		if m.ModelIndex > 0 {
			m.ModelIndex--
		}
	case "down", "j":
		if m.ModelIndex < len(m.DiscoveredModels)-1 {
			m.ModelIndex++
		}
	case "enter":
		if len(m.DiscoveredModels) > 0 && m.ModelIndex >= 0 && m.ModelIndex < len(m.DiscoveredModels) {
			newModel := m.DiscoveredModels[m.ModelIndex]
			m.Record.Model = newModel
			m.Record.Name = newModel
			m.regeneratePayloadPreservingStream()
			m.Record.CustomPayload = m.Textarea.Value()
			if err := m.DB.UpdateRecord(&m.Record); err != nil {
				m.setStatus(fmt.Sprintf("⚠️ Failed to save model switch to DB: %v", err), true)
			} else {
				m.setStatus(fmt.Sprintf("Switched model to '%s' (Model list length: %d)", newModel, len(m.DiscoveredModels)), false)
			}
		}
		m.SelectingModel = false
	}
	return m, nil, ""
}

func (m *TesterModel) regeneratePayloadPreservingStream() {
	currentStream := true
	var oldPayload map[string]interface{}
	if err := json.Unmarshal([]byte(m.Textarea.Value()), &oldPayload); err == nil {
		if stream, ok := oldPayload["stream"].(bool); ok {
			currentStream = stream
		}
	}
	newTemplate := api.GeneratePayloadTemplate(m.Record.APIType, m.Record.Model, m.ReasoningEffort)
	var newPayload map[string]interface{}
	if err := json.Unmarshal([]byte(newTemplate), &newPayload); err == nil {
		if m.Record.APIType != api.APITypeOpenAIImages {
			newPayload["stream"] = currentStream
		}
		if buf, err := json.MarshalIndent(newPayload, "", "  "); err == nil {
			m.Textarea.SetValue(string(buf))
			return
		}
	}
	m.Textarea.SetValue(newTemplate)
}

func (m TesterModel) handleMainKey(msg tea.KeyMsg) (TesterModel, tea.Cmd, string) {
	switch msg.String() {
	case "esc":
		m.CancelRequests()
		return m, nil, "back_to_manager"
	case "alt+a":
		return m.openSizePicker()
	case "ctrl+o":
		return m.openLatestImage()
	case "alt+m":
		if len(m.DiscoveredModels) > 0 {
			m.SelectingModel = true
		} else {
			m.setStatus("No models list available via /models for this provider (Model list length: 0)", false)
		}
		return m, nil, ""
	case "tab", "shift+tab":
		if m.ActivePane == PaneRequest {
			m.ActivePane = PaneResponse
			m.Textarea.Blur()
		} else {
			m.ActivePane = PaneRequest
			m.Textarea.Focus()
		}
		return m, textarea.Blink, ""
	case "ctrl+y":
		if payload := m.Textarea.Value(); payload != "" {
			m.copyToClipboard(payload, "📋 Request Payload JSON copied to clipboard!")
			return m, nil, ""
		}
	case "ctrl+u":
		if m.LastResult != nil && m.LastResult.FormattedBody != "" {
			m.copyToClipboard(m.LastResult.FormattedBody, "📋 Response JSON copied to clipboard!")
			return m, nil, ""
		}
	case "pgdown", "pagedown", "ctrl+f":
		if m.ActivePane == PaneResponse {
			m.Viewport.HalfPageDown()
			return m, nil, ""
		}
	case "pgup", "pageup", "ctrl+b":
		if m.ActivePane == PaneResponse {
			m.Viewport.HalfPageUp()
			return m, nil, ""
		}
	case "ctrl+s", "ctrl+r":
		return m.startExecution()
	case "alt+s":
		return m.toggleStreamMode()
	case "alt+1", "alt+2", "alt+3", "alt+4", "alt+5", "alt+6":
		return m.setReasoningShortcut(msg.String())
	}
	return m.updateActiveComponent(msg)
}

func (m *TesterModel) copyToClipboard(text, successMessage string) {
	if err := clipboard.WriteAll(text); err != nil {
		m.setStatus(fmt.Sprintf("❌ Copy failed: %v", err), true)
		return
	}
	m.setStatus(successMessage, false)
}

func (m TesterModel) openSizePicker() (TesterModel, tea.Cmd, string) {
	if m.Record.APIType != api.APITypeOpenAIImages {
		m.setStatus("Image size presets are only applicable to image generation (openai_images)", false)
		return m, nil, ""
	}
	m.SelectingAspectRatio = true
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(m.Textarea.Value()), &payload); err == nil {
		currentSize, hasSize := payload["size"].(string)
		currentRatio, _ := payload["aspect_ratio"].(string)
		for i, option := range SupportedImageSizes {
			if (hasSize && strings.EqualFold(option.Size, currentSize)) || (!hasSize && option.Ratio == currentRatio) {
				m.AspectRatioIndex = i
				break
			}
		}
	}
	return m, nil, ""
}

func (m TesterModel) openLatestImage() (TesterModel, tea.Cmd, string) {
	if m.LatestSavedImage == "" {
		if m.Record.APIType == api.APITypeOpenAIImages {
			m.setStatus("No image has been generated yet in this session (Press Ctrl+S to generate)", false)
		}
		return m, nil, ""
	}
	if err := api.OpenImageFile(m.LatestSavedImage); err != nil {
		if errors.Is(err, api.ErrHeadlessEnvironment) {
			m.setStatus(fmt.Sprintf("⚠️ %v", err), true)
		} else {
			m.setStatus(fmt.Sprintf("❌ Failed to open image: %v", err), true)
		}
		return m, nil, ""
	}
	m.setStatus(fmt.Sprintf("🖼️ Opened %s in system viewer", m.LatestSavedImage), false)
	return m, nil, ""
}

func (m TesterModel) startExecution() (TesterModel, tea.Cmd, string) {
	if m.IsExecuting {
		m.setStatus("⏳ A request is already running — wait for it to finish or press Esc.", false)
		return m, nil, ""
	}
	m.IsExecuting = true
	m.setStatus("", false)
	m.ReasoningText = ""
	m.ContentText = ""
	m.StreamStatusCode = 0
	m.StreamLatency = 0
	m.StreamPromptTokens = 0
	m.StreamCompletionTokens = 0
	m.StreamTotalTokens = 0
	m.StreamError = ""
	m.LastResult = nil
	m.LastStreamRender = time.Time{}
	m.setViewportContent("")

	m.Record.CustomPayload = m.Textarea.Value()
	m.Record.ReasoningEffort = m.ReasoningEffort
	if err := m.DB.UpdateRecord(&m.Record); err != nil {
		m.setStatus(fmt.Sprintf("⚠️ Failed to save record to DB: %v", err), true)
	}

	var payload map[string]interface{}
	isStream := false
	if err := json.Unmarshal([]byte(m.Textarea.Value()), &payload); err == nil {
		isStream, _ = payload["stream"].(bool)
	} else {
		isStream = strings.Contains(m.Textarea.Value(), `"stream": true`) || strings.Contains(m.Textarea.Value(), `"stream":true`)
	}
	if m.Record.APIType == api.APITypeOpenAIImages {
		isStream = false
	}
	m.IsStreamMode = isStream
	ctx, id := beginTrackedRequest(&m.cancelExecution, &m.requestID)
	if !isStream {
		return m, tea.Batch(m.Spinner.Tick, m.runExecuteNonStreamCmd(ctx, id)), ""
	}
	ch := make(chan api.StreamChunkMsg, 100)
	m.StreamChan = ch
	return m, tea.Batch(m.Spinner.Tick, m.runExecuteStreamCmd(ctx, ch), waitForStreamChunkCmd(id, ch)), ""
}

func (m TesterModel) toggleStreamMode() (TesterModel, tea.Cmd, string) {
	if m.Record.APIType == api.APITypeOpenAIImages {
		m.setStatus("Stream mode is not applicable to image generation", false)
		return m, nil, ""
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(m.Textarea.Value()), &payload); err != nil {
		m.setStatus("❌ Cannot toggle stream: invalid JSON payload", true)
		return m, nil, ""
	}
	currentStream, _ := payload["stream"].(bool)
	payload["stream"] = !currentStream
	buf, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		m.setStatus(fmt.Sprintf("❌ Cannot toggle stream: %v", err), true)
		return m, nil, ""
	}
	m.Textarea.SetValue(string(buf))
	if currentStream {
		m.setStatus("🔄 Stream mode: OFF", false)
	} else {
		m.setStatus("🔄 Stream mode: ON", false)
	}
	return m, nil, ""
}

func (m TesterModel) setReasoningShortcut(key string) (TesterModel, tea.Cmd, string) {
	efforts := map[string]string{
		"alt+1": db.ReasoningEffortNone,
		"alt+2": db.ReasoningEffortLow,
		"alt+3": db.ReasoningEffortMedium,
		"alt+4": db.ReasoningEffortHigh,
		"alt+5": db.ReasoningEffortXHigh,
		"alt+6": db.ReasoningEffortMax,
	}
	m.ReasoningEffort = efforts[key]
	m.regeneratePayloadPreservingStream()
	return m, nil, ""
}

func (m TesterModel) updateActiveComponent(msg tea.Msg) (TesterModel, tea.Cmd, string) {
	var cmd tea.Cmd
	if m.IsExecuting {
		m.Spinner, cmd = m.Spinner.Update(msg)
	} else if m.ActivePane == PaneRequest {
		m.Textarea, cmd = m.Textarea.Update(msg)
	} else {
		m.Viewport, cmd = m.Viewport.Update(msg)
	}
	return m, cmd, ""
}
