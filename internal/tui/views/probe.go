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
	SelectedAPIType  string
	SelectedModel    string
	DiscoveredModels []string
	ModelCursor      int
	APITypeCursor    int
	StatusMsg        string
	IsError          bool
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
	apiKeyIn.Placeholder = "sk-..."
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
			m.StatusMsg = fmt.Sprintf("Discovered %d models via /models. Pick or specify one below.", len(m.DiscoveredModels))
		} else {
			m.ModelInput.SetValue("gpt-4o")
			m.StatusMsg = "Could not fetch models automatically. Please specify your target model below."
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
		if len(msg.result.SupportedAPITypes) == 0 {
			m.StatusMsg = "No supported API endpoints detected for this URL, Key & Model."
			m.IsError = true
			m.Step = StepSelectModel
			return m, nil, ""
		}
		m.Step = StepSelectAPITypeAndName
		m.SelectedAPIType = msg.result.SupportedAPITypes[0]
		m.NameInput.SetValue(fmt.Sprintf("%s (%s)", m.SelectedModel, m.SelectedAPIType))
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
			}

		case "enter":
			switch m.Step {
			case StepInputCredentials:
				if strings.TrimSpace(m.BaseURLInput.Value()) == "" || strings.TrimSpace(m.APIKeyInput.Value()) == "" {
					m.StatusMsg = "Please enter both Base URL and API Key"
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

				m.Step = StepProbing
				m.StatusMsg = ""
				m.IsError = false
				return m, tea.Batch(m.Spinner.Tick, m.runProbeCmd()), ""

			case StepSelectAPITypeAndName:
				if len(m.ProbeResult.SupportedAPITypes) > 0 && m.APITypeCursor < len(m.ProbeResult.SupportedAPITypes) {
					m.SelectedAPIType = m.ProbeResult.SupportedAPITypes[m.APITypeCursor]
				}
				aliasName := strings.TrimSpace(m.NameInput.Value())
				if aliasName == "" {
					aliasName = fmt.Sprintf("%s (%s)", m.SelectedModel, m.SelectedAPIType)
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
			} else if m.Step == StepSelectAPITypeAndName && m.ProbeResult != nil {
				if m.APITypeCursor > 0 {
					m.APITypeCursor--
				}
			}

		case "down", "j":
			if m.Step == StepSelectModel && len(m.DiscoveredModels) > 0 {
				if m.ModelCursor < len(m.DiscoveredModels)-1 {
					m.ModelCursor++
					m.SelectedModel = m.DiscoveredModels[m.ModelCursor]
					m.ModelInput.SetValue(m.SelectedModel)
				}
			} else if m.Step == StepSelectAPITypeAndName && m.ProbeResult != nil {
				if m.APITypeCursor < len(m.ProbeResult.SupportedAPITypes)-1 {
					m.APITypeCursor++
				}
			}
		}
	}

	var cmd tea.Cmd
	switch m.Step {
	case StepInputCredentials:
		if m.FocusIndex == 0 {
			m.BaseURLInput, cmd = m.BaseURLInput.Update(msg)
		} else {
			m.APIKeyInput, cmd = m.APIKeyInput.Update(msg)
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
			sb.WriteString(styles.MetricLabelStyle.Render("Discovered Models List (Use ↑/↓ to navigate):") + "\n")
			maxShow := 6
			for i, md := range m.DiscoveredModels {
				if i >= maxShow {
					sb.WriteString(fmt.Sprintf("   ... and %d more models\n", len(m.DiscoveredModels)-maxShow))
					break
				}
				prefix := "  "
				if i == m.ModelCursor {
					prefix = "👉"
				}
				sb.WriteString(fmt.Sprintf("%s %s\n", prefix, styles.MetricValueStyle.Render(md)))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("Target Model Name:\n" + m.ModelInput.View() + "\n\n")
		sb.WriteString(styles.HelpStyle.Render("[↑/↓] Quick Pick Discovered Model  [Enter] Start API Probing  [Esc] Cancel"))

	case StepProbing:
		sb.WriteString(m.Spinner.View() + fmt.Sprintf(" Probing API endpoints with selected model '%s'...\n", m.SelectedModel))

	case StepSelectAPITypeAndName:
		sb.WriteString(styles.SubtitleStyle.Render("3. Select Supported API Type & Name Record") + "\n\n")
		sb.WriteString(styles.MetricLabelStyle.Render("Detected API Capabilities:") + "\n")
		for i, apiType := range m.ProbeResult.SupportedAPITypes {
			prefix := "  "
			if i == m.APITypeCursor {
				prefix = "👉"
			}
			detail := m.ProbeResult.EndpointDetails[apiType]
			sb.WriteString(fmt.Sprintf("%s %s (%s)\n", prefix, styles.BadgeSuccessStyle.Render(apiType), detail))
		}

		sb.WriteString("\nRecord Alias Name:\n" + m.NameInput.View() + "\n\n")
		sb.WriteString(styles.HelpStyle.Render("[↑/↓] Change Selected API Type  [Enter] Save Record to SQLite  [Esc] Cancel"))
	}

	return sb.String()
}
