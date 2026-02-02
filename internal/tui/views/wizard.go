// Package views provides TUI view components for the Dreamteller application.
package views

import (
	"fmt"
	"strings"

	"github.com/azyu/dreamteller/internal/tui/styles"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// WizardStep represents a step in the wizard.
type WizardStep int

const (
	StepProjectName WizardStep = iota
	StepGenre
	StepPremise
	StepWorldSetting
	StepCharacters
	StepPlotOutline
	StepLLMProvider
)

const totalSteps = 7

// Genre represents a story genre option.
type Genre struct {
	Name        string
	Description string
}

// Genres available for selection.
var Genres = []Genre{
	{Name: "fantasy", Description: "Magic, mythical creatures, and otherworldly realms"},
	{Name: "sci-fi", Description: "Technology, space, and future possibilities"},
	{Name: "romance", Description: "Love, relationships, and emotional journeys"},
	{Name: "mystery", Description: "Puzzles, secrets, and revelations"},
	{Name: "thriller", Description: "Suspense, danger, and high stakes"},
	{Name: "horror", Description: "Fear, the supernatural, and the macabre"},
	{Name: "literary", Description: "Character-driven narratives and prose artistry"},
}

// LLMProvider represents an LLM provider option.
type LLMProvider struct {
	Name        string
	Description string
}

// LLMProviders available for selection.
var LLMProviders = []LLMProvider{
	{Name: "openai", Description: "OpenAI GPT models (requires API key)"},
	{Name: "gemini", Description: "Google Gemini models (requires API key)"},
	{Name: "local", Description: "Local LLM via Ollama or compatible API"},
}

// WizardResult holds all collected data from the wizard.
type WizardResult struct {
	ProjectName  string
	Genre        string
	Premise      string
	WorldSetting string
	Characters   string
	PlotOutline  string
	LLMProvider  string
}

// WizardModel implements tea.Model for the setup wizard.
type WizardModel struct {
	currentStep WizardStep
	totalSteps  int
	result      WizardResult
	completed   bool
	cancelled   bool
	err         error

	// Input components
	nameInput        textinput.Model
	premiseInput     textarea.Model
	worldInput       textarea.Model
	charactersInput  textarea.Model
	plotInput        textarea.Model
	genreList        list.Model
	providerList     list.Model

	// Dimensions
	width  int
	height int
	ready  bool
}

// listItem implements list.Item for genre and provider selections.
type listItem struct {
	name        string
	description string
}

func (i listItem) Title() string       { return i.name }
func (i listItem) Description() string { return i.description }
func (i listItem) FilterValue() string { return i.name }

// NewWizard creates a new wizard model.
func NewWizard() *WizardModel {
	// Project name input
	nameInput := textinput.New()
	nameInput.Placeholder = "my-novel"
	nameInput.Focus()
	nameInput.CharLimit = 64
	nameInput.Width = 40
	nameInput.PromptStyle = styles.InputPrompt
	nameInput.TextStyle = styles.InputText

	// Premise textarea
	premiseInput := textarea.New()
	premiseInput.Placeholder = "Describe your story's core premise in a few sentences..."
	premiseInput.SetWidth(60)
	premiseInput.SetHeight(4)
	premiseInput.CharLimit = 1000

	// World setting textarea
	worldInput := textarea.New()
	worldInput.Placeholder = "Describe the world where your story takes place..."
	worldInput.SetWidth(60)
	worldInput.SetHeight(4)
	worldInput.CharLimit = 2000

	// Characters textarea
	charactersInput := textarea.New()
	charactersInput.Placeholder = "Describe your main characters (name, role, traits)..."
	charactersInput.SetWidth(60)
	charactersInput.SetHeight(6)
	charactersInput.CharLimit = 3000

	// Plot outline textarea
	plotInput := textarea.New()
	plotInput.Placeholder = "Outline the main plot points or story beats..."
	plotInput.SetWidth(60)
	plotInput.SetHeight(6)
	plotInput.CharLimit = 3000

	// Genre list
	genreItems := make([]list.Item, len(Genres))
	for i, g := range Genres {
		genreItems[i] = listItem{name: g.Name, description: g.Description}
	}

	genreDelegate := list.NewDefaultDelegate()
	genreDelegate.Styles.SelectedTitle = styles.SelectedItem
	genreDelegate.Styles.NormalTitle = styles.ListItem

	genreList := list.New(genreItems, genreDelegate, 40, 14)
	genreList.Title = "Select a Genre"
	genreList.SetShowStatusBar(false)
	genreList.SetFilteringEnabled(false)
	genreList.SetShowHelp(false)
	genreList.Styles.Title = styles.Title

	// Provider list
	providerItems := make([]list.Item, len(LLMProviders))
	for i, p := range LLMProviders {
		providerItems[i] = listItem{name: p.Name, description: p.Description}
	}

	providerDelegate := list.NewDefaultDelegate()
	providerDelegate.Styles.SelectedTitle = styles.SelectedItem
	providerDelegate.Styles.NormalTitle = styles.ListItem

	providerList := list.New(providerItems, providerDelegate, 50, 10)
	providerList.Title = "Select LLM Provider"
	providerList.SetShowStatusBar(false)
	providerList.SetFilteringEnabled(false)
	providerList.SetShowHelp(false)
	providerList.Styles.Title = styles.Title

	return &WizardModel{
		currentStep:     StepProjectName,
		totalSteps:      totalSteps,
		nameInput:       nameInput,
		premiseInput:    premiseInput,
		worldInput:      worldInput,
		charactersInput: charactersInput,
		plotInput:       plotInput,
		genreList:       genreList,
		providerList:    providerList,
		result: WizardResult{
			Genre:       "fantasy",
			LLMProvider: "openai",
		},
	}
}

// Init initializes the model.
func (m *WizardModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages.
func (m *WizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Update component widths
		contentWidth := min(80, msg.Width-4)
		m.nameInput.Width = contentWidth - 10
		m.premiseInput.SetWidth(contentWidth)
		m.worldInput.SetWidth(contentWidth)
		m.charactersInput.SetWidth(contentWidth)
		m.plotInput.SetWidth(contentWidth)
		m.genreList.SetWidth(contentWidth)
		m.providerList.SetWidth(contentWidth)

		return m, nil

	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	// Update the active component
	var cmd tea.Cmd
	switch m.currentStep {
	case StepProjectName:
		m.nameInput, cmd = m.nameInput.Update(msg)
		cmds = append(cmds, cmd)
	case StepGenre:
		m.genreList, cmd = m.genreList.Update(msg)
		cmds = append(cmds, cmd)
	case StepPremise:
		m.premiseInput, cmd = m.premiseInput.Update(msg)
		cmds = append(cmds, cmd)
	case StepWorldSetting:
		m.worldInput, cmd = m.worldInput.Update(msg)
		cmds = append(cmds, cmd)
	case StepCharacters:
		m.charactersInput, cmd = m.charactersInput.Update(msg)
		cmds = append(cmds, cmd)
	case StepPlotOutline:
		m.plotInput, cmd = m.plotInput.Update(msg)
		cmds = append(cmds, cmd)
	case StepLLMProvider:
		m.providerList, cmd = m.providerList.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// handleKeyMsg processes keyboard input.
func (m *WizardModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.cancelled = true
		return m, tea.Quit

	case tea.KeyEnter:
		if err := m.validateCurrentStep(); err != nil {
			m.err = err
			return m, nil
		}
		m.saveCurrentStep()
		return m.nextStep()

	case tea.KeyTab:
		// Skip with defaults
		m.saveCurrentStep()
		return m.nextStep()

	case tea.KeyBackspace, tea.KeyDelete:
		// Go back only if input is empty (for text inputs) or always for lists
		if m.canGoBack() {
			return m.previousStep()
		}
	}

	return m, nil
}

// canGoBack determines if backspace should navigate back.
func (m *WizardModel) canGoBack() bool {
	if m.currentStep == StepProjectName {
		return false
	}

	switch m.currentStep {
	case StepGenre, StepLLMProvider:
		return true
	case StepPremise:
		return m.premiseInput.Value() == ""
	case StepWorldSetting:
		return m.worldInput.Value() == ""
	case StepCharacters:
		return m.charactersInput.Value() == ""
	case StepPlotOutline:
		return m.plotInput.Value() == ""
	default:
		return m.nameInput.Value() == ""
	}
}

// nextStep advances to the next step.
func (m *WizardModel) nextStep() (tea.Model, tea.Cmd) {
	if m.currentStep >= WizardStep(m.totalSteps-1) {
		m.completed = true
		return m, tea.Quit
	}

	m.currentStep++
	m.err = nil
	return m, m.focusCurrentStep()
}

// previousStep goes back to the previous step.
func (m *WizardModel) previousStep() (tea.Model, tea.Cmd) {
	if m.currentStep > StepProjectName {
		m.currentStep--
		m.err = nil
		return m, m.focusCurrentStep()
	}
	return m, nil
}

// focusCurrentStep focuses the appropriate input for the current step.
func (m *WizardModel) focusCurrentStep() tea.Cmd {
	// Blur all inputs first
	m.nameInput.Blur()
	m.premiseInput.Blur()
	m.worldInput.Blur()
	m.charactersInput.Blur()
	m.plotInput.Blur()

	switch m.currentStep {
	case StepProjectName:
		m.nameInput.Focus()
		return textinput.Blink
	case StepPremise:
		m.premiseInput.Focus()
		return textarea.Blink
	case StepWorldSetting:
		m.worldInput.Focus()
		return textarea.Blink
	case StepCharacters:
		m.charactersInput.Focus()
		return textarea.Blink
	case StepPlotOutline:
		m.plotInput.Focus()
		return textarea.Blink
	}
	return nil
}

// saveCurrentStep saves the current step's value to the result.
func (m *WizardModel) saveCurrentStep() {
	switch m.currentStep {
	case StepProjectName:
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			name = "my-novel"
		}
		m.result.ProjectName = name
	case StepGenre:
		if item, ok := m.genreList.SelectedItem().(listItem); ok {
			m.result.Genre = item.name
		}
	case StepPremise:
		m.result.Premise = strings.TrimSpace(m.premiseInput.Value())
	case StepWorldSetting:
		m.result.WorldSetting = strings.TrimSpace(m.worldInput.Value())
	case StepCharacters:
		m.result.Characters = strings.TrimSpace(m.charactersInput.Value())
	case StepPlotOutline:
		m.result.PlotOutline = strings.TrimSpace(m.plotInput.Value())
	case StepLLMProvider:
		if item, ok := m.providerList.SelectedItem().(listItem); ok {
			m.result.LLMProvider = item.name
		}
	}
}

// validateCurrentStep validates the current step's input.
func (m *WizardModel) validateCurrentStep() error {
	switch m.currentStep {
	case StepProjectName:
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			return nil // Will use default
		}
		if strings.ContainsAny(name, "/\\:*?\"<>|") {
			return fmt.Errorf("project name contains invalid characters")
		}
		if len(name) > 64 {
			return fmt.Errorf("project name too long (max 64 characters)")
		}
	}
	return nil
}

// getCurrentStepTitle returns the title for the current step.
func (m *WizardModel) getCurrentStepTitle() string {
	titles := []string{
		"Project Name",
		"Genre",
		"Story Premise",
		"World Setting",
		"Main Characters",
		"Plot Outline",
		"LLM Provider",
	}
	if int(m.currentStep) < len(titles) {
		return titles[m.currentStep]
	}
	return "Unknown Step"
}

// getCurrentStepHelp returns the help text for the current step.
func (m *WizardModel) getCurrentStepHelp() string {
	helps := []string{
		"Enter a name for your project (used for the folder name)",
		"Choose the primary genre for your story",
		"Describe the core premise of your story in a few sentences",
		"Describe the world, time period, and setting of your story",
		"List your main characters with their names, roles, and key traits",
		"Outline the main plot points or story beats you have in mind",
		"Choose which LLM provider to use for AI assistance",
	}
	if int(m.currentStep) < len(helps) {
		return helps[m.currentStep]
	}
	return ""
}

// View renders the wizard.
func (m *WizardModel) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var sb strings.Builder

	// Header
	header := styles.Header.Render("DREAMTELLER - Project Setup")
	sb.WriteString(header)
	sb.WriteString("\n\n")

	// Progress indicator
	progress := m.renderProgress()
	sb.WriteString(progress)
	sb.WriteString("\n\n")

	// Step title
	stepTitle := styles.Title.Render(fmt.Sprintf("Step %d: %s", m.currentStep+1, m.getCurrentStepTitle()))
	sb.WriteString(stepTitle)
	sb.WriteString("\n")

	// Step help
	stepHelp := styles.Subtitle.Render(m.getCurrentStepHelp())
	sb.WriteString(stepHelp)
	sb.WriteString("\n\n")

	// Step content
	sb.WriteString(m.renderCurrentStep())
	sb.WriteString("\n")

	// Error display
	if m.err != nil {
		sb.WriteString("\n")
		sb.WriteString(styles.ErrorText.Render("Error: " + m.err.Error()))
		sb.WriteString("\n")
	}

	// Navigation help
	sb.WriteString("\n")
	sb.WriteString(m.renderNavigationHelp())

	return sb.String()
}

// renderProgress renders the progress indicator.
func (m *WizardModel) renderProgress() string {
	var parts []string
	for i := 0; i < m.totalSteps; i++ {
		if i < int(m.currentStep) {
			parts = append(parts, styles.SuccessText.Render("●"))
		} else if i == int(m.currentStep) {
			parts = append(parts, styles.InfoText.Render("●"))
		} else {
			parts = append(parts, styles.MutedText.Render("○"))
		}
	}
	return strings.Join(parts, " ")
}

// renderCurrentStep renders the input for the current step.
func (m *WizardModel) renderCurrentStep() string {
	switch m.currentStep {
	case StepProjectName:
		return styles.FocusedBorder.Render(m.nameInput.View())
	case StepGenre:
		return m.genreList.View()
	case StepPremise:
		return styles.FocusedBorder.Render(m.premiseInput.View())
	case StepWorldSetting:
		return styles.FocusedBorder.Render(m.worldInput.View())
	case StepCharacters:
		return styles.FocusedBorder.Render(m.charactersInput.View())
	case StepPlotOutline:
		return styles.FocusedBorder.Render(m.plotInput.View())
	case StepLLMProvider:
		return m.providerList.View()
	}
	return ""
}

// renderNavigationHelp renders the navigation help text.
func (m *WizardModel) renderNavigationHelp() string {
	var parts []string

	parts = append(parts, fmt.Sprintf("%s confirm", styles.HelpKey.Render("Enter")))
	parts = append(parts, fmt.Sprintf("%s skip", styles.HelpKey.Render("Tab")))

	if m.currentStep > StepProjectName {
		parts = append(parts, fmt.Sprintf("%s back", styles.HelpKey.Render("Backspace")))
	}

	parts = append(parts, fmt.Sprintf("%s cancel", styles.HelpKey.Render("Esc")))

	return styles.HelpDesc.Render(strings.Join(parts, "  "))
}

// Result returns the collected wizard data.
func (m *WizardModel) Result() WizardResult {
	return m.result
}

// Completed returns true if the wizard completed successfully.
func (m *WizardModel) Completed() bool {
	return m.completed
}

// Cancelled returns true if the wizard was cancelled.
func (m *WizardModel) Cancelled() bool {
	return m.cancelled
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
