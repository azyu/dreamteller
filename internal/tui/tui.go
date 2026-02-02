// Package tui provides the terminal user interface using Bubble Tea.
package tui

import (
	"fmt"
	"strings"

	"github.com/azyu/dreamteller/internal/llm"
	"github.com/azyu/dreamteller/internal/project"
	"github.com/azyu/dreamteller/internal/search"
	"github.com/azyu/dreamteller/internal/tui/styles"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ViewState represents the current view mode.
type ViewState int

const (
	ViewChat ViewState = iota
	ViewHelp
	ViewContext
	ViewChapters
	ViewSuggestion
)

// Message represents a chat message.
type Message struct {
	Role    string
	Content string
}

// Model is the main TUI model.
type Model struct {
	// Project
	project *project.Project

	// View state
	view       ViewState
	width      int
	height     int
	ready      bool
	err        error
	statusText string

	// Chat components
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	messages []Message

	// State flags
	streaming bool
	inputMode bool

	// Suggestion handling
	suggestionHandler  *SuggestionHandler
	pendingSuggestion  *SuggestionResult
	toolCallAccumulator *ToolCallAccumulator
}

// New creates a new TUI model.
func New(proj *project.Project, searchEngine *search.FTSEngine) *Model {
	ta := textarea.New()
	ta.Placeholder = "Enter your message... (/help for commands)"
	ta.Focus()
	ta.CharLimit = 4000
	ta.SetWidth(80)
	ta.SetHeight(3)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.Spinner

	return &Model{
		project:             proj,
		textarea:            ta,
		spinner:             sp,
		messages:            []Message{},
		inputMode:           true,
		view:                ViewChat,
		suggestionHandler:   NewSuggestionHandler(proj, searchEngine),
		toolCallAccumulator: NewToolCallAccumulator(),
	}
}

// Init initializes the model.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		m.spinner.Tick,
	)
}

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle special keys first
		model, cmd := m.handleKeyMsg(msg)
		if cmd != nil {
			return model, cmd
		}
		// If no command returned, continue to update textarea below

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-8)
			m.viewport.YPosition = 2
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 8
		}

		m.textarea.SetWidth(msg.Width - 4)
		m.updateViewport()

	case spinner.TickMsg:
		if m.streaming {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case StreamChunkMsg:
		return m.handleStreamChunk(msg)

	case StreamDoneMsg:
		m.streaming = false
		m.inputMode = true
		m.textarea.Focus()
		m.updateViewport()

	case StreamErrorMsg:
		m.streaming = false
		m.inputMode = true
		m.err = msg.Err
		m.textarea.Focus()

	case errMsg:
		m.err = msg.err

	case SuggestionMsg:
		m.pendingSuggestion = msg.Suggestion
		m.view = ViewSuggestion
		m.streaming = false
		m.inputMode = false
		m.updateViewport()
	}

	// Update textarea if in input mode
	if m.inputMode && !m.streaming {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update viewport
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// handleKeyMsg handles keyboard input.
// Returns (model, cmd) where cmd is nil if the key should be passed to textarea.
func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle suggestion view keys
	if m.view == ViewSuggestion {
		return m.handleSuggestionKey(msg)
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		if m.streaming {
			// Cancel streaming
			m.streaming = false
			m.inputMode = true
			m.textarea.Focus()
			return m, nil
		}
		return m, tea.Quit

	case tea.KeyEsc:
		if m.view != ViewChat {
			m.view = ViewChat
			m.updateViewport()
			return m, nil
		}
		if m.streaming {
			m.streaming = false
			m.inputMode = true
			m.textarea.Focus()
			return m, nil
		}
		// Let Esc pass through to textarea (clears input)

	case tea.KeyEnter:
		if !m.streaming && m.inputMode {
			return m.handleSubmit()
		}
	}

	// Return nil cmd to let the key pass through to textarea
	return m, nil
}

// handleSuggestionKey handles keyboard input in suggestion view.
func (m *Model) handleSuggestionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		return m.rejectSuggestion()
	case tea.KeyRunes:
		key := string(msg.Runes)
		switch key {
		case "a", "y":
			return m.acceptSuggestion()
		case "r", "n":
			return m.rejectSuggestion()
		case "m", "e":
			// Modify - return to chat with suggestion context
			if m.pendingSuggestion != nil {
				m.messages = append(m.messages, Message{
					Role:    "system",
					Content: fmt.Sprintf("Suggestion pending modification: %s", m.pendingSuggestion.Title),
				})
			}
			m.pendingSuggestion = nil
			m.view = ViewChat
			m.inputMode = true
			m.textarea.Focus()
			m.updateViewport()
			return m, nil
		default:
			// Check if the key matches an action
			if m.pendingSuggestion != nil {
				for _, action := range m.pendingSuggestion.Actions {
					if action.Key == key && action.Handler != nil {
						if err := action.Handler(); err != nil {
							m.err = err
						} else {
							m.messages = append(m.messages, Message{
								Role:    "system",
								Content: fmt.Sprintf("Selected: %s", action.Label),
							})
						}
						m.pendingSuggestion = nil
						m.view = ViewChat
						m.inputMode = true
						m.textarea.Focus()
						m.updateViewport()
						return m, nil
					}
				}
			}
		}
	}
	return m, nil
}

// acceptSuggestion handles accepting a pending suggestion.
func (m *Model) acceptSuggestion() (tea.Model, tea.Cmd) {
	if m.pendingSuggestion == nil {
		return m.returnToChat()
	}

	// For context updates that require approval, execute the update
	if m.pendingSuggestion.RequiresApproval && m.pendingSuggestion.Type == SuggestionTypeContextUpdate {
		update, ok := m.pendingSuggestion.ParsedData.(llm.ContextUpdate)
		if ok {
			if err := m.suggestionHandler.ExecuteContextUpdate(update); err != nil {
				m.err = err
			} else {
				m.messages = append(m.messages, Message{
					Role:    "system",
					Content: fmt.Sprintf("Context update applied: %s/%s.md", update.FileType, update.FileName),
				})
			}
		}
	} else {
		// For other suggestions, just acknowledge
		m.messages = append(m.messages, Message{
			Role:    "system",
			Content: fmt.Sprintf("Accepted: %s", m.pendingSuggestion.Title),
		})
	}

	return m.returnToChat()
}

// rejectSuggestion handles rejecting a pending suggestion.
func (m *Model) rejectSuggestion() (tea.Model, tea.Cmd) {
	if m.pendingSuggestion != nil {
		m.messages = append(m.messages, Message{
			Role:    "system",
			Content: fmt.Sprintf("Rejected: %s", m.pendingSuggestion.Title),
		})
	}

	return m.returnToChat()
}

// returnToChat returns from suggestion view to chat view.
func (m *Model) returnToChat() (tea.Model, tea.Cmd) {
	m.pendingSuggestion = nil
	m.view = ViewChat
	m.inputMode = true
	m.textarea.Focus()
	m.updateViewport()
	return m, nil
}

// handleStreamChunk handles incoming stream chunks.
func (m *Model) handleStreamChunk(msg StreamChunkMsg) (tea.Model, tea.Cmd) {
	// Handle tool call deltas
	if msg.ToolCall != nil {
		m.toolCallAccumulator.AddDelta(msg.ToolCall)
		return m, m.spinner.Tick
	}

	// Handle text content
	if msg.Content != "" {
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
			m.messages[len(m.messages)-1].Content += msg.Content
		} else {
			m.messages = append(m.messages, Message{
				Role:    "assistant",
				Content: msg.Content,
			})
		}
		m.updateViewport()
	}

	// Check if stream is done with tool calls
	if msg.Done && m.toolCallAccumulator.HasCalls() {
		return m.processToolCalls()
	}

	return m, m.spinner.Tick
}

// processToolCalls processes accumulated tool calls.
func (m *Model) processToolCalls() (tea.Model, tea.Cmd) {
	calls := m.toolCallAccumulator.GetCompletedCalls()
	m.toolCallAccumulator.Reset()

	if len(calls) == 0 {
		return m, nil
	}

	// Process the first tool call (support single tool call for now)
	call := calls[0]
	suggestion, err := m.suggestionHandler.HandleToolCall(call)
	if err != nil {
		m.err = err
		m.streaming = false
		m.inputMode = true
		m.textarea.Focus()
		return m, nil
	}

	return m, func() tea.Msg {
		return SuggestionMsg{Suggestion: suggestion}
	}
}

// handleSubmit processes user input.
func (m *Model) handleSubmit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.textarea.Value())
	if input == "" {
		return m, nil
	}

	// Check for slash commands
	if strings.HasPrefix(input, "/") {
		return m.handleCommand(input)
	}

	// Add user message
	m.messages = append(m.messages, Message{
		Role:    "user",
		Content: input,
	})

	// Clear input
	m.textarea.Reset()
	m.updateViewport()

	// Start streaming (placeholder - will be implemented with LLM)
	m.streaming = true
	m.inputMode = false

	// For now, return a mock response
	return m, m.mockStreamResponse()
}

// handleCommand processes slash commands.
func (m *Model) handleCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/help":
		m.view = ViewHelp
		m.updateViewport()

	case "/quit", "/exit", "/q":
		return m, tea.Quit

	case "/clear":
		m.messages = []Message{}
		m.updateViewport()

	case "/context":
		m.view = ViewContext
		m.updateViewport()

	case "/chapters":
		m.view = ViewChapters
		m.updateViewport()

	case "/back":
		m.view = ViewChat
		m.updateViewport()

	case "/search":
		if len(parts) > 1 {
			query := strings.Join(parts[1:], " ")
			m.statusText = fmt.Sprintf("Searching: %s", query)
			// TODO: Implement search
		} else {
			m.err = fmt.Errorf("usage: /search <query>")
		}

	case "/chapter":
		if len(parts) > 1 {
			m.statusText = fmt.Sprintf("Switching to chapter: %s", parts[1])
			// TODO: Implement chapter switching
		} else {
			m.err = fmt.Errorf("usage: /chapter <number>")
		}

	case "/reindex":
		m.statusText = "Reindexing..."
		// TODO: Implement reindex

	default:
		m.err = fmt.Errorf("unknown command: %s", cmd)
	}

	m.textarea.Reset()
	return m, nil
}

// mockStreamResponse simulates streaming for testing.
func (m *Model) mockStreamResponse() tea.Cmd {
	return func() tea.Msg {
		// Simulate AI response
		response := "I understand you want to work on your novel. What would you like to do? I can help you:\n\n" +
			"- Develop your characters further\n" +
			"- Work on plot points\n" +
			"- Write or edit chapters\n" +
			"- Explore world-building details\n\n" +
			"Just let me know what you'd like to focus on!"

		m.messages = append(m.messages, Message{
			Role:    "assistant",
			Content: response,
		})

		return StreamDoneMsg{}
	}
}

// updateViewport updates the viewport content.
func (m *Model) updateViewport() {
	var content string

	switch m.view {
	case ViewChat:
		content = m.renderChat()
	case ViewHelp:
		content = m.renderHelp()
	case ViewContext:
		content = m.renderContext()
	case ViewChapters:
		content = m.renderChapters()
	case ViewSuggestion:
		content = m.renderSuggestion()
	}

	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

// renderChat renders the chat view.
func (m *Model) renderChat() string {
	var sb strings.Builder

	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			sb.WriteString(styles.UserMessage.Render("You: " + msg.Content))
		case "assistant":
			sb.WriteString(styles.AssistantMessage.Render("AI: " + msg.Content))
		case "system":
			sb.WriteString(styles.SystemMessage.Render(msg.Content))
		}
		sb.WriteString("\n\n")
	}

	if m.streaming {
		sb.WriteString(m.spinner.View() + " Thinking...")
	}

	return sb.String()
}

// renderHelp renders the help view.
func (m *Model) renderHelp() string {
	help := `
DREAMTELLER - Help

Commands:
  /help      - Show this help
  /quit      - Exit the application
  /clear     - Clear chat history
  /context   - View/manage context files
  /chapters  - View/manage chapters
  /search    - Search context (usage: /search <query>)
  /chapter   - Switch chapter (usage: /chapter <number>)
  /reindex   - Rebuild search index
  /back      - Return to chat view

Keyboard Shortcuts:
  Ctrl+C     - Cancel current operation / Quit
  Esc        - Cancel / Return to chat
  Enter      - Submit message

Press /back or Esc to return to chat.
`
	return styles.InfoText.Render(help)
}

// renderContext renders the context management view.
func (m *Model) renderContext() string {
	var sb strings.Builder
	sb.WriteString(styles.Title.Render("Context Files"))
	sb.WriteString("\n\n")

	if m.project == nil {
		sb.WriteString(styles.ErrorText.Render("No project loaded"))
		return sb.String()
	}

	// Characters
	sb.WriteString(styles.Subtitle.Render("Characters:"))
	sb.WriteString("\n")
	characters, _ := m.project.LoadCharacters()
	if len(characters) == 0 {
		sb.WriteString(styles.MutedText.Render("  No characters defined\n"))
	} else {
		for _, c := range characters {
			sb.WriteString(styles.ListItem.Render("  - " + c.Name + "\n"))
		}
	}

	// Settings
	sb.WriteString("\n")
	sb.WriteString(styles.Subtitle.Render("Settings:"))
	sb.WriteString("\n")
	settings, _ := m.project.LoadSettings()
	if len(settings) == 0 {
		sb.WriteString(styles.MutedText.Render("  No settings defined\n"))
	} else {
		for _, s := range settings {
			sb.WriteString(styles.ListItem.Render("  - " + s.Name + "\n"))
		}
	}

	// Plots
	sb.WriteString("\n")
	sb.WriteString(styles.Subtitle.Render("Plot Points:"))
	sb.WriteString("\n")
	plots, _ := m.project.LoadPlots()
	if len(plots) == 0 {
		sb.WriteString(styles.MutedText.Render("  No plot points defined\n"))
	} else {
		for _, p := range plots {
			sb.WriteString(styles.ListItem.Render(fmt.Sprintf("  %d. %s\n", p.Order, p.Title)))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(styles.MutedText.Render("Press /back or Esc to return to chat."))

	return sb.String()
}

// renderChapters renders the chapters view.
func (m *Model) renderChapters() string {
	var sb strings.Builder
	sb.WriteString(styles.Title.Render("Chapters"))
	sb.WriteString("\n\n")

	if m.project == nil {
		sb.WriteString(styles.ErrorText.Render("No project loaded"))
		return sb.String()
	}

	chapters, _ := m.project.LoadChapters()
	if len(chapters) == 0 {
		sb.WriteString(styles.MutedText.Render("No chapters written yet.\n"))
		sb.WriteString(styles.InfoText.Render("Start chatting to begin writing!"))
	} else {
		for _, ch := range chapters {
			sb.WriteString(styles.ListItem.Render(
				fmt.Sprintf("  Chapter %d: %s\n", ch.Number, ch.Title),
			))
		}
	}

	sb.WriteString("\n\n")
	sb.WriteString(styles.MutedText.Render("Press /back or Esc to return to chat."))

	return sb.String()
}

// renderSuggestion renders the suggestion view.
func (m *Model) renderSuggestion() string {
	var sb strings.Builder

	if m.pendingSuggestion == nil {
		sb.WriteString(styles.MutedText.Render("No pending suggestion."))
		return sb.String()
	}

	// Title
	sb.WriteString(styles.Title.Render(m.pendingSuggestion.Title))
	sb.WriteString("\n\n")

	// Content
	sb.WriteString(m.pendingSuggestion.Content)
	sb.WriteString("\n")

	// Actions
	if len(m.pendingSuggestion.Actions) > 0 {
		sb.WriteString(styles.Subtitle.Render("Actions:"))
		sb.WriteString("\n")
		for _, action := range m.pendingSuggestion.Actions {
			sb.WriteString(fmt.Sprintf("  [%s] %s\n", styles.HelpKey.Render(action.Key), action.Label))
		}
		sb.WriteString("\n")
	}

	// Standard controls
	if m.pendingSuggestion.RequiresApproval {
		sb.WriteString(styles.InfoText.Render("This action requires your approval."))
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("  [%s] Accept  ", styles.HelpKey.Render("a")))
		sb.WriteString(fmt.Sprintf("[%s] Reject  ", styles.HelpKey.Render("r")))
		sb.WriteString(fmt.Sprintf("[%s] Edit", styles.HelpKey.Render("e")))
	} else {
		sb.WriteString(fmt.Sprintf("  [%s] OK  ", styles.HelpKey.Render("a")))
		sb.WriteString(fmt.Sprintf("[%s] Dismiss", styles.HelpKey.Render("Esc")))
	}

	return sb.String()
}

// View renders the TUI.
func (m *Model) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var sb strings.Builder

	// Header
	projectName := "No Project"
	if m.project != nil && m.project.Info != nil {
		projectName = m.project.Info.Name
	}
	header := styles.Header.Render(fmt.Sprintf("DREAMTELLER - %s", projectName))
	sb.WriteString(header)
	sb.WriteString("\n")

	// Main content
	sb.WriteString(m.viewport.View())
	sb.WriteString("\n")

	// Error display
	if m.err != nil {
		sb.WriteString(styles.ErrorText.Render("Error: "+m.err.Error()) + "\n")
		m.err = nil
	}

	// Status bar
	if m.statusText != "" {
		sb.WriteString(styles.StatusBar.Render(m.statusText) + "\n")
		m.statusText = ""
	}

	// Input area (only in chat view)
	if m.view == ViewChat {
		sb.WriteString(styles.InputPrompt.Render("> "))
		sb.WriteString(m.textarea.View())
	}

	// Help hint
	helpHint := styles.HelpKey.Render("/help") + styles.HelpDesc.Render(" for commands")
	sb.WriteString("\n")
	sb.WriteString(lipgloss.PlaceHorizontal(m.width, lipgloss.Right, helpHint))

	return sb.String()
}

// Message types for streaming
type StreamChunkMsg struct {
	Content  string
	ToolCall *llm.ToolCallDelta
	Done     bool
}

type StreamDoneMsg struct{}

type StreamErrorMsg struct {
	Err error
}

type errMsg struct {
	err error
}
