// Package tui provides the terminal user interface using Bubble Tea.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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

type ViewState int

const (
	ViewChat ViewState = iota
	ViewHelp
	ViewContext
	ViewChapters
	ViewSuggestion
)

type ContextMode int

const (
	ContextEssential ContextMode = iota
	ContextHybrid
	ContextFull
)

func (c ContextMode) String() string {
	switch c {
	case ContextEssential:
		return "Essential"
	case ContextHybrid:
		return "Hybrid"
	case ContextFull:
		return "Full"
	default:
		return "Unknown"
	}
}

func (c ContextMode) Next() ContextMode {
	return (c + 1) % 3
}

// Message represents a chat message.
type Message struct {
	Role    string
	Content string
}

type Model struct {
	project      *project.Project
	provider     llm.Provider
	searchEngine *search.FTSEngine
	modelName    string
	providerName string
	baseURL      string
	contextMode  ContextMode

	view       ViewState
	width      int
	height     int
	ready      bool
	err        error
	statusText string

	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	messages []Message

	streaming        bool
	inputMode        bool
	streamController *StreamController
	streamChan       <-chan llm.StreamChunk

	suggestionHandler   *SuggestionHandler
	pendingSuggestion   *SuggestionResult
	toolCallAccumulator *ToolCallAccumulator

	modelSelectMode  bool
	availableModels  []string
	modelSelectIndex int

	toast Toast
}

// New creates a new TUI model.
func New(proj *project.Project, provider llm.Provider, searchEngine *search.FTSEngine, modelName, providerName, baseURL string) *Model {
	ta := textarea.New()
	ta.Placeholder = "Enter your message... (/help for commands)"
	ta.Focus()
	ta.CharLimit = 4000
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)
	ta.Prompt = "> "
	ta.FocusedStyle.Prompt = styles.InputPrompt
	ta.FocusedStyle.Text = styles.InputText
	ta.FocusedStyle.Placeholder = styles.MutedText
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle = ta.FocusedStyle

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.Spinner

	return &Model{
		project:             proj,
		provider:            provider,
		searchEngine:        searchEngine,
		modelName:           modelName,
		providerName:        providerName,
		baseURL:             baseURL,
		textarea:            ta,
		spinner:             sp,
		messages:            []Message{},
		inputMode:           true,
		view:                ViewChat,
		suggestionHandler:   NewSuggestionHandler(proj, searchEngine),
		toolCallAccumulator: NewToolCallAccumulator(),
	}
}

func (m *Model) Init() tea.Cmd {
	m.loadHistory()

	cmds := []tea.Cmd{
		textarea.Blink,
		m.spinner.Tick,
	}

	if m.isFirstOpen() && m.provider != nil {
		cmds = append(cmds, m.sendGreeting())
	}

	return tea.Batch(cmds...)
}

func (m *Model) isFirstOpen() bool {
	return len(m.messages) == 0
}

func (m *Model) sendGreeting() tea.Cmd {
	greetingPrompt := m.buildGreetingPrompt()

	m.messages = append(m.messages, Message{
		Role:    "user",
		Content: greetingPrompt,
	})
	m.saveMessage("user", greetingPrompt)

	m.streaming = true
	m.inputMode = false

	return tea.Batch(m.spinner.Tick, m.startStream(greetingPrompt))
}

func (m *Model) buildGreetingPrompt() string {
	if m.project == nil || m.project.Info == nil {
		return "안녕하세요! 이 프로젝트에 대해 간단히 소개해주시고, 어떤 장면부터 시작하면 좋을지 제안해주세요."
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("안녕하세요! '%s' 프로젝트를 시작합니다.", m.project.Info.Name))

	if characters, err := m.project.LoadCharacters(); err == nil && len(characters) > 0 {
		names := make([]string, 0, len(characters))
		for _, c := range characters {
			names = append(names, c.Name)
		}
		parts = append(parts, fmt.Sprintf("등장인물: %s", strings.Join(names, ", ")))
	}

	parts = append(parts, "현재 설정된 캐릭터와 배경을 간단히 요약하고, 어떤 장면부터 시작하면 좋을지 제안해주세요.")

	return strings.Join(parts, " ")
}

func (m *Model) loadHistory() {
	if m.project == nil || m.project.DB == nil {
		return
	}

	history, err := m.project.DB.GetConversationHistory(defaultHistoryLoadLimit)
	if err != nil {
		return
	}

	msgs := make([]Message, 0, len(history))
	for _, record := range history {
		msgs = append(msgs, Message{Role: record.Role, Content: record.Content})
	}

	// Budget-aware truncation for what we keep in memory.
	// If provider is not available, keep the DB ordering as-is.
	if m.provider != nil {
		if env, err := newAssemblyEnv(m.project, m.provider, m.modelName); err == nil {
			msgs = truncateTUIMessagesToBudget(env.tokenizer, msgs, env.budget.History)
		}
	}

	m.messages = append(m.messages, msgs...)
}

func (m *Model) saveMessage(role, content string) {
	if m.project == nil || m.project.DB == nil {
		return
	}
	_ = m.project.DB.SaveConversationMessage(role, content)
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
		m.textarea.Focus()

		errText := "API Error"
		if msg.Err != nil {
			errText = msg.Err.Error()
		}
		toast, cmd := showToast(errText, ToastError, 5*time.Second)
		m.toast = toast
		return m, cmd

	case errMsg:
		toast, cmd := showToast(msg.err.Error(), ToastError, 5*time.Second)
		m.toast = toast
		return m, cmd

	case clearToastMsg:
		m.toast.Update(msg)
		return m, nil

	case SuggestionMsg:
		m.pendingSuggestion = msg.Suggestion
		m.view = ViewSuggestion
		m.streaming = false
		m.inputMode = false
		m.updateViewport()

	case modelsListMsg:
		if msg.err != nil {
			m.err = msg.err
			m.statusText = ""
		} else {
			m.availableModels = msg.models
			m.modelSelectIndex = 0
			m.modelSelectMode = true
			m.inputMode = false
			m.statusText = "Select a model (↑/↓ to navigate, Enter to select, Esc to cancel)"
			m.updateViewport()
		}

	case StreamReadyMsg:
		m.streamChan = msg.StreamChan
		return m, m.readNextChunk()
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
	// Handle model selection mode
	if m.modelSelectMode {
		return m.handleModelSelectKey(msg)
	}

	// Handle suggestion view keys
	if m.view == ViewSuggestion {
		return m.handleSuggestionKey(msg)
	}

	switch msg.Type {
	case tea.KeyCtrlC:
		if m.streaming {
			m.cancelStream()
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
			m.cancelStream()
			return m, nil
		}
		// Let Esc pass through to textarea (clears input)

	case tea.KeyEnter:
		if !m.streaming && m.inputMode {
			return m.handleSubmit()
		}

	case tea.KeyTab:
		if m.inputMode && !m.streaming {
			m.contextMode = m.contextMode.Next()
			return m, nil
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

func (m *Model) handleModelSelectKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.modelSelectMode = false
		m.inputMode = true
		m.statusText = ""
		m.textarea.Focus()
		m.updateViewport()
		return m, nil

	case tea.KeyEnter:
		if len(m.availableModels) > 0 && m.modelSelectIndex < len(m.availableModels) {
			m.modelName = m.availableModels[m.modelSelectIndex]
			m.statusText = fmt.Sprintf("Switched to %s", m.modelName)
		}
		m.modelSelectMode = false
		m.inputMode = true
		m.textarea.Focus()
		m.updateViewport()
		return m, nil

	case tea.KeyUp:
		if m.modelSelectIndex > 0 {
			m.modelSelectIndex--
			m.updateViewport()
		}
		return m, nil

	case tea.KeyDown:
		if m.modelSelectIndex < len(m.availableModels)-1 {
			m.modelSelectIndex++
			m.updateViewport()
		}
		return m, nil
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
	if msg.ToolCall != nil {
		m.toolCallAccumulator.AddDelta(msg.ToolCall)
	}

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

	if msg.Done {
		var cmds []tea.Cmd

		if msg.FinishReason == llm.FinishReasonContentFilter {
			toast, toastCmd := showToast("응답이 안전 필터에 의해 차단되었습니다", ToastWarning, 5*time.Second)
			m.toast = toast
			cmds = append(cmds, toastCmd)
		}

		if m.toolCallAccumulator.HasCalls() {
			model, cmd := m.processToolCalls()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return model, tea.Batch(cmds...)
		}
		if len(m.messages) > 0 && m.messages[len(m.messages)-1].Role == "assistant" {
			m.saveMessage("assistant", m.messages[len(m.messages)-1].Content)
		}
		m.streamChan = nil
		cmds = append(cmds, func() tea.Msg { return StreamDoneMsg{} })
		return m, tea.Batch(cmds...)
	}

	return m, tea.Batch(m.spinner.Tick, m.readNextChunk())
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

	if strings.HasPrefix(input, "/") {
		return m.handleCommand(input)
	}

	m.messages = append(m.messages, Message{
		Role:    "user",
		Content: input,
	})
	m.saveMessage("user", input)

	m.textarea.Reset()
	m.updateViewport()

	if m.streamController != nil {
		m.streamController.Cancel()
	}

	m.streaming = true
	m.inputMode = false

	if m.provider == nil {
		m.messages = append(m.messages, Message{
			Role:    "assistant",
			Content: "No LLM provider configured. Please set up a provider in your config.",
		})
		return m, func() tea.Msg { return StreamDoneMsg{} }
	}

	return m, tea.Batch(m.spinner.Tick, m.startStream(input))
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

	case "/models":
		return m.showModelSelection()

	default:
		m.err = fmt.Errorf("unknown command: %s", cmd)
	}

	m.textarea.Reset()
	return m, nil
}

func (m *Model) startStream(userInput string) tea.Cmd {
	provider := m.provider
	project := m.project
	contextMode := m.contextMode
	searchEngine := m.searchEngine
	messages := make([]Message, len(m.messages))
	copy(messages, m.messages)

	ctx, cancel := context.WithTimeout(context.Background(), DefaultStreamConfig().Timeout)
	m.streamController = &StreamController{ctx: ctx, cancel: cancel, config: DefaultStreamConfig()}

	return func() tea.Msg {
		assembled, err := assembleChatRequest(project, provider, m.modelName, contextMode, searchEngine, messages)
		if err != nil {
			return StreamErrorMsg{Err: err}
		}
		req := assembled.Request

		streamChan, err := provider.Stream(ctx, req)
		if err != nil {
			return StreamErrorMsg{Err: err}
		}
		return StreamReadyMsg{StreamChan: streamChan}
	}
}

func buildSystemPromptAsync(proj *project.Project, contextMode ContextMode, searchEngine *search.FTSEngine, userInput string) string {
	builder := llm.NewSystemPromptBuilder()
	builder.AddRole(llm.DefaultNovelWritingPrompt())

	if proj != nil && proj.Info != nil {
		builder.AddProjectInfo(proj.Info.Name, proj.Config.Genre)
		builder.AddWritingStyle(proj.Config.Writing)
	}

	switch contextMode {
	case ContextEssential:
		builder.AddContext(buildEssentialContextAsync(proj))

	case ContextHybrid:
		builder.AddContext(buildEssentialContextAsync(proj))
		if searchEngine != nil && userInput != "" {
			if searchContext := buildSearchContextAsync(searchEngine, userInput); searchContext != "" {
				builder.AddContext("\n### Additional Search Results\n" + searchContext)
			}
		}

	case ContextFull:
		builder.AddContext(buildFullContextAsync(proj))
	}

	return builder.Build()
}

func buildEssentialContextAsync(proj *project.Project) string {
	if proj == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## Story Context\n\n")

	if characters, err := proj.LoadCharacters(); err == nil && len(characters) > 0 {
		sb.WriteString("### Characters\n")
		for _, c := range characters {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", c.Name, truncateForEssential(c.Description, 200)))
		}
		sb.WriteString("\n")
	}

	if settings, err := proj.LoadSettings(); err == nil && len(settings) > 0 {
		sb.WriteString("### Settings\n")
		for _, s := range settings {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", s.Name, truncateForEssential(s.Description, 200)))
		}
		sb.WriteString("\n")
	}

	if plots, err := proj.LoadPlots(); err == nil && len(plots) > 0 {
		sb.WriteString("### Plot Points\n")
		for _, p := range plots {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", p.Title, truncateForEssential(p.Description, 200)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func buildSearchContextAsync(searchEngine *search.FTSEngine, query string) string {
	if searchEngine == nil {
		return ""
	}

	results, err := searchEngine.Search(query, 5)
	if err != nil || len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("**%s** (score: %.2f):\n%s\n\n", r.SourcePath, r.Score, r.Content))
	}
	return sb.String()
}

func buildFullContextAsync(proj *project.Project) string {
	if proj == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## Complete Story Context\n\n")

	if characters, err := proj.LoadCharacters(); err == nil && len(characters) > 0 {
		sb.WriteString("### Characters\n\n")
		for _, c := range characters {
			sb.WriteString(fmt.Sprintf("#### %s\n%s\n\n", c.Name, c.Description))
		}
	}

	if settings, err := proj.LoadSettings(); err == nil && len(settings) > 0 {
		sb.WriteString("### Settings\n\n")
		for _, s := range settings {
			sb.WriteString(fmt.Sprintf("#### %s\n%s\n\n", s.Name, s.Description))
		}
	}

	if plots, err := proj.LoadPlots(); err == nil && len(plots) > 0 {
		sb.WriteString("### Plot\n\n")
		for _, p := range plots {
			sb.WriteString(fmt.Sprintf("#### %s\n%s\n\n", p.Title, p.Description))
		}
	}

	return sb.String()
}

func buildChatMessagesAsync(systemPrompt string, messages []Message) []llm.ChatMessage {
	chatMessages := []llm.ChatMessage{
		llm.NewSystemMessage(systemPrompt),
	}

	for _, msg := range messages {
		switch msg.Role {
		case "user":
			chatMessages = append(chatMessages, llm.NewUserMessage(msg.Content))
		case "assistant":
			chatMessages = append(chatMessages, llm.NewAssistantMessage(msg.Content))
		}
	}

	return chatMessages
}

func (m *Model) readNextChunk() tea.Cmd {
	return func() tea.Msg {
		if m.streamChan == nil {
			return StreamDoneMsg{}
		}

		chunk, ok := <-m.streamChan
		if !ok {
			return StreamChunkMsg{Done: true}
		}

		if chunk.Error != nil {
			return StreamErrorMsg{Err: chunk.Error}
		}

		return StreamChunkMsg{
			Content:      chunk.Delta,
			ToolCall:     chunk.ToolCall,
			Done:         chunk.Done,
			FinishReason: chunk.FinishReason,
		}
	}
}

func (m *Model) buildSystemPrompt(userInput string) string {
	builder := llm.NewSystemPromptBuilder()
	builder.AddRole(llm.DefaultNovelWritingPrompt())

	if m.project != nil && m.project.Info != nil {
		builder.AddProjectInfo(m.project.Info.Name, m.project.Config.Genre)
		builder.AddWritingStyle(m.project.Config.Writing)
	}

	switch m.contextMode {
	case ContextEssential:
		builder.AddContext(m.buildEssentialContext())

	case ContextHybrid:
		builder.AddContext(m.buildEssentialContext())
		if m.searchEngine != nil && userInput != "" {
			if searchContext := m.buildSearchContext(userInput); searchContext != "" {
				builder.AddContext("\n### Additional Search Results\n" + searchContext)
			}
		}

	case ContextFull:
		builder.AddContext(m.buildFullContext())
	}

	return builder.Build()
}

func (m *Model) buildEssentialContext() string {
	if m.project == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## Story Context\n\n")

	if characters, err := m.project.LoadCharacters(); err == nil && len(characters) > 0 {
		sb.WriteString("### Characters\n")
		for _, c := range characters {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", c.Name, truncateForEssential(c.Description, 200)))
		}
		sb.WriteString("\n")
	}

	if settings, err := m.project.LoadSettings(); err == nil && len(settings) > 0 {
		sb.WriteString("### Settings\n")
		for _, s := range settings {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", s.Name, truncateForEssential(s.Description, 200)))
		}
		sb.WriteString("\n")
	}

	if plots, err := m.project.LoadPlots(); err == nil && len(plots) > 0 {
		sb.WriteString("### Plot\n")
		for _, p := range plots {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", p.Title, truncateForEssential(p.Description, 200)))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m *Model) buildFullContext() string {
	if m.project == nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## Complete Story Context\n\n")

	if characters, err := m.project.LoadCharacters(); err == nil && len(characters) > 0 {
		sb.WriteString("### Characters\n\n")
		for _, c := range characters {
			sb.WriteString(fmt.Sprintf("#### %s\n%s\n\n", c.Name, c.Description))
		}
	}

	if settings, err := m.project.LoadSettings(); err == nil && len(settings) > 0 {
		sb.WriteString("### Settings\n\n")
		for _, s := range settings {
			sb.WriteString(fmt.Sprintf("#### %s\n%s\n\n", s.Name, s.Description))
		}
	}

	if plots, err := m.project.LoadPlots(); err == nil && len(plots) > 0 {
		sb.WriteString("### Plot\n\n")
		for _, p := range plots {
			sb.WriteString(fmt.Sprintf("#### %s\n%s\n\n", p.Title, p.Description))
		}
	}

	return sb.String()
}

func (m *Model) buildSearchContext(userInput string) string {
	if m.searchEngine == nil || userInput == "" {
		return ""
	}

	results, err := m.searchEngine.Search(userInput, 8)
	if err != nil || len(results) == 0 {
		return ""
	}

	chunks := make([]llm.ContextChunk, 0, len(results))
	for _, r := range results {
		chunks = append(chunks, llm.ContextChunk{
			Content:    r.Content,
			SourceType: r.SourceType,
			SourcePath: r.SourcePath,
			Score:      r.Score,
		})
	}
	return (&llm.ContextManager{}).BuildContextPrompt(chunks)
}

func truncateForEssential(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")
	if len(lines) > 0 {
		s = lines[0]
	}
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}

// buildChatMessages converts internal messages to LLM format.
func (m *Model) buildChatMessages(systemPrompt string) []llm.ChatMessage {
	messages := []llm.ChatMessage{
		llm.NewSystemMessage(systemPrompt),
	}

	for _, msg := range m.messages {
		switch msg.Role {
		case "user":
			messages = append(messages, llm.NewUserMessage(msg.Content))
		case "assistant":
			messages = append(messages, llm.NewAssistantMessage(msg.Content))
		}
	}

	return messages
}

// cancelStream cancels the current streaming operation.
func (m *Model) cancelStream() {
	if m.streamController != nil {
		m.streamController.Cancel()
	}
	m.streaming = false
	m.inputMode = true
	m.streamChan = nil
	m.textarea.Focus()
}

// updateViewport updates the viewport content.
func (m *Model) updateViewport() {
	var content string

	if m.modelSelectMode {
		content = m.renderModelSelect()
		m.viewport.SetContent(content)
		return
	}

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

func (m *Model) renderModelSelect() string {
	var sb strings.Builder
	sb.WriteString(styles.Title.Render("Select Model"))
	sb.WriteString("\n\n")

	if len(m.availableModels) == 0 {
		sb.WriteString(styles.MutedText.Render("No models available"))
		return sb.String()
	}

	for i, model := range m.availableModels {
		prefix := "  "
		style := styles.MutedText
		if i == m.modelSelectIndex {
			prefix = "> "
			style = styles.SelectedItem
		}
		if model == m.modelName {
			sb.WriteString(style.Render(fmt.Sprintf("%s%s (current)\n", prefix, model)))
		} else {
			sb.WriteString(style.Render(fmt.Sprintf("%s%s\n", prefix, model)))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(styles.HelpDesc.Render("↑/↓ Navigate • Enter Select • Esc Cancel"))
	return sb.String()
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

	if m.view == ViewChat {
		sb.WriteString(styles.MutedText.Render(strings.Repeat("─", m.width)))
		sb.WriteString("\n")
		sb.WriteString(m.textarea.View())
		sb.WriteString("\n")
		sb.WriteString(styles.MutedText.Render(strings.Repeat("─", m.width)))
	}

	modelInfo := styles.StatusBar.Render("🤖 " + m.modelName)
	contextInfo := styles.HelpKey.Render("[Tab]") + styles.HelpDesc.Render(" "+m.contextMode.String())
	helpHint := styles.HelpKey.Render("/help") + styles.HelpDesc.Render(" for commands")

	leftPart := modelInfo + "  " + contextInfo

	if m.streaming {
		spinnerPart := m.spinner.View() + " " + styles.HelpKey.Render("[esc]") + styles.HelpDesc.Render(" interrupt")
		gap := m.width - lipgloss.Width(leftPart) - lipgloss.Width(spinnerPart)
		if gap < 0 {
			gap = 0
		}
		statusLine := leftPart + strings.Repeat(" ", gap) + spinnerPart
		sb.WriteString("\n")
		sb.WriteString(statusLine)
	} else {
		gap := m.width - lipgloss.Width(leftPart) - lipgloss.Width(helpHint)
		if gap < 0 {
			gap = 0
		}
		statusLine := leftPart + strings.Repeat(" ", gap) + helpHint
		sb.WriteString("\n")
		sb.WriteString(statusLine)
	}

	appView := sb.String()

	if m.toast.Visible {
		toastView := m.toast.View(m.width / 2)
		appView = renderToastTopRight(toastView, appView, 2)
	}

	return appView
}

type StreamChunkMsg struct {
	Content      string
	ToolCall     *llm.ToolCallDelta
	Done         bool
	FinishReason string
}

type StreamDoneMsg struct{}

type StreamErrorMsg struct {
	Err error
}

type StreamReadyMsg struct {
	StreamChan <-chan llm.StreamChunk
}

type errMsg struct {
	err error
}

func (m *Model) showModelSelection() (tea.Model, tea.Cmd) {
	m.statusText = "Fetching models..."
	m.textarea.Reset()

	return m, m.fetchModelsCmd()
}

type modelsListMsg struct {
	models []string
	err    error
}

func (m *Model) fetchModelsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.providerName != "local" {
			return modelsListMsg{err: fmt.Errorf("/models only supported for local provider")}
		}

		models, err := fetchLocalModelsForTUI(m.baseURL)
		return modelsListMsg{models: models, err: err}
	}
}

func fetchLocalModelsForTUI(baseURL string) ([]string, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("no base URL configured")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(strings.TrimSuffix(baseURL, "/") + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]string, len(result.Models))
	for i, m := range result.Models {
		models[i] = m.Name
	}

	return models, nil
}
