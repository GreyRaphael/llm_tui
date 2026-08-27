package views

import (
	"fmt"
	"strings"

	"llm_tui/internal/api"
	"llm_tui/internal/db"
	"llm_tui/internal/tui/styles"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type ProbeStep int

const (
	StepInputCredentials ProbeStep = iota
	StepFetchingModels
	StepSelectModel
	StepProbing
	StepSelectAPITypeAndName
)

type ProbeModel struct {
	DB               *db.DB
	Step             ProbeStep
	Width            int
	Height           int
	BaseURLInput     textinput.Model
	APIKeyInput      textinput.Model
	NameInput        textinput.Model
	ModelInput       textinput.Model
	FocusIndex       int
	Spinner          spinner.Model
	ProbeResult      *api.ProbeResult
	AvailableAPITypes []string
	SelectedAPIType  string
	SelectedModel    string
	DiscoveredModels []string
	ModelCursor      int
	APITypeCursor    int
	StatusMsg        string
	IsError          bool
	AutofilledKey    bool
	APIKeyEdited     bool
}

type modelsFetchedMsg struct {
	models []string
	err    error
}

type probeFinishedMsg struct {
	result *api.ProbeResult
	err    error
}

func NewProbeModel(database *db.DB) ProbeModel {
	baseUrlIn := textinput.New()
	baseUrlIn.Placeholder = "https://api.openai.com or https://api.deepseek.com"
	baseUrlIn.Focus()
	baseUrlIn.CharLimit = 256
	baseUrlIn.Width = 50

	apiKeyIn := textinput.New()
	apiKeyIn.Placeholder = "sk-... (optional, leave empty for no auth)"
	apiKeyIn.EchoMode = textinput.EchoPassword
	apiKeyIn.EchoCharacter = '•'
	apiKeyIn.CharLimit = 256
	apiKeyIn.Width = 50

	nameIn := textinput.New()
	nameIn.Placeholder = "Provider Alias (e.g., DeepSeek Official, OpenAI o3)"
	nameIn.CharLimit = 64
	nameIn.Width = 50

	modelIn := textinput.New()
	modelIn.Placeholder = "Custom model string (e.g. qwq-32b, gpt-4o)"
	modelIn.CharLimit = 64
	modelIn.Width = 50

	s := spinner.New()
	s.Spinner = spinner.Dot

	return ProbeModel{
		DB:           database,
		Step:         StepInputCredentials,
		BaseURLInput: baseUrlIn,
		APIKeyInput:  apiKeyIn,
		NameInput:    nameIn,
		ModelInput:   modelIn,
		FocusIndex:   0,
		Spinner:      s,
	}
}

func (m *ProbeModel) Resize(w, h int) {
	m.Width = w
	m.Height = h
	inputWidth := w - 10
	if inputWidth < 50 {
		inputWidth = 50
	}
	m.BaseURLInput.Width = inputWidth
	m.APIKeyInput.Width = inputWidth
	m.NameInput.Width = inputWidth
	m.ModelInput.Width = inputWidth
}

func (m ProbeModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m ProbeModel) Update(msg tea.Msg) (ProbeModel, tea.Cmd, string) {
	var action string

	switch msg := msg.(type) {
	case modelsFetchedMsg:
		m.DiscoveredModels = msg.models
		m.Step = StepSelectModel
		if len(m.DiscoveredModels) > 0 {
			m.SelectedModel = m.DiscoveredModels[0]
			m.ModelInput.SetValue(m.SelectedModel)
			m.StatusMsg = fmt.Sprintf("Discovered %d models via /models (Model list length: %d). Pick or specify one below.", len(m.DiscoveredModels), len(m.DiscoveredModels))
		} else {
			// No models could be discovered; require the user to enter a model id
			// manually instead of silently falling back to a default like gpt-4o.
			m.SelectedModel = ""
			m.ModelInput.SetValue("")
			m.ModelCursor = 0
			m.StatusMsg = "Could not fetch models automatically (Model list length: 0). Please enter the exact model id below."
		}
		m.ModelInput.Focus()
		m.IsError = false
		return m, textinput.Blink, ""

	case probeFinishedMsg:
		if msg.err != nil {
			m.StatusMsg = fmt.Sprintf("Probing error: %v", msg.err)
			m.IsError = true
			m.Step = StepSelectModel
			return m, nil, ""
		}
		m.ProbeResult = msg.result
		m.AvailableAPITypes = buildSelectableAPITypes(msg.result, m.SelectedModel)
		m.APITypeCursor = 0
		if len(m.AvailableAPITypes) > 0 {
			m.SelectedAPIType = m.AvailableAPITypes[0]
		}
		m.Step = StepSelectAPITypeAndName
		m.NameInput.SetValue(m.SelectedModel)
		m.NameInput.Focus()
		return m, textinput.Blink, ""

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			action = "back_to_manager"
			return m, nil, action

		case "tab", "shift+tab":
			if m.Step == StepInputCredentials {
				if m.FocusIndex == 0 {
					m.FocusIndex = 1
					m.BaseURLInput.Blur()
					m.APIKeyInput.Focus()
				} else {
					m.FocusIndex = 0
					m.APIKeyInput.Blur()
					m.BaseURLInput.Focus()
				}
				m.checkAutofillAPIKey()
			}

		case "enter":
			switch m.Step {
			case StepInputCredentials:
				m.checkAutofillAPIKey()
				if strings.TrimSpace(m.BaseURLInput.Value()) == "" {
					m.StatusMsg = "Please enter a Base URL"
					m.IsError = true
					return m, nil, ""
				}
				m.Step = StepFetchingModels
				m.StatusMsg = ""
				m.IsError = false
				return m, tea.Batch(m.Spinner.Tick, m.runFetchModelsCmd()), ""

			case StepSelectModel:
				customModel := strings.TrimSpace(m.ModelInput.Value())
				if customModel != "" {
					m.SelectedModel = customModel
				} else if len(m.DiscoveredModels) > 0 && m.ModelCursor < len(m.DiscoveredModels) {
					m.SelectedModel = m.DiscoveredModels[m.ModelCursor]
				}

				if m.SelectedModel == "" {
					m.StatusMsg = "Please enter or select a model string"
					m.IsError = true
					return m, nil, ""
				}

				// Direct image model routing: if model contains image indicators, skip network probing entirely
				if api.IsImageModel(m.SelectedModel) {
					m.Step = StepSelectAPITypeAndName
					m.ProbeResult = &api.ProbeResult{
						BaseURL:           m.BaseURLInput.Value(),
						APIKey:            m.APIKeyInput.Value(),
						SupportedAPITypes: []string{api.APITypeOpenAIImages, api.APITypeOpenAIChat, api.APITypeOpenAIResponses, api.APITypeAnthropic},
						EndpointDetails: map[string]string{
							api.APITypeOpenAIImages:    "Direct routing (Probe skipped to preserve quota)",
							api.APITypeOpenAIChat:      "Available",
							api.APITypeOpenAIResponses: "Available",
							api.APITypeAnthropic:       "Available",
						},
					}
					m.AvailableAPITypes = []string{api.APITypeOpenAIImages, api.APITypeOpenAIChat, api.APITypeOpenAIResponses, api.APITypeAnthropic}
					m.APITypeCursor = 0
					m.SelectedAPIType = api.APITypeOpenAIImages
					m.NameInput.SetValue(m.SelectedModel)
					m.NameInput.Focus()
					m.StatusMsg = "⚡ Image model detected — probe skipped to preserve quota. Press Enter to save."
					m.IsError = false
					return m, textinput.Blink, ""
				}

				m.Step = StepProbing
				m.StatusMsg = ""
				m.IsError = false
				return m, tea.Batch(m.Spinner.Tick, m.runProbeCmd()), ""

			case StepSelectAPITypeAndName:
				types := m.ensureAvailableAPITypes()
				if len(types) > 0 && m.APITypeCursor < len(types) {
					m.SelectedAPIType = types[m.APITypeCursor]
				}
				aliasName := strings.TrimSpace(m.NameInput.Value())
				if aliasName == "" {
					aliasName = m.SelectedModel
				}

				rec := &db.ProviderRecord{
					Name:            aliasName,
					BaseURL:         m.BaseURLInput.Value(),
					APIKey:          m.APIKeyInput.Value(),
					APIType:         m.SelectedAPIType,
					Model:           m.SelectedModel,
					ReasoningEffort: db.ReasoningEffortNone,
				}

				err := m.DB.CreateRecord(rec)
				if err != nil {
					m.StatusMsg = fmt.Sprintf("Failed to save record: %v", err)
					m.IsError = true
					return m, nil, ""
				}
				action = "record_created"
				return m, nil, action
			}

		case "up", "k":
			if m.Step == StepSelectModel && len(m.DiscoveredModels) > 0 {
				if m.ModelCursor > 0 {
					m.ModelCursor--
					m.SelectedModel = m.DiscoveredModels[m.ModelCursor]
					m.ModelInput.SetValue(m.SelectedModel)
				}
			} else if m.Step == StepSelectAPITypeAndName {
				types := m.ensureAvailableAPITypes()
				if len(types) > 0 && m.APITypeCursor > 0 {
					m.selectAPIType(m.APITypeCursor - 1)
				}
			}

		case "down", "j":
			if m.Step == StepSelectModel && len(m.DiscoveredModels) > 0 {
				if m.ModelCursor < len(m.DiscoveredModels)-1 {
					m.ModelCursor++
					m.SelectedModel = m.DiscoveredModels[m.ModelCursor]
					m.ModelInput.SetValue(m.SelectedModel)
				}
			} else if m.Step == StepSelectAPITypeAndName {
				types := m.ensureAvailableAPITypes()
				if len(types) > 0 && m.APITypeCursor < len(types)-1 {
					m.selectAPIType(m.APITypeCursor + 1)
				}
			}
		}
	}

	var cmd tea.Cmd
	switch m.Step {
	case StepInputCredentials:
		if m.FocusIndex == 0 {
			m.BaseURLInput, cmd = m.BaseURLInput.Update(msg)
			m.checkAutofillAPIKey()
		} else {
			m.APIKeyInput, cmd = m.APIKeyInput.Update(msg)
			// Any keystroke in the API key field is a manual override; never let
			// the autofill silently clobber it afterwards.
			if _, ok := msg.(tea.KeyMsg); ok {
				m.APIKeyEdited = true
				if m.AutofilledKey {
					m.AutofilledKey = false
					m.StatusMsg = ""
				}
			}
		}
	case StepFetchingModels, StepProbing:
		m.Spinner, cmd = m.Spinner.Update(msg)
	case StepSelectModel:
		m.ModelInput, cmd = m.ModelInput.Update(msg)
	case StepSelectAPITypeAndName:
		m.NameInput, cmd = m.NameInput.Update(msg)
	}

	return m, cmd, action
}

func (m *ProbeModel) ensureAvailableAPITypes() []string {
	if len(m.AvailableAPITypes) == 0 {
		m.AvailableAPITypes = buildSelectableAPITypes(m.ProbeResult, m.SelectedModel)
	}
	return m.AvailableAPITypes
}

// selectAPIType moves the API type cursor and keeps the auto-generated alias in
// sync with the selected type, without overwriting a user-customized name.
func (m *ProbeModel) selectAPIType(cursor int) {
	types := m.ensureAvailableAPITypes()
	if len(types) == 0 {
		return
	}
	if cursor < 0 {
		cursor = 0
	} else if cursor >= len(types) {
		cursor = len(types) - 1
	}

	oldType := m.SelectedAPIType
	m.APITypeCursor = cursor
	m.SelectedAPIType = types[cursor]

	if oldType == m.SelectedAPIType || m.SelectedModel == "" {
		return
	}
	oldDefault := fmt.Sprintf("%s (%s)", m.SelectedModel, oldType)
	if m.NameInput.Value() == oldDefault || m.NameInput.Value() == m.SelectedModel {
		m.NameInput.SetValue(m.SelectedModel)
	}
}

func buildSelectableAPITypes(result *api.ProbeResult, modelName string) []string {
	var ordered []string
	seen := make(map[string]bool)

	add := func(t string) {
		if !seen[t] {
			seen[t] = true
			ordered = append(ordered, t)
		}
	}

	if api.IsImageModel(modelName) {
		add(api.APITypeOpenAIImages)
	}

	if result != nil {
		for _, t := range result.SupportedAPITypes {
			add(t)
		}
	}

	add(api.APITypeOpenAIChat)
	add(api.APITypeOpenAIResponses)
	add(api.APITypeOpenAIImages)
	add(api.APITypeAnthropic)

	return ordered
}

func (m *ProbeModel) checkAutofillAPIKey() {
	if m.Step != StepInputCredentials {
		return
	}
	baseURL := strings.TrimSpace(m.BaseURLInput.Value())
	if baseURL == "" {
		return
	}
	// Once the user has typed anything into the API key field, respect their
	// entry and never silently override it with a saved key.
	if m.APIKeyEdited {
		return
	}

	existingKey := m.DB.GetAPIKeyByBaseURL(baseURL)
	if existingKey == "" {
		// The Base URL changed away from any saved record: don't leave a stale
		// auto-filled key pointing at a different endpoint.
		if m.AutofilledKey {
			m.APIKeyInput.SetValue("")
			m.AutofilledKey = false
			m.StatusMsg = ""
		}
		return
	}
	if !m.AutofilledKey || m.APIKeyInput.Value() == "" {
		m.APIKeyInput.SetValue(existingKey)
		m.AutofilledKey = true
		m.StatusMsg = "🔑 Auto-filled API Key from existing records (press Tab to modify or keep)."
		m.IsError = false
	}
}

func (m ProbeModel) runFetchModelsCmd() tea.Cmd {
	baseURL := m.BaseURLInput.Value()
	apiKey := m.APIKeyInput.Value()
	return func() tea.Msg {
		models, err := api.FetchModels(baseURL, apiKey)
		return modelsFetchedMsg{models: models, err: err}
	}
}

func (m ProbeModel) runProbeCmd() tea.Cmd {
	baseURL := m.BaseURLInput.Value()
	apiKey := m.APIKeyInput.Value()
	selectedModel := m.SelectedModel
	return func() tea.Msg {
		res, err := api.ProbeProviderWithModel(baseURL, apiKey, selectedModel)
		return probeFinishedMsg{result: res, err: err}
	}
}

func (m ProbeModel) View() string {
	var sb strings.Builder

	header := styles.HeaderStyle.Render("🔍 Provider Setup Wizard")
	sb.WriteString(header + "\n\n")

	if m.StatusMsg != "" {
		if m.IsError {
			sb.WriteString(styles.ErrorStyle.Render("❌ "+m.StatusMsg) + "\n\n")
		} else {
			sb.WriteString(styles.SubtitleStyle.Render("ℹ️  "+m.StatusMsg) + "\n\n")
		}
	}

	switch m.Step {
	case StepInputCredentials:
		sb.WriteString(styles.SubtitleStyle.Render("1. Enter Provider Connection Details") + "\n\n")
		sb.WriteString("Base URL:\n" + m.BaseURLInput.View() + "\n\n")
		sb.WriteString("API Key:\n" + m.APIKeyInput.View() + "\n\n")
		sb.WriteString(styles.HelpStyle.Render("[Tab] Switch Field  [Enter] Fetch Available Models  [Esc] Cancel"))

	case StepFetchingModels:
		sb.WriteString(m.Spinner.View() + " Querying /models list from provider endpoint...\n")

	case StepSelectModel:
		sb.WriteString(styles.SubtitleStyle.Render("2. Select or Specify Target Model") + "\n\n")
		if len(m.DiscoveredModels) > 0 {
			totalModels := len(m.DiscoveredModels)
			sb.WriteString(styles.MetricLabelStyle.Render(fmt.Sprintf("Discovered Models List (Model list length: %d, [%d/%d], Use ↑/↓ to navigate):", totalModels, m.ModelCursor+1, totalModels)) + "\n")
			maxVisible := 20

			startIdx := m.ModelCursor - maxVisible/2
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
				sb.WriteString(fmt.Sprintf("   ▲ %d models above...\n", startIdx))
			}

			for i := startIdx; i < endIdx; i++ {
				prefix := "  "
				if i == m.ModelCursor {
					prefix = "👉"
				}
				sb.WriteString(fmt.Sprintf("%s %s\n", prefix, styles.MetricValueStyle.Render(m.DiscoveredModels[i])))
			}

			if endIdx < totalModels {
				sb.WriteString(fmt.Sprintf("   ▼ %d models below...\n", totalModels-endIdx))
			}
			sb.WriteString("\n")
		} else {
			sb.WriteString(styles.HelpStyle.Render("No models discovered via /models endpoint (Model list length: 0).") + "\n\n")
		}
		sb.WriteString("Target Model Name:\n" + m.ModelInput.View() + "\n\n")
		sb.WriteString(styles.HelpStyle.Render("[↑/↓] Quick Pick Discovered Model  [Enter] Start API Probing  [Esc] Cancel"))

	case StepProbing:
		sb.WriteString(m.Spinner.View() + fmt.Sprintf(" Probing API endpoints with selected model '%s'...\n", m.SelectedModel))

	case StepSelectAPITypeAndName:
		sb.WriteString(styles.SubtitleStyle.Render("3. Select Supported API Type & Name Record") + "\n\n")
		sb.WriteString(styles.MetricLabelStyle.Render("Available API Capabilities (Use ↑/↓ to choose):") + "\n")
		types := m.ensureAvailableAPITypes()
		for i, apiType := range types {
			prefix := "  "
			if i == m.APITypeCursor {
				prefix = "👉"
			}
			detail := ""
			if m.ProbeResult != nil && m.ProbeResult.EndpointDetails != nil {
				detail = m.ProbeResult.EndpointDetails[apiType]
			}
			if detail == "" {
				detail = "Available"
			}
			badgeStyle := styles.BadgeStyle
			if m.ProbeResult != nil {
				for _, supportedType := range m.ProbeResult.SupportedAPITypes {
					if supportedType == apiType {
						badgeStyle = styles.BadgeSuccessStyle
						break
					}
				}
			}
			sb.WriteString(fmt.Sprintf("%s %s (%s)\n", prefix, badgeStyle.Render(apiType), detail))
		}

		sb.WriteString("\nRecord Alias Name:\n" + m.NameInput.View() + "\n\n")
		sb.WriteString(styles.HelpStyle.Render("[↑/↓] Change Selected API Type  [Enter] Save Record to SQLite  [Esc] Cancel"))
	}

	return sb.String()
}
