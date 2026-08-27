package views

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"llm_tui/internal/api"
	"llm_tui/internal/clipboard"
	"llm_tui/internal/db"
	"llm_tui/internal/tui/styles"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

type ActivePane int

const (
	PaneRequest ActivePane = iota
	PaneResponse
)

type ImageSizePreset struct {
	Size  string
	Ratio string
	Tier  string
	Desc  string
}

var SupportedImageSizes = []ImageSizePreset{
	// 0.5K (极速预览)
	{"512x512", "1:1", "0.5K", "512×512   (1:1 Square - 0.5K Preview, Lowest Token Cost)"},
	{"704x528", "4:3", "0.5K", "704×528   (4:3 Classic Landscape - 0.5K Preview)"},
	{"528x704", "3:4", "0.5K", "528×704   (3:4 Classic Portrait - 0.5K Preview)"},
	{"768x512", "3:2", "0.5K", "768×512   (3:2 DSLR Landscape - 0.5K Preview)"},
	{"512x768", "2:3", "0.5K", "512×768   (2:3 DSLR Portrait - 0.5K Preview)"},
	{"912x512", "16:9", "0.5K", "912×512   (16:9 Widescreen - 0.5K Preview)"},
	{"512x912", "9:16", "0.5K", "512×912   (9:16 Mobile Portrait - 0.5K Preview)"},
	{"1024x438", "21:9", "0.5K", "1024×438  (21:9 Ultrawide - 0.5K Preview)"},

	// 1K (标准清晰度 - 默认推荐)
	{"1024x1024", "1:1", "1K", "1024×1024 (1:1 Square - 1K Standard, Recommended)"},
	{"1408x1056", "4:3", "1K", "1408×1056 (4:3 Classic Landscape - 1K Standard)"},
	{"1056x1408", "3:4", "1K", "1056×1408 (3:4 Classic Portrait - 1K Standard)"},
	{"1536x1024", "3:2", "1K", "1536×1024 (3:2 DSLR Landscape - 1K Standard)"},
	{"1024x1536", "2:3", "1K", "1024×1536 (2:3 DSLR Portrait - 1K Standard)"},
	{"1792x1024", "16:9", "1K", "1792×1024 (16:9 Widescreen - 1K Desktop Wallpaper)"},
	{"1024x1792", "9:16", "1K", "1024×1792 (9:16 Mobile Portrait - 1K Phone Wallpaper)"},
	{"2048x876", "21:9", "1K", "2048×876  (21:9 Ultrawide Cinematic - 1K Standard)"},

	// 2K (生产级高清)
	{"2048x2048", "1:1", "2K", "2048×2048 (1:1 Square - 2K High-Res Production)"},
	{"2816x2112", "4:3", "2K", "2816×2112 (4:3 Classic Landscape - 2K High-Res)"},
	{"2112x2816", "3:4", "2K", "2112×2816 (3:4 Classic Portrait - 2K High-Res)"},
	{"3072x2048", "3:2", "2K", "3072×2048 (3:2 DSLR Landscape - 2K High-Res)"},
	{"2048x3072", "2:3", "2K", "2048×3072 (2:3 DSLR Portrait - 2K High-Res)"},
	{"2752x1536", "16:9", "2K", "2752×1536 (16:9 Widescreen - 2K High-Res Wallpaper)"},
	{"1536x2752", "9:16", "2K", "1536×2752 (9:16 Mobile Portrait - 2K High-Res Phone)"},
	{"4096x1752", "21:9", "2K", "4096×1752 (21:9 Ultrawide Cinematic - 2K High-Res)"},

	// 4K (超清大师)
	{"4096x4096", "1:1", "4K", "4096×4096 (1:1 Square - 4K Ultra-HD Master)"},
	{"5632x4224", "4:3", "4K", "5632×4224 (4:3 Classic Landscape - 4K Ultra-HD)"},
	{"4224x5632", "3:4", "4K", "4224×5632 (3:4 Classic Portrait - 4K Ultra-HD)"},
	{"6144x4096", "3:2", "4K", "6144×4096 (3:2 DSLR Landscape - 4K Ultra-HD)"},
	{"4096x6144", "2:3", "4K", "4096×6144 (2:3 DSLR Portrait - 4K Ultra-HD)"},
	{"5632x3072", "16:9", "4K", "5632×3072 (16:9 Widescreen - 4K Ultra-HD Master)"},
	{"3072x5632", "9:16", "4K", "3072×5632 (9:16 Mobile Portrait - 4K Ultra-HD Master)"},
	{"8192x3504", "21:9", "4K", "8192×3504 (21:9 Ultrawide Cinematic - 4K Ultra-HD)"},
}

// Deprecated: Alias for backward compatibility
var SupportedAspectRatios = SupportedImageSizes

type TesterModel struct {
	DB                   *db.DB
	Record               db.ProviderRecord
	ReasoningEffort      string
	Textarea             textarea.Model
	Viewport             viewport.Model
	Spinner              spinner.Model
	ActivePane           ActivePane
	IsExecuting          bool
	IsStreamMode         bool
	SelectingModel       bool
	SelectingAspectRatio bool
	SelectingSize        bool
	DiscoveredModels     []string
	ModelIndex           int
	AspectRatioIndex     int
	SizeIndex            int
	LastResult           *api.TestResult
	LatestSavedImage     string
	CopyStatusMsg        string
	Width                int
	Height               int
	StreamChan       chan api.StreamChunkMsg
	// StreamID is bumped on every stream request send; each streamed chunk is
	// tagged with the id of the request it belongs to so stale chunks from a
	// superseded request are dropped instead of polluting the current stream.
	StreamID int
	// CancelStream cancels the in-flight stream request so its goroutine and
	// HTTP connection are released when the user navigates away or quits.
	CancelStream           context.CancelFunc
	ReasoningText          string
	ContentText            string
	StreamStatusCode       int
	StreamLatency          time.Duration
	StreamPromptTokens     int
	StreamCompletionTokens int
	StreamTotalTokens      int
	StreamError            string
	LastRawViewportContent string
	LastStreamRender       time.Time
}

type executeFinishedMsg struct {
	result *api.TestResult
}

type testerModelsFetchedMsg struct {
	models []string
	err    error
}

// streamChunkMsg wraps an api.StreamChunkMsg with the id of the request it
// belongs to. Because bubbletea cannot cancel an already-running command goroutine
// (which may be blocked reading an old channel), a stale chunk can be delivered
// after a new request starts; the id lets the model drop it.
type streamChunkMsg struct {
	id  int
	msg api.StreamChunkMsg
}

func renderMarkdown(text string, width int) string {
	if text == "" {
		return ""
	}
	if width <= 0 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return text
	}
	rendered, err := r.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimSpace(rendered)
}

// formatTPS returns tokens-per-second as a short string, or "-" when neither
// latency nor token count is available yet.
func formatTPS(totalTokens int, latency time.Duration) string {
	secs := latency.Seconds()
	if secs <= 0 || totalTokens <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2f", float64(totalTokens)/secs)
}

// formatLatency renders a duration in seconds with two decimal places.
func formatLatency(latency time.Duration) string {
	return fmt.Sprintf("%.2fs", latency.Seconds())
}

func NewTesterModel(database *db.DB, record db.ProviderRecord) TesterModel {
	ta := textarea.New()
	ta.Placeholder = "Enter request JSON payload here..."
	ta.Focus()
	ta.SetWidth(40)
	ta.SetHeight(16)

	vp := viewport.New(40, 16)

	s := spinner.New()
	s.Spinner = spinner.Dot

	reasoning := record.ReasoningEffort
	if reasoning == "" {
		reasoning = db.ReasoningEffortNone
	}

	initialPayload := record.CustomPayload
	if initialPayload == "" {
		initialPayload = api.GeneratePayloadTemplate(record.APIType, record.Model, reasoning)
	}
	ta.SetValue(initialPayload)

	return TesterModel{
		DB:              database,
		Record:          record,
		ReasoningEffort: reasoning,
		Textarea:        ta,
		Viewport:        vp,
		Spinner:         s,
		ActivePane:      PaneRequest,
	}
}

func (m *TesterModel) setViewportContent(content string) {
	m.LastRawViewportContent = content
	if content == "" {
		m.Viewport.SetContent("")
		return
	}
	if m.Viewport.Width > 0 {
		wrapped := lipgloss.NewStyle().Width(m.Viewport.Width).Render(content)
		m.Viewport.SetContent(wrapped)
	} else {
		m.Viewport.SetContent(content)
	}
}

func (m *TesterModel) Resize(w, h int) {
	m.Width = w
	m.Height = h

	totalWidth := w - 4
	halfWidth := (totalWidth - 2) / 2
	if halfWidth < 35 {
		halfWidth = 40
	}

	paneHeight := h - 13
	if paneHeight < 12 {
		paneHeight = 16
	}

	m.Textarea.SetWidth(halfWidth - 4)
	m.Textarea.SetHeight(paneHeight - 2)

	m.Viewport.Width = halfWidth - 4
	m.Viewport.Height = paneHeight - 5

	if m.LastRawViewportContent != "" {
		m.setViewportContent(m.LastRawViewportContent)
	}
}

func (m TesterModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.runFetchModelsCmd())
}

func (m TesterModel) Update(msg tea.Msg) (TesterModel, tea.Cmd, string) {
	var action string
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case testerModelsFetchedMsg:
		m.DiscoveredModels = msg.models
		for i, md := range m.DiscoveredModels {
			if md == m.Record.Model {
				m.ModelIndex = i
				break
			}
		}
		if len(m.DiscoveredModels) > 0 && m.CopyStatusMsg == "" {
			m.CopyStatusMsg = fmt.Sprintf("Discovered %d models via /models (Model list length: %d)", len(m.DiscoveredModels), len(m.DiscoveredModels))
		}
		return m, nil, ""

	case executeFinishedMsg:
		m.IsExecuting = false
		m.LastResult = msg.result
		m.setViewportContent(msg.result.FormattedBody)
		m.Viewport.GotoTop()
		if len(msg.result.SavedImages) > 0 {
			m.LatestSavedImage = msg.result.SavedImages[0]
			m.CopyStatusMsg = fmt.Sprintf("🖼️ Saved %d image(s) to ./generated_images! [Ctrl+O] to open.", len(msg.result.SavedImages))
		} else {
			m.CopyStatusMsg = ""
		}
		return m, nil, ""

	case streamChunkMsg:
		if msg.id != m.StreamID {
			// A chunk from a superseded request can still be delivered because
			// bubbletea cannot cancel a command goroutine already blocked on an
			// old channel — drop it so it cannot pollute the current stream.
			return m, nil, ""
		}
		return m.handleStreamChunk(msg.msg)

	case api.StreamChunkMsg:
		// Legacy direct entry point (also used by unit tests); it is treated as
		// belonging to the current stream.
		return m.handleStreamChunk(msg)

	case tea.KeyMsg:
		if m.SelectingAspectRatio || m.SelectingSize {
			switch msg.String() {
			case "esc":
				m.SelectingAspectRatio = false
				m.SelectingSize = false
				return m, nil, ""
			case "up", "k":
				if m.AspectRatioIndex > 0 {
					m.AspectRatioIndex--
					m.SizeIndex = m.AspectRatioIndex
				}
				return m, nil, ""
			case "down", "j":
				if m.AspectRatioIndex < len(SupportedImageSizes)-1 {
					m.AspectRatioIndex++
					m.SizeIndex = m.AspectRatioIndex
				}
				return m, nil, ""
			case "enter":
				if m.AspectRatioIndex >= 0 && m.AspectRatioIndex < len(SupportedImageSizes) {
					preset := SupportedImageSizes[m.AspectRatioIndex]
					var payloadMap map[string]interface{}
					if err := json.Unmarshal([]byte(m.Textarea.Value()), &payloadMap); err == nil {
						payloadMap["size"] = preset.Size
						delete(payloadMap, "aspect_ratio")
						delete(payloadMap, "aspectRatio")
						delete(payloadMap, "image_size")
						delete(payloadMap, "imageSize")
						delete(payloadMap, "_imageSize")
						if buf, err := json.MarshalIndent(payloadMap, "", "  "); err == nil {
							m.Textarea.SetValue(string(buf))
						}
					}
					m.Record.CustomPayload = m.Textarea.Value()
					_ = m.DB.UpdateRecord(&m.Record)
					m.CopyStatusMsg = fmt.Sprintf("📐 Changed size to '%s' (%s %s)", preset.Size, preset.Ratio, preset.Tier)
				}
				m.SelectingAspectRatio = false
				m.SelectingSize = false
				return m, nil, ""
			}
			return m, nil, ""
		}

		if m.SelectingModel {
			switch msg.String() {
			case "esc":
				m.SelectingModel = false
				return m, nil, ""
			case "up", "k":
				if m.ModelIndex > 0 {
					m.ModelIndex--
				}
				return m, nil, ""
			case "down", "j":
				if m.ModelIndex < len(m.DiscoveredModels)-1 {
					m.ModelIndex++
				}
				return m, nil, ""
			case "enter":
				if len(m.DiscoveredModels) > 0 && m.ModelIndex < len(m.DiscoveredModels) {
					newModel := m.DiscoveredModels[m.ModelIndex]
					m.Record.Model = newModel
					m.Record.Name = newModel
					// Preserve current stream setting when regenerating template
					currentStream := true
					var oldPayload map[string]interface{}
					if err := json.Unmarshal([]byte(m.Textarea.Value()), &oldPayload); err == nil {
						if s, ok := oldPayload["stream"].(bool); ok {
							currentStream = s
						}
					}
					newTemplate := api.GeneratePayloadTemplate(m.Record.APIType, m.Record.Model, m.ReasoningEffort)
					var newPayload map[string]interface{}
					if err := json.Unmarshal([]byte(newTemplate), &newPayload); err == nil {
						newPayload["stream"] = currentStream
						if buf, err := json.MarshalIndent(newPayload, "", "  "); err == nil {
							m.Textarea.SetValue(string(buf))
						} else {
							m.Textarea.SetValue(newTemplate)
						}
					} else {
						m.Textarea.SetValue(newTemplate)
					}
					m.Record.CustomPayload = m.Textarea.Value()
					if err := m.DB.UpdateRecord(&m.Record); err != nil {
						m.CopyStatusMsg = fmt.Sprintf("⚠️ Failed to save model switch to DB: %v", err)
					} else {
						m.CopyStatusMsg = fmt.Sprintf("Switched model to '%s' (Model list length: %d)", newModel, len(m.DiscoveredModels))
					}
				}
				m.SelectingModel = false
				return m, nil, ""
			}
			return m, nil, ""
		}

		switch msg.String() {
		case "esc":
			// Cancel any in-flight stream before leaving so its goroutine and
			// HTTP connection do not leak while the user is on another screen.
			m.CancelStreamRequest()
			action = "back_to_manager"
			return m, nil, action

		case "alt+a":
			if m.Record.APIType == api.APITypeOpenAIImages {
				m.SelectingAspectRatio = true
				m.SelectingSize = true
				var payloadMap map[string]interface{}
				if err := json.Unmarshal([]byte(m.Textarea.Value()), &payloadMap); err == nil {
					currSize, hasSize := payloadMap["size"].(string)
					currRatio, _ := payloadMap["aspect_ratio"].(string)
					for idx, opt := range SupportedImageSizes {
						if hasSize && (opt.Size == currSize || strings.EqualFold(opt.Size, currSize)) {
							m.AspectRatioIndex = idx
							m.SizeIndex = idx
							break
						} else if !hasSize && opt.Ratio == currRatio {
							m.AspectRatioIndex = idx
							m.SizeIndex = idx
							break
						}
					}
				}
			} else {
				m.CopyStatusMsg = "Image size presets are only applicable to image generation (openai_images)"
			}
			return m, nil, ""

		case "ctrl+o":
			if m.LatestSavedImage != "" {
				err := api.OpenImageFile(m.LatestSavedImage)
				if err != nil {
					m.CopyStatusMsg = fmt.Sprintf("❌ Failed to open image: %v", err)
				} else {
					m.CopyStatusMsg = fmt.Sprintf("🖼️ Opened %s in system viewer", m.LatestSavedImage)
				}
			} else if m.Record.APIType == api.APITypeOpenAIImages {
				m.CopyStatusMsg = "No image has been generated yet in this session (Press Ctrl+S to generate)"
			}
			return m, nil, ""

		case "alt+m":
			if len(m.DiscoveredModels) > 0 {
				m.SelectingModel = true
			} else {
				m.CopyStatusMsg = "No models list available via /models for this provider (Model list length: 0)"
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

		// Copy Request JSON with Ctrl+Y
		case "ctrl+y":
			reqPayload := m.Textarea.Value()
			if reqPayload != "" {
				err := clipboard.WriteAll(reqPayload)
				if err == nil {
					m.CopyStatusMsg = "📋 Request Payload JSON copied to clipboard!"
				} else {
					m.CopyStatusMsg = fmt.Sprintf("❌ Copy failed: %v", err)
				}
				return m, nil, ""
			}

		// Copy Response JSON with Ctrl+U
		case "ctrl+u":
			if m.LastResult != nil && m.LastResult.FormattedBody != "" {
				err := clipboard.WriteAll(m.LastResult.FormattedBody)
				if err == nil {
					m.CopyStatusMsg = "📋 Response JSON copied to clipboard!"
				} else {
					m.CopyStatusMsg = fmt.Sprintf("❌ Copy failed: %v", err)
				}
				return m, nil, ""
			}

		// Page Down / Page Up scrolling for Response Viewport
		case "pgdown", "pagedown", "ctrl+f":
			if m.ActivePane == PaneResponse {
				m.Viewport.HalfViewDown()
				return m, nil, ""
			}
		case "pgup", "pageup", "ctrl+b":
			if m.ActivePane == PaneResponse {
				m.Viewport.HalfViewUp()
				return m, nil, ""
			}

		case "ctrl+s", "ctrl+r":
			// Guard against a second send while a request is already running:
			// instead of stacking concurrent streams (and risking stale chunks
			// bleeding into the new response), reject it with a hint.
			if m.IsExecuting {
				m.CopyStatusMsg = "⏳ A request is already running — wait for it to finish or press Esc."
				return m, nil, ""
			}
			m.CancelStreamRequest()
			m.IsExecuting = true
			m.CopyStatusMsg = ""
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
				m.CopyStatusMsg = fmt.Sprintf("⚠️ Failed to save record to DB: %v", err)
			}

			// Parse stream property from request payload
			var payloadMap map[string]interface{}
			isStream := false
			if err := json.Unmarshal([]byte(m.Textarea.Value()), &payloadMap); err == nil {
				if s, ok := payloadMap["stream"].(bool); ok {
					isStream = s
				}
			} else {
				if strings.Contains(m.Textarea.Value(), `"stream": true`) || strings.Contains(m.Textarea.Value(), `"stream":true`) {
					isStream = true
				}
			}
			m.IsStreamMode = isStream

			if isStream {
				execCmd, ch := m.runExecuteStreamCmd()
				m.StreamChan = ch
				return m, tea.Batch(m.Spinner.Tick, execCmd), ""
			} else {
				return m, tea.Batch(m.Spinner.Tick, m.runExecuteNonStreamCmd()), ""
			}

		case "alt+s":
			// Toggle stream parameter in payload JSON
			var payloadMap map[string]interface{}
			if err := json.Unmarshal([]byte(m.Textarea.Value()), &payloadMap); err == nil {
				currentStream := false
				if s, ok := payloadMap["stream"].(bool); ok {
					currentStream = s
				}
				payloadMap["stream"] = !currentStream
				buf, err := json.MarshalIndent(payloadMap, "", "  ")
				if err == nil {
					m.Textarea.SetValue(string(buf))
				}
				if !currentStream {
					m.CopyStatusMsg = "🔄 Stream mode: ON"
				} else {
					m.CopyStatusMsg = "🔄 Stream mode: OFF"
				}
			} else {
				m.CopyStatusMsg = "❌ Cannot toggle stream: invalid JSON payload"
			}
			return m, nil, ""

		case "alt+1", "alt+2", "alt+3", "alt+4":
			switch msg.String() {
			case "alt+1":
				m.ReasoningEffort = db.ReasoningEffortNone
			case "alt+2":
				m.ReasoningEffort = db.ReasoningEffortLow
			case "alt+3":
				m.ReasoningEffort = db.ReasoningEffortHigh
			case "alt+4":
				m.ReasoningEffort = db.ReasoningEffortMax
			}
			// Preserve current stream setting when regenerating template
			currentStream := true
			var oldPayload map[string]interface{}
			if err := json.Unmarshal([]byte(m.Textarea.Value()), &oldPayload); err == nil {
				if s, ok := oldPayload["stream"].(bool); ok {
					currentStream = s
				}
			}
			newTemplate := api.GeneratePayloadTemplate(m.Record.APIType, m.Record.Model, m.ReasoningEffort)
			var newPayload map[string]interface{}
			if err := json.Unmarshal([]byte(newTemplate), &newPayload); err == nil {
				newPayload["stream"] = currentStream
				if buf, err := json.MarshalIndent(newPayload, "", "  "); err == nil {
					m.Textarea.SetValue(string(buf))
				} else {
					m.Textarea.SetValue(newTemplate)
				}
			} else {
				m.Textarea.SetValue(newTemplate)
			}
			return m, nil, ""
		}
	}

	if m.IsExecuting {
		m.Spinner, cmd = m.Spinner.Update(msg)
	} else if m.ActivePane == PaneRequest {
		m.Textarea, cmd = m.Textarea.Update(msg)
	} else {
		m.Viewport, cmd = m.Viewport.Update(msg)
	}

	return m, cmd, action
}

// handleStreamChunk applies a single streamed chunk from the current request to
// the model, re-renders the response viewport (throttled at ~30 FPS during streaming),
// and either schedules the next chunk read (StreamID-tagged) or finalizes the result when done.
func (m TesterModel) handleStreamChunk(msg api.StreamChunkMsg) (TesterModel, tea.Cmd, string) {
	if msg.StatusCode != 0 {
		m.StreamStatusCode = msg.StatusCode
	}
	if msg.Latency != 0 {
		m.StreamLatency = msg.Latency
	}
	if msg.PromptTokens > 0 {
		m.StreamPromptTokens = msg.PromptTokens
	}
	if msg.CompletionTokens > 0 {
		m.StreamCompletionTokens = msg.CompletionTokens
	}
	if msg.TotalTokens > 0 {
		m.StreamTotalTokens = msg.TotalTokens
	}
	if msg.Err != nil {
		m.StreamError = msg.Err.Error()
	}

	if msg.ReasoningDelta != "" {
		m.ReasoningText += msg.ReasoningDelta
	}
	if msg.ContentDelta != "" {
		m.ContentText += msg.ContentDelta
	}

	// When stream is still in progress, throttle viewport reflow & repainting to ~30 FPS (~33ms)
	// and use lightweight text layout without expensive markdown formatting.
	if !msg.Done {
		shouldRender := m.LastStreamRender.IsZero() || time.Since(m.LastStreamRender) >= 33*time.Millisecond
		if shouldRender {
			var formattedContent string
			if m.ReasoningText != "" {
				var rBuilder strings.Builder
				rBuilder.WriteString(styles.BadgeAccentStyle.Render("💭 Thinking Process") + "\n")
				rBuilder.WriteString(styles.HelpStyle.Render("-------------------") + "\n")
				rBuilder.WriteString(m.ReasoningText + "\n\n")
				rBuilder.WriteString(styles.BadgeSuccessStyle.Render("💬 Response Content") + "\n")
				rBuilder.WriteString(styles.HelpStyle.Render("-------------------") + "\n")
				rBuilder.WriteString(m.ContentText)
				formattedContent = rBuilder.String()
			} else if m.ContentText != "" {
				formattedContent = m.ContentText
			} else if m.StreamError != "" {
				formattedContent = styles.ErrorStyle.Render("Error: " + m.StreamError)
			}

			m.setViewportContent(formattedContent)
			if m.IsExecuting {
				m.Viewport.GotoBottom()
			}
			m.LastStreamRender = time.Now()
		}

		return m, waitForStreamChunkCmd(m.StreamID, m.StreamChan), ""
	}

	// Stream is complete (msg.Done == true): perform final high-quality formatting & markdown rendering.
	m.IsExecuting = false
	m.StreamChan = nil
	m.CancelStreamRequest()

	reasoningText := m.ReasoningText
	contentText := m.ContentText

	var formattedContent string
	if reasoningText != "" {
		var rBuilder strings.Builder
		rBuilder.WriteString(styles.BadgeAccentStyle.Render("💭 Thinking Process") + "\n")
		rBuilder.WriteString(styles.HelpStyle.Render("-------------------") + "\n")

		renderedReasoning := renderMarkdown(reasoningText, m.Viewport.Width)
		if renderedReasoning != "" {
			rBuilder.WriteString(renderedReasoning + "\n\n")
		} else {
			rBuilder.WriteString(reasoningText + "\n\n")
		}

		rBuilder.WriteString(styles.BadgeSuccessStyle.Render("💬 Response Content") + "\n")
		rBuilder.WriteString(styles.HelpStyle.Render("-------------------") + "\n")

		renderedContent := renderMarkdown(contentText, m.Viewport.Width)
		if renderedContent != "" {
			rBuilder.WriteString(renderedContent)
		} else {
			rBuilder.WriteString(contentText)
		}
		formattedContent = rBuilder.String()
	} else if contentText != "" {
		if json.Valid([]byte(contentText)) {
			formattedContent = api.FormatJSON(contentText)
		} else {
			renderedContent := renderMarkdown(contentText, m.Viewport.Width)
			if renderedContent != "" {
				formattedContent = renderedContent
			} else {
				formattedContent = contentText
			}
		}
	} else if m.StreamError != "" {
		formattedContent = styles.ErrorStyle.Render("Error: " + m.StreamError)
	}

	var assembledJSON string
	if reasoningText != "" || contentText != "" {
		assembledMap := map[string]interface{}{
			"model": m.Record.Model,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":              "assistant",
						"content":           contentText,
						"reasoning_content": reasoningText,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     m.StreamPromptTokens,
				"completion_tokens": m.StreamCompletionTokens,
				"total_tokens":      m.StreamTotalTokens,
			},
		}
		buf, err := json.Marshal(assembledMap)
		if err == nil {
			assembledJSON = string(buf)
		} else {
			assembledJSON = contentText
		}
	} else if m.StreamError != "" {
		assembledJSON = fmt.Sprintf(`{"error": %q}`, m.StreamError)
	}

	m.LastResult = &api.TestResult{
		StatusCode:       m.StreamStatusCode,
		Latency:          m.StreamLatency,
		PromptTokens:     m.StreamPromptTokens,
		CompletionTokens: m.StreamCompletionTokens,
		TotalTokens:      m.StreamTotalTokens,
		RawBody:          assembledJSON,
		FormattedBody:    api.FormatJSON(assembledJSON),
		Error:            m.StreamError,
	}
	m.CopyStatusMsg = ""

	m.setViewportContent(formattedContent)
	m.Viewport.GotoBottom()
	return m, nil, ""
}

func (m TesterModel) runFetchModelsCmd() tea.Cmd {
	baseURL := m.Record.BaseURL
	apiKey := m.Record.APIKey
	return func() tea.Msg {
		models, err := api.FetchModels(baseURL, apiKey)
		return testerModelsFetchedMsg{models: models, err: err}
	}
}

func (m TesterModel) runExecuteNonStreamCmd() tea.Cmd {
	baseURL := m.Record.BaseURL
	apiKey := m.Record.APIKey
	apiType := m.Record.APIType
	payload := m.Textarea.Value()

	return func() tea.Msg {
		res := api.ExecuteTestRequest(baseURL, apiKey, apiType, payload)
		return executeFinishedMsg{result: res}
	}
}

func (m *TesterModel) runExecuteStreamCmd() (tea.Cmd, chan api.StreamChunkMsg) {
	// Every send gets a fresh id (and context) so that a stream superseded by a
	// new request can be cancelled and its late chunks dropped by id.
	m.StreamID++
	ctx, cancel := context.WithCancel(context.Background())
	m.CancelStream = cancel

	baseURL := m.Record.BaseURL
	apiKey := m.Record.APIKey
	apiType := m.Record.APIType
	payload := m.Textarea.Value()

	ch := make(chan api.StreamChunkMsg, 100)
	go api.ExecuteStreamRequest(ctx, baseURL, apiKey, apiType, payload, ch)
	return waitForStreamChunkCmd(m.StreamID, ch), ch
}

// CancelStreamRequest cancels the in-flight stream (if any) so its goroutine and
// HTTP connection are released promptly when the user navigates away or quits,
// instead of leaking until the hard timeout expires.
func (m *TesterModel) CancelStreamRequest() {
	if m.CancelStream != nil {
		m.CancelStream()
		m.CancelStream = nil
	}
}

func waitForStreamChunkCmd(id int, ch chan api.StreamChunkMsg) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return streamChunkMsg{id: id, msg: api.StreamChunkMsg{Done: true}}
		}
		msg, ok := <-ch
		if !ok {
			return streamChunkMsg{id: id, msg: api.StreamChunkMsg{Done: true}}
		}
		return streamChunkMsg{id: id, msg: msg}
	}
}

func (m TesterModel) View() string {
	var sb strings.Builder

	var header string
	if m.Record.APIType == api.APITypeOpenAIImages {
		header = styles.HeaderStyle.Render(fmt.Sprintf("🖼️ AI Image Laboratory: %s (%s)", m.Record.Model, m.Record.APIType))
	} else {
		header = styles.HeaderStyle.Render(fmt.Sprintf("🧪 LLM Chat Laboratory: %s (%s)", m.Record.Model, m.Record.APIType))
	}
	sb.WriteString(header + "\n\n")

	// Info Card with Model Switcher & Aspect Ratio Badges
	var modelBadge string
	if len(m.DiscoveredModels) > 0 {
		modelBadge = fmt.Sprintf("Model: %s [Press Alt+M to switch (Model list length: %d)]", m.Record.Model, len(m.DiscoveredModels))
	} else {
		modelBadge = fmt.Sprintf("Model: %s [Press Alt+M to switch (Model list length: 0)]", m.Record.Model)
	}

	var sizeBadge string
	if m.Record.APIType == api.APITypeOpenAIImages {
		currSizeDisplay := "512x512 (1:1 0.5K)"
		var payloadMap map[string]interface{}
		if err := json.Unmarshal([]byte(m.Textarea.Value()), &payloadMap); err == nil {
			if s, ok := payloadMap["size"].(string); ok && s != "" {
				matched := false
				for _, opt := range SupportedImageSizes {
					if strings.EqualFold(opt.Size, s) {
						currSizeDisplay = fmt.Sprintf("%s (%s %s)", opt.Size, opt.Ratio, opt.Tier)
						matched = true
						break
					}
				}
				if !matched {
					currSizeDisplay = s
				}
			} else if r, ok := payloadMap["aspect_ratio"].(string); ok && r != "" {
				currSizeDisplay = fmt.Sprintf("Ratio: %s", r)
			}
		}
		sizeBadge = fmt.Sprintf(" | Size: %s [Press Alt+A to change]", styles.MetricValueStyle.Render(currSizeDisplay))
	}

	info := fmt.Sprintf(
		"Base URL: %s | API Type: %s | %s%s",
		m.Record.BaseURL,
		styles.BadgeSuccessStyle.Render(m.Record.APIType),
		styles.MetricValueStyle.Render(modelBadge),
		sizeBadge,
	)
	sb.WriteString(styles.CardStyle.Render(info) + "\n")

	if m.Record.APIType == api.APITypeOpenAIImages {
		sb.WriteString(styles.HelpStyle.Render("📁 Output directory: ./generated_images/ | Press [Ctrl+O] to open last generated image in system viewer") + "\n")
	} else {
		// Reasoning Effort Switcher for Chat models
		sb.WriteString(styles.MetricLabelStyle.Render("Reasoning Effort Shortcuts: ") + " ")
		efforts := []string{db.ReasoningEffortNone, db.ReasoningEffortLow, db.ReasoningEffortHigh, db.ReasoningEffortMax}
		for i, eff := range efforts {
			num := i + 1
			if eff == m.ReasoningEffort {
				sb.WriteString(styles.BadgeAccentStyle.Render(fmt.Sprintf("[%d] %s", num, eff)) + " ")
			} else {
				sb.WriteString(styles.BadgeStyle.Render(fmt.Sprintf("[%d] %s", num, eff)) + " ")
			}
		}
	}
	sb.WriteString("\n\n")

	// Calculate Pane Dimensions
	totalWidth := m.Width - 4
	halfWidth := (totalWidth - 2) / 2
	if halfWidth < 35 {
		halfWidth = 40
	}
	paneHeight := m.Height - 13
	if paneHeight < 12 {
		paneHeight = 16
	}

	// 1. Render Left Pane (Request Payload Editor OR Size / Model Picker Overlay)
	var leftBorderColor lipgloss.Color
	var leftTitle string
	var leftContent string

	if m.SelectingAspectRatio || m.SelectingSize {
		totalSizes := len(SupportedImageSizes)
		leftBorderColor = styles.ColorAccent
		leftTitle = fmt.Sprintf("📐 Select Image Size ([%d/%d] - ↑/↓ to move, Enter to apply, Esc to cancel)", m.AspectRatioIndex+1, totalSizes)

		var sizeContentBuilder strings.Builder
		sizeContentBuilder.WriteString(styles.SubtitleStyle.Render(leftTitle) + "\n\n")

		maxVisible := paneHeight - 4
		if maxVisible < 10 {
			maxVisible = 10
		}

		startIdx := m.AspectRatioIndex - maxVisible/2
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := startIdx + maxVisible
		if endIdx > totalSizes {
			endIdx = totalSizes
			startIdx = endIdx - maxVisible
			if startIdx < 0 {
				startIdx = 0
			}
		}

		if startIdx > 0 {
			sizeContentBuilder.WriteString(styles.HelpStyle.Render(fmt.Sprintf("  ▲ ... %d smaller sizes above ...", startIdx)) + "\n")
		}

		for i := startIdx; i < endIdx; i++ {
			opt := SupportedImageSizes[i]
			prefix := "  "
			if i == m.AspectRatioIndex {
				prefix = "👉"
			}
			sizeContentBuilder.WriteString(fmt.Sprintf("%s %-10s %s\n", prefix, styles.BadgeAccentStyle.Render(opt.Size), styles.MetricValueStyle.Render(opt.Desc)))
		}

		if endIdx < totalSizes {
			sizeContentBuilder.WriteString(styles.HelpStyle.Render(fmt.Sprintf("  ▼ ... %d larger sizes below ...", totalSizes-endIdx)) + "\n")
		}

		leftContent = sizeContentBuilder.String()

	} else if m.SelectingModel {
		totalModels := len(m.DiscoveredModels)
		leftBorderColor = styles.ColorAccent
		leftTitle = fmt.Sprintf("🤖 Select Model (Model list length: %d, [%d/%d] - ↑/↓ to move, Enter to apply, Esc to cancel)", totalModels, m.ModelIndex+1, totalModels)

		var modelContentBuilder strings.Builder
		modelContentBuilder.WriteString(styles.SubtitleStyle.Render(leftTitle) + "\n\n")

		maxVisible := paneHeight - 4
		if maxVisible < 20 {
			maxVisible = 20
		}

		startIdx := m.ModelIndex - maxVisible/2
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := startIdx + maxVisible
		if endIdx > totalModels {
			endIdx = totalModels
			startIdx = endIdx - maxVisible
			if startIdx < 0 {
				startIdx = 0
			}
		}

		if startIdx > 0 {
			modelContentBuilder.WriteString(fmt.Sprintf("  ▲ %d models above...\n", startIdx))
		}

		for i := startIdx; i < endIdx; i++ {
			prefix := "  "
			if i == m.ModelIndex {
				prefix = "👉"
			}
			modelContentBuilder.WriteString(fmt.Sprintf("%s %s\n", prefix, styles.MetricValueStyle.Render(m.DiscoveredModels[i])))
		}

		if endIdx < totalModels {
			modelContentBuilder.WriteString(fmt.Sprintf("  ▼ %d models below...\n", totalModels-endIdx))
		}

		leftContent = modelContentBuilder.String()

	} else {
		if m.ActivePane == PaneRequest {
			leftBorderColor = styles.ColorSecondary
			leftTitle = "📝 Request Payload JSON (Active Focus)"
		} else {
			leftBorderColor = styles.ColorMuted
			leftTitle = "📝 Request Payload JSON"
		}
		leftContent = fmt.Sprintf("%s\n%s", styles.SubtitleStyle.Render(leftTitle), m.Textarea.View())
	}

	leftBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(leftBorderColor).
		Padding(0, 1).
		Width(halfWidth).
		Height(paneHeight)

	leftPaneView := leftBoxStyle.Render(leftContent)

	// 2. Render Right Pane (Response Panel)
	var rightBorderColor lipgloss.Color
	var rightTitle string
	if m.IsStreamMode {
		if m.ActivePane == PaneResponse {
			rightBorderColor = styles.ColorSecondary
			rightTitle = "📊 Response Stream (Active Scroll)"
		} else {
			rightBorderColor = styles.ColorMuted
			rightTitle = "📊 Response Stream"
		}
	} else {
		if m.ActivePane == PaneResponse {
			rightBorderColor = styles.ColorSecondary
			rightTitle = "📊 Response JSON (Active Scroll)"
		} else {
			rightBorderColor = styles.ColorMuted
			rightTitle = "📊 Response JSON"
		}
	}

	rightBoxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(rightBorderColor).
		Padding(0, 1).
		Width(halfWidth).
		Height(paneHeight)

	var rightContentBuilder strings.Builder
	rightContentBuilder.WriteString(styles.SubtitleStyle.Render(rightTitle) + "\n")

	if m.CopyStatusMsg != "" {
		rightContentBuilder.WriteString(styles.BadgeSuccessStyle.Render(m.CopyStatusMsg) + "\n")
	}

	if m.IsExecuting {
		if m.IsStreamMode {
			statusText := "HTTP ..."
			if m.StreamStatusCode > 0 {
				statusText = fmt.Sprintf("HTTP %d", m.StreamStatusCode)
			}
			metrics := fmt.Sprintf(
				"%s %s | Latency: %s",
				m.Spinner.View(),
				styles.BadgeSuccessStyle.Render(statusText),
				styles.MetricValueStyle.Render(formatLatency(m.StreamLatency)),
			)
			rightContentBuilder.WriteString(metrics + "\n\n")
			if m.StreamError != "" {
				streamErrSummary := m.StreamError
				if idx := strings.IndexAny(streamErrSummary, "\n\r"); idx > 0 {
					streamErrSummary = streamErrSummary[:idx]
				}
				if len(streamErrSummary) > 80 {
					streamErrSummary = streamErrSummary[:80] + "..."
				}
				rightContentBuilder.WriteString(styles.ErrorStyle.Render("Error: "+streamErrSummary) + "\n")
			}
			rightContentBuilder.WriteString(m.Viewport.View())
		} else {
			rightContentBuilder.WriteString("\n" + m.Spinner.View() + " Sending request payload...\n")
		}
	} else if m.LastResult != nil {
		var statusStyle lipgloss.Style
		statusText := fmt.Sprintf("HTTP %d", m.LastResult.StatusCode)
		if m.LastResult.StatusCode == 200 && m.LastResult.Error == "" {
			statusStyle = styles.BadgeSuccessStyle
		} else {
			statusStyle = styles.BadgeAccentStyle
			if m.LastResult.Error != "" {
				statusText = fmt.Sprintf("HTTP %d (API Error)", m.LastResult.StatusCode)
			}
		}

		metrics := fmt.Sprintf(
			"%s | Latency: %s | tps: %s",
			statusStyle.Render(statusText),
			styles.MetricValueStyle.Render(formatLatency(m.LastResult.Latency)),
			formatTPS(m.LastResult.TotalTokens, m.LastResult.Latency),
		)
		rightContentBuilder.WriteString(metrics + "\n\n")
		if m.LastResult.Error != "" {
			errSummary := m.LastResult.Error
			// Truncate to first line for display above viewport
			if idx := strings.IndexAny(errSummary, "\n\r"); idx > 0 {
				errSummary = errSummary[:idx]
			}
			if len(errSummary) > 80 {
				errSummary = errSummary[:80] + "..."
			}
			rightContentBuilder.WriteString(styles.ErrorStyle.Render("Error: "+errSummary) + "\n")
		}
		rightContentBuilder.WriteString(m.Viewport.View())
	} else {
		rightContentBuilder.WriteString(lipgloss.NewStyle().Foreground(styles.ColorMuted).Render("\nPress [Ctrl+S] to send payload and view response here."))
	}

	rightPaneView := rightBoxStyle.Render(rightContentBuilder.String())

	// Join Columns Side-by-Side
	splitView := lipgloss.JoinHorizontal(lipgloss.Top, leftPaneView, " ", rightPaneView)
	sb.WriteString(splitView + "\n")

	// Help Footer
	var helpKey string
	if m.Record.APIType == api.APITypeOpenAIImages {
		helpKey = styles.HelpStyle.Render(
			"[Ctrl+S] Generate  [Ctrl+O] Open Image  [Alt+A] Aspect Ratio  [Alt+M] Model  [Ctrl+Y] Copy Req  [Ctrl+U] Copy Resp  [PgUp/PgDn] Scroll  [Tab] Pane  [Esc] Manager",
		)
	} else {
		helpKey = styles.HelpStyle.Render(
			"[Ctrl+S] Send  [Ctrl+Y] Copy Req  [Ctrl+U] Copy Resp  [PgUp/PgDn] Scroll  [Alt+M] Model  [Alt+S] Stream  [Tab] Pane  [Alt+1~4] Reasoning  [Esc] Manager",
		)
	}
	sb.WriteString(helpKey)

	return sb.String()
}
