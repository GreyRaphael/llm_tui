package views

import (
	"cmp"
	"fmt"
	"strings"

	"llm_tui/internal/api"
	"llm_tui/internal/db"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m ProbeModel) Update(msg tea.Msg) (ProbeModel, tea.Cmd, Action) {
	switch msg := msg.(type) {
	case modelsFetchedMsg:
		return m.handleModelsFetched(msg)
	case probeFinishedMsg:
		return m.handleProbeFinished(msg)
	case tea.KeyMsg:
		return m.handleProbeKey(msg)
	default:
		return m.updateActiveInput(msg)
	}
}

func (m ProbeModel) handleModelsFetched(msg modelsFetchedMsg) (ProbeModel, tea.Cmd, Action) {
	if !finishTrackedRequest(&m.cancelRequest, &m.requestID, msg.id) {
		return m, nil, ActionNone
	}
	m.DiscoveredModels = msg.models
	m.ModelCursor = 0
	m.Step = StepSelectModel
	if len(m.DiscoveredModels) > 0 {
		m.SelectedModel = m.DiscoveredModels[0]
		m.ModelInput.SetValue(m.SelectedModel)
		m.StatusMsg = fmt.Sprintf("Discovered %d models via /models (Model list length: %d). Pick or specify one below.", len(m.DiscoveredModels), len(m.DiscoveredModels))
		m.IsError = false
	} else {
		m.SelectedModel = ""
		m.ModelInput.SetValue("")
		m.StatusMsg = "Could not fetch models automatically (Model list length: 0). Please enter the exact model id below."
		if msg.err != nil {
			m.StatusMsg = fmt.Sprintf("Could not fetch models automatically (Model list length: 0): %v. Please enter the exact model id below.", msg.err)
		}
		m.IsError = msg.err != nil
	}
	m.ModelInput.Focus()
	return m, textinput.Blink, ActionNone
}

func (m ProbeModel) handleProbeFinished(msg probeFinishedMsg) (ProbeModel, tea.Cmd, Action) {
	if !finishTrackedRequest(&m.cancelRequest, &m.requestID, msg.id) {
		return m, nil, ActionNone
	}
	if msg.err != nil {
		m.StatusMsg = fmt.Sprintf("Probing error: %v", msg.err)
		m.IsError = true
		m.Step = StepSelectModel
		return m, nil, ActionNone
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
	return m, textinput.Blink, ActionNone
}

func (m ProbeModel) handleProbeKey(msg tea.KeyMsg) (ProbeModel, tea.Cmd, Action) {
	switch msg.String() {
	case "esc":
		m.CancelRequest()
		return m, nil, ActionBackToManager
	case "tab", "shift+tab":
		m.switchCredentialFocus()
	case "enter":
		return m.handleProbeEnter()
	case "up":
		if m.moveProbeSelection(-1) {
			return m, nil, ActionNone
		}
	case "down":
		if m.moveProbeSelection(1) {
			return m, nil, ActionNone
		}
	}
	return m.updateActiveInput(msg)
}

func (m *ProbeModel) switchCredentialFocus() {
	if m.Step != StepInputCredentials {
		return
	}
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

func (m ProbeModel) handleProbeEnter() (ProbeModel, tea.Cmd, Action) {
	switch m.Step {
	case StepInputCredentials:
		return m.startModelFetch()
	case StepSelectModel:
		return m.startModelProbe()
	case StepSelectAPITypeAndName:
		return m.saveProviderRecord()
	default:
		return m, nil, ActionNone
	}
}

func (m ProbeModel) startModelFetch() (ProbeModel, tea.Cmd, Action) {
	m.checkAutofillAPIKey()
	if strings.TrimSpace(m.BaseURLInput.Value()) == "" {
		m.StatusMsg = "Please enter a Base URL"
		m.IsError = true
		return m, nil, ActionNone
	}
	m.Step = StepFetchingModels
	m.StatusMsg = ""
	m.IsError = false
	ctx, id := beginTrackedRequest(&m.cancelRequest, &m.requestID)
	return m, tea.Batch(m.Spinner.Tick, m.runFetchModelsCmd(ctx, id)), ActionNone
}

func (m ProbeModel) startModelProbe() (ProbeModel, tea.Cmd, Action) {
	customModel := strings.TrimSpace(m.ModelInput.Value())
	if customModel != "" {
		m.SelectedModel = customModel
	} else if len(m.DiscoveredModels) > 0 && m.ModelCursor >= 0 && m.ModelCursor < len(m.DiscoveredModels) {
		m.SelectedModel = m.DiscoveredModels[m.ModelCursor]
	}
	if m.SelectedModel == "" {
		m.StatusMsg = "Please enter or select a model string"
		m.IsError = true
		return m, nil, ActionNone
	}
	if api.IsImageModel(m.SelectedModel) {
		return m.selectImageModel()
	}
	m.Step = StepProbing
	m.StatusMsg = ""
	m.IsError = false
	ctx, id := beginTrackedRequest(&m.cancelRequest, &m.requestID)
	return m, tea.Batch(m.Spinner.Tick, m.runProbeCmd(ctx, id)), ActionNone
}

func (m ProbeModel) selectImageModel() (ProbeModel, tea.Cmd, Action) {
	m.Step = StepSelectAPITypeAndName
	m.ProbeResult = &api.ProbeResult{
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
	return m, textinput.Blink, ActionNone
}

func (m ProbeModel) saveProviderRecord() (ProbeModel, tea.Cmd, Action) {
	types := m.ensureAvailableAPITypes()
	if len(types) > 0 && m.APITypeCursor >= 0 && m.APITypeCursor < len(types) {
		m.SelectedAPIType = types[m.APITypeCursor]
	}
	alias := strings.TrimSpace(m.NameInput.Value())
	if alias == "" {
		alias = m.SelectedModel
	}
	rawURL := m.BaseURLInput.Value()
	baseURL := cmp.Or(api.NormalizeURL(rawURL), strings.TrimSpace(rawURL))
	rec := &db.ProviderRecord{
		Name:            alias,
		BaseURL:         baseURL,
		APIKey:          m.APIKeyInput.Value(),
		APIType:         m.SelectedAPIType,
		Model:           m.SelectedModel,
		ReasoningEffort: api.ReasoningEffortNone,
	}
	if err := m.DB.CreateRecord(rec); err != nil {
		m.StatusMsg = fmt.Sprintf("Failed to save record: %v", err)
		m.IsError = true
		return m, nil, ActionNone
	}
	return m, nil, ActionRecordCreated
}

// moveProbeSelection moves the active list cursor and reports whether the key
// was consumed. When it returns false the key must still reach the focused text
// input (e.g. arrow keys while editing the Base URL).
func (m *ProbeModel) moveProbeSelection(delta int) bool {
	if m.Step == StepSelectModel && len(m.DiscoveredModels) > 0 {
		next := m.ModelCursor + delta
		if next >= 0 && next < len(m.DiscoveredModels) {
			m.ModelCursor = next
			m.SelectedModel = m.DiscoveredModels[next]
			m.ModelInput.SetValue(m.SelectedModel)
		}
		return true
	}
	if m.Step == StepSelectAPITypeAndName {
		types := m.ensureAvailableAPITypes()
		if len(types) == 0 {
			return false
		}
		next := m.APITypeCursor + delta
		if next >= 0 && next < len(types) {
			m.selectAPIType(next)
		}
		return true
	}
	return false
}

func (m ProbeModel) updateActiveInput(msg tea.Msg) (ProbeModel, tea.Cmd, Action) {
	var cmd tea.Cmd
	switch m.Step {
	case StepInputCredentials:
		if m.FocusIndex == 0 {
			m.BaseURLInput, cmd = m.BaseURLInput.Update(msg)
			m.checkAutofillAPIKey()
		} else {
			m.APIKeyInput, cmd = m.APIKeyInput.Update(msg)
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
	return m, cmd, ActionNone
}
