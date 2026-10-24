package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"llm_tui/internal/api"
	"llm_tui/internal/db"

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

type TesterModel struct {
	DB                     *db.DB
	Record                 db.ProviderRecord
	ReasoningEffort        string
	Textarea               textarea.Model
	Viewport               viewport.Model
	Spinner                spinner.Model
	ActivePane             ActivePane
	IsExecuting            bool
	IsStreamMode           bool
	SelectingModel         bool
	SelectingAspectRatio   bool
	DiscoveredModels       []string
	ModelIndex             int
	AspectRatioIndex       int
	LastResult             *api.TestResult
	LatestSavedImage       string
	CopyStatusMsg          string
	Width                  int
	Height                 int
	StreamChan             chan api.StreamChunkMsg
	requestID              uint64
	cancelExecution        context.CancelFunc
	modelsRequestID        uint64
	cancelModels           context.CancelFunc
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
	id     uint64
	result *api.TestResult
}

type testerModelsFetchedMsg struct {
	id     uint64
	models []string
	err    error
}

// streamChunkMsg wraps an api.StreamChunkMsg with the id of the request it
// belongs to. Because bubbletea cannot cancel an already-running command goroutine
// (which may be blocked reading an old channel), a stale chunk can be delivered
// after a new request starts; the id lets the model drop it.
type streamChunkMsg struct {
	id  uint64
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

	halfWidth, paneHeight := m.paneDimensions()

	m.Textarea.SetWidth(halfWidth - 4)
	m.Textarea.SetHeight(paneHeight - 2)

	m.Viewport.Width = halfWidth - 4
	m.Viewport.Height = paneHeight - 5

	if m.LastRawViewportContent != "" {
		m.setViewportContent(m.LastRawViewportContent)
	}
}

func (m *TesterModel) Init() tea.Cmd {
	ctx, id := beginTrackedRequest(&m.cancelModels, &m.modelsRequestID)
	return tea.Batch(textarea.Blink, m.runFetchModelsCmd(ctx, id))
}

func (m TesterModel) Update(msg tea.Msg) (TesterModel, tea.Cmd, string) {
	switch msg := msg.(type) {
	case testerModelsFetchedMsg:
		return m.handleModelsFetched(msg)
	case executeFinishedMsg:
		return m.handleExecuteFinished(msg)
	case streamChunkMsg:
		if msg.id == 0 || msg.id != m.requestID {
			return m, nil, ""
		}
		return m.handleStreamChunk(msg.msg)
	case tea.KeyMsg:
		if m.SelectingAspectRatio {
			return m.handleSizeKey(msg)
		}
		if m.SelectingModel {
			return m.handleModelKey(msg)
		}
		return m.handleMainKey(msg)
	default:
		return m.updateActiveComponent(msg)
	}
}

func (m TesterModel) runFetchModelsCmd(ctx context.Context, id uint64) tea.Cmd {
	baseURL := m.Record.BaseURL
	apiKey := m.Record.APIKey
	return func() tea.Msg {
		models, err := api.FetchModels(ctx, baseURL, apiKey)
		return testerModelsFetchedMsg{id: id, models: models, err: err}
	}
}

func (m TesterModel) runExecuteNonStreamCmd(ctx context.Context, id uint64) tea.Cmd {
	baseURL := m.Record.BaseURL
	apiKey := m.Record.APIKey
	apiType := m.Record.APIType
	payload := m.Textarea.Value()

	return func() tea.Msg {
		res := api.ExecuteTestRequest(ctx, baseURL, apiKey, apiType, payload)
		return executeFinishedMsg{id: id, result: res}
	}
}

func (m TesterModel) runExecuteStreamCmd(ctx context.Context, ch chan<- api.StreamChunkMsg) tea.Cmd {
	baseURL := m.Record.BaseURL
	apiKey := m.Record.APIKey
	apiType := m.Record.APIType
	payload := m.Textarea.Value()

	return func() tea.Msg {
		api.ExecuteStreamRequest(ctx, baseURL, apiKey, apiType, payload, ch)
		return nil
	}
}

// CancelRequests releases all HTTP work owned by this screen.
func (m *TesterModel) CancelRequests() {
	cancelTrackedRequest(&m.cancelExecution, &m.requestID)
	cancelTrackedRequest(&m.cancelModels, &m.modelsRequestID)
	m.IsExecuting = false
	m.StreamChan = nil
}

func waitForStreamChunkCmd(id uint64, ch <-chan api.StreamChunkMsg) tea.Cmd {
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
