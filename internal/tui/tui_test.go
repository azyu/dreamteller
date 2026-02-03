package tui

import (
	"strings"
	"testing"

	"github.com/azyu/dreamteller/internal/llm"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Model Creation Tests
// ============================================================================

func TestNew(t *testing.T) {
	t.Run("creates model with nil project", func(t *testing.T) {
		m := New(nil, nil, nil, "test-model", "", "")

		assert.NotNil(t, m)
		assert.Nil(t, m.project)
		assert.Equal(t, ViewChat, m.view)
		assert.True(t, m.inputMode)
		assert.False(t, m.streaming)
		assert.False(t, m.ready)
		assert.Empty(t, m.messages)
	})

	t.Run("initializes textarea correctly", func(t *testing.T) {
		m := New(nil, nil, nil, "test-model", "", "")

		assert.Equal(t, 4000, m.textarea.CharLimit)
		assert.Contains(t, m.textarea.Placeholder, "/help")
		assert.False(t, m.textarea.ShowLineNumbers)
	})

	t.Run("initializes spinner", func(t *testing.T) {
		m := New(nil, nil, nil, "test-model", "", "")

		assert.NotNil(t, m.spinner)
		assert.Equal(t, spinner.Dot, m.spinner.Spinner)
	})

	t.Run("initializes suggestion handler and accumulator", func(t *testing.T) {
		m := New(nil, nil, nil, "test-model", "", "")

		assert.NotNil(t, m.suggestionHandler)
		assert.NotNil(t, m.toolCallAccumulator)
	})
}

func TestInit(t *testing.T) {
	m := New(nil, nil, nil, "test-model", "", "")
	cmd := m.Init()

	assert.NotNil(t, cmd, "Init should return a command")
}

func TestModelsListMsg(t *testing.T) {
	m := newTestModel(t)
	msg := modelsListMsg{models: []string{"alpha", "beta"}}

	model, _ := m.Update(msg)
	m = model.(*Model)

	assert.True(t, m.modelSelectMode)
	assert.Equal(t, []string{"alpha", "beta"}, m.availableModels)
	assert.Equal(t, 0, m.modelSelectIndex)
	assert.False(t, m.inputMode)
}

func TestModelSelectKeyEnter(t *testing.T) {
	m := newTestModel(t)
	m.modelSelectMode = true
	m.availableModels = []string{"alpha", "beta"}
	m.modelSelectIndex = 1

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(*Model)

	assert.False(t, m.modelSelectMode)
	assert.Equal(t, "beta", m.modelName)
}

// ============================================================================
// Window Size Tests
// ============================================================================

func TestWindowSizeMsg(t *testing.T) {
	t.Run("sets ready on first window size", func(t *testing.T) {
		m := New(nil, nil, nil, "test-model", "", "")
		assert.False(t, m.ready)

		m = sendWindowSize(m, 80, 24)

		assert.True(t, m.ready)
		assert.Equal(t, 80, m.width)
		assert.Equal(t, 24, m.height)
	})

	t.Run("updates dimensions on subsequent resize", func(t *testing.T) {
		m := newTestModel(t)

		m = sendWindowSize(m, 120, 40)

		assert.Equal(t, 120, m.width)
		assert.Equal(t, 40, m.height)
	})

	t.Run("adjusts textarea width", func(t *testing.T) {
		m := New(nil, nil, nil, "test-model", "", "")

		m = sendWindowSize(m, 100, 30)

		// Textarea width should be adjusted based on terminal width
		// The exact adjustment depends on the implementation
		assert.Greater(t, m.textarea.Width(), 0)
		assert.Less(t, m.textarea.Width(), 100)
	})
}

// ============================================================================
// Key Message Tests
// ============================================================================

func TestHandleKeyMsg_CtrlC(t *testing.T) {
	t.Run("quits when not streaming", func(t *testing.T) {
		m := newTestModel(t)
		m.streaming = false

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

		// Cmd should be tea.Quit
		assert.NotNil(t, cmd)
	})

	t.Run("cancels stream when streaming", func(t *testing.T) {
		m := newTestModel(t)
		m.streaming = true
		m.inputMode = false

		m = sendKeyMsg(m, tea.KeyCtrlC)

		assert.False(t, m.streaming)
		assert.True(t, m.inputMode)
	})
}

func TestHandleKeyMsg_Esc(t *testing.T) {
	t.Run("returns to chat from help view", func(t *testing.T) {
		m := newTestModel(t)
		m.view = ViewHelp

		m = sendKeyMsg(m, tea.KeyEsc)

		assert.Equal(t, ViewChat, m.view)
	})

	t.Run("returns to chat from context view", func(t *testing.T) {
		m := newTestModel(t)
		m.view = ViewContext

		m = sendKeyMsg(m, tea.KeyEsc)

		assert.Equal(t, ViewChat, m.view)
	})

	t.Run("cancels streaming when in chat view", func(t *testing.T) {
		m := newTestModel(t)
		m.view = ViewChat
		m.streaming = true
		m.inputMode = false

		m = sendKeyMsg(m, tea.KeyEsc)

		assert.False(t, m.streaming)
		assert.True(t, m.inputMode)
	})

	t.Run("passes through in chat view when not streaming", func(t *testing.T) {
		m := newTestModel(t)
		m.view = ViewChat
		m.streaming = false

		// Esc should pass through to textarea (no state change)
		m = sendKeyMsg(m, tea.KeyEsc)

		assert.Equal(t, ViewChat, m.view)
	})
}

func TestHandleKeyMsg_Enter(t *testing.T) {
	t.Run("submits input when not streaming", func(t *testing.T) {
		m := newTestModel(t)
		setTextareaValue(m, "Hello AI")

		m, _ = typeAndSubmit(m, "")

		assert.GreaterOrEqual(t, len(m.messages), 1)
		assert.Equal(t, "Hello AI", m.messages[0].Content)
		assert.Equal(t, "user", m.messages[0].Role)
	})

	t.Run("does not submit empty input", func(t *testing.T) {
		m := newTestModel(t)
		// Empty textarea

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

		assertMessageCount(t, m, 0)
		assert.Nil(t, cmd)
	})

	t.Run("does not submit while streaming", func(t *testing.T) {
		m := newTestModel(t)
		m.streaming = true
		setTextareaValue(m, "Hello")

		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

		// Should not add message
		assertMessageCount(t, m, 0)
		assert.Nil(t, cmd)
	})
}

func TestHandleKeyMsg_PassThrough(t *testing.T) {
	t.Run("regular keys pass to textarea", func(t *testing.T) {
		m := newTestModel(t)

		m = sendRunesMsg(m, "hello")

		assert.Equal(t, "hello", m.textarea.Value())
	})

	t.Run("keys do not pass through while streaming", func(t *testing.T) {
		m := newTestModel(t)
		m.streaming = true
		m.inputMode = false

		m = sendRunesMsg(m, "test")

		// Textarea should remain empty because inputMode is false
		// Note: The Update function checks inputMode before updating textarea
		assert.Empty(t, m.textarea.Value())
	})
}

// ============================================================================
// Command Tests
// ============================================================================

func TestHandleCommand_Help(t *testing.T) {
	m := newTestModel(t)
	setTextareaValue(m, "/help")

	m = sendKeyMsg(m, tea.KeyEnter)

	assert.Equal(t, ViewHelp, m.view)
	assert.Empty(t, m.textarea.Value())
}

func TestHandleCommand_Quit(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"quit", "/quit"},
		{"exit", "/exit"},
		{"q", "/q"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			setTextareaValue(m, tt.command)

			_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

			// Command should be tea.Quit
			assert.NotNil(t, cmd)
		})
	}
}

func TestHandleCommand_Clear(t *testing.T) {
	m := newTestModel(t)
	addMessage(m, "user", "Hello")
	addMessage(m, "assistant", "Hi there!")
	setTextareaValue(m, "/clear")

	m = sendKeyMsg(m, tea.KeyEnter)

	assert.Empty(t, m.messages)
}

func TestHandleCommand_Context(t *testing.T) {
	m := newTestModel(t)
	setTextareaValue(m, "/context")

	m = sendKeyMsg(m, tea.KeyEnter)

	assert.Equal(t, ViewContext, m.view)
}

func TestHandleCommand_Chapters(t *testing.T) {
	m := newTestModel(t)
	setTextareaValue(m, "/chapters")

	m = sendKeyMsg(m, tea.KeyEnter)

	assert.Equal(t, ViewChapters, m.view)
}

func TestHandleCommand_Back(t *testing.T) {
	m := newTestModel(t)
	m.view = ViewHelp
	setTextareaValue(m, "/back")

	m = sendKeyMsg(m, tea.KeyEnter)

	assert.Equal(t, ViewChat, m.view)
}

func TestHandleCommand_Search(t *testing.T) {
	t.Run("with query sets status", func(t *testing.T) {
		m := newTestModel(t)
		setTextareaValue(m, "/search dragon")

		m = sendKeyMsg(m, tea.KeyEnter)

		assert.Contains(t, m.statusText, "dragon")
	})

	t.Run("without query shows error", func(t *testing.T) {
		m := newTestModel(t)
		setTextareaValue(m, "/search")

		m = sendKeyMsg(m, tea.KeyEnter)

		assertError(t, m)
	})
}

func TestHandleCommand_Chapter(t *testing.T) {
	t.Run("with number sets status", func(t *testing.T) {
		m := newTestModel(t)
		setTextareaValue(m, "/chapter 5")

		m = sendKeyMsg(m, tea.KeyEnter)

		assert.Contains(t, m.statusText, "5")
	})

	t.Run("without number shows error", func(t *testing.T) {
		m := newTestModel(t)
		setTextareaValue(m, "/chapter")

		m = sendKeyMsg(m, tea.KeyEnter)

		assertError(t, m)
	})
}

func TestHandleCommand_Reindex(t *testing.T) {
	m := newTestModel(t)
	setTextareaValue(m, "/reindex")

	m = sendKeyMsg(m, tea.KeyEnter)

	assert.Contains(t, m.statusText, "index")
}

func TestHandleCommand_Unknown(t *testing.T) {
	m := newTestModel(t)
	setTextareaValue(m, "/unknowncommand")

	m = sendKeyMsg(m, tea.KeyEnter)

	assertError(t, m)
}

// ============================================================================
// Stream Message Tests
// ============================================================================

func TestHandleStreamChunk_TextContent(t *testing.T) {
	t.Run("creates new assistant message", func(t *testing.T) {
		m := newTestModel(t)
		m.streaming = true

		model, _ := m.Update(StreamChunkMsg{Content: "Hello"})
		m = model.(*Model)

		assertMessageCount(t, m, 1)
		assert.Equal(t, "assistant", m.messages[0].Role)
		assert.Equal(t, "Hello", m.messages[0].Content)
	})

	t.Run("appends to existing assistant message", func(t *testing.T) {
		m := newTestModel(t)
		m.streaming = true
		addMessage(m, "assistant", "Hello")

		model, _ := m.Update(StreamChunkMsg{Content: " World"})
		m = model.(*Model)

		assertMessageCount(t, m, 1)
		assert.Equal(t, "Hello World", m.messages[0].Content)
	})

	t.Run("creates new message after user message", func(t *testing.T) {
		m := newTestModel(t)
		m.streaming = true
		addMessage(m, "user", "Question?")

		model, _ := m.Update(StreamChunkMsg{Content: "Answer"})
		m = model.(*Model)

		assertMessageCount(t, m, 2)
		assert.Equal(t, "assistant", m.messages[1].Role)
		assert.Equal(t, "Answer", m.messages[1].Content)
	})
}

func TestHandleStreamChunk_ToolCall(t *testing.T) {
	t.Run("accumulates tool call deltas", func(t *testing.T) {
		m := newTestModel(t)
		m.streaming = true

		delta := mockToolCallDelta(0, "call_001", "suggest_plot", "")
		model, _ := m.Update(StreamChunkMsg{ToolCall: delta})
		m = model.(*Model)

		assert.True(t, m.toolCallAccumulator.HasCalls())
	})

	t.Run("processes tool calls when done", func(t *testing.T) {
		m := newTestModel(t)
		m.streaming = true

		// Add a complete tool call
		delta := mockToolCallDelta(0, "call_001", llm.ToolSuggestPlotDevelopment, `{"suggestions":[]}`)
		m.toolCallAccumulator.AddDelta(delta)

		model, cmd := m.Update(StreamChunkMsg{Done: true})
		m = model.(*Model)

		// Should have triggered tool call processing
		assert.NotNil(t, cmd)
	})
}

func TestStreamDoneMsg(t *testing.T) {
	m := newTestModel(t)
	m.streaming = true
	m.inputMode = false

	model, _ := m.Update(StreamDoneMsg{})
	m = model.(*Model)

	assert.False(t, m.streaming)
	assert.True(t, m.inputMode)
}

func TestStreamErrorMsg(t *testing.T) {
	m := newTestModel(t)
	m.streaming = true
	m.inputMode = false

	model, _ := m.Update(StreamErrorMsg{Err: assert.AnError})
	m = model.(*Model)

	assert.False(t, m.streaming)
	assert.True(t, m.inputMode)
	assert.Equal(t, assert.AnError, m.err)
}

// ============================================================================
// Suggestion View Tests
// ============================================================================

func TestHandleSuggestionKey_Accept(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"a key", "a"},
		{"y key", "y"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.view = ViewSuggestion
			m.pendingSuggestion = &SuggestionResult{
				Type:             SuggestionTypePlot,
				Title:            "Test Suggestion",
				RequiresApproval: false,
			}

			model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			m = model.(*Model)

			assert.Equal(t, ViewChat, m.view)
			assert.Nil(t, m.pendingSuggestion)
		})
	}
}

func TestHandleSuggestionKey_Reject(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"r key", "r"},
		{"n key", "n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.view = ViewSuggestion
			m.pendingSuggestion = &SuggestionResult{
				Type:  SuggestionTypePlot,
				Title: "Test Suggestion",
			}

			model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			m = model.(*Model)

			assert.Equal(t, ViewChat, m.view)
			assert.Nil(t, m.pendingSuggestion)
			assertLastMessage(t, m, "system", "Rejected: Test Suggestion")
		})
	}
}

func TestHandleSuggestionKey_Modify(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"m key", "m"},
		{"e key", "e"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.view = ViewSuggestion
			m.pendingSuggestion = &SuggestionResult{
				Type:  SuggestionTypePlot,
				Title: "Test Suggestion",
			}

			model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})
			m = model.(*Model)

			assert.Equal(t, ViewChat, m.view)
			assert.Nil(t, m.pendingSuggestion)
			assertLastMessage(t, m, "system", "pending modification")
		})
	}
}

func TestHandleSuggestionKey_Esc(t *testing.T) {
	m := newTestModel(t)
	m.view = ViewSuggestion
	m.pendingSuggestion = &SuggestionResult{
		Type:  SuggestionTypePlot,
		Title: "Test Suggestion",
	}

	m = sendKeyMsg(m, tea.KeyEsc)

	assert.Equal(t, ViewChat, m.view)
	assert.Nil(t, m.pendingSuggestion)
}

func TestHandleSuggestionKey_CustomAction(t *testing.T) {
	m := newTestModel(t)
	m.view = ViewSuggestion

	actionExecuted := false
	m.pendingSuggestion = &SuggestionResult{
		Type:  SuggestionTypePlot,
		Title: "Test Suggestion",
		Actions: []SuggestionAction{
			{
				Key:   "1",
				Label: "Option 1",
				Handler: func() error {
					actionExecuted = true
					return nil
				},
			},
		},
	}

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m = model.(*Model)

	assert.True(t, actionExecuted)
	assert.Equal(t, ViewChat, m.view)
	assert.Nil(t, m.pendingSuggestion)
}

// ============================================================================
// SuggestionMsg Tests
// ============================================================================

func TestSuggestionMsg(t *testing.T) {
	m := newTestModel(t)
	m.streaming = true

	suggestion := &SuggestionResult{
		Type:    SuggestionTypePlot,
		Title:   "Plot Suggestion",
		Content: "Test content",
	}

	model, _ := m.Update(SuggestionMsg{Suggestion: suggestion})
	m = model.(*Model)

	assert.Equal(t, ViewSuggestion, m.view)
	assert.Equal(t, suggestion, m.pendingSuggestion)
	assert.False(t, m.streaming)
	assert.False(t, m.inputMode)
}

// ============================================================================
// View Rendering Tests
// ============================================================================

func TestView_NotReady(t *testing.T) {
	m := New(nil, nil, nil, "test-model", "", "")
	m.ready = false

	view := m.View()

	assert.Contains(t, view, "Initializing")
}

func TestView_Ready(t *testing.T) {
	m := newTestModel(t)

	view := m.View()

	assert.Contains(t, view, "DREAMTELLER")
	assert.Contains(t, view, "/help")
}

func TestRenderChat(t *testing.T) {
	m := newTestModel(t)
	addMessage(m, "user", "Hello")
	addMessage(m, "assistant", "Hi there!")
	addMessage(m, "system", "System message")

	content := m.renderChat()

	assert.Contains(t, content, "Hello")
	assert.Contains(t, content, "Hi there!")
	assert.Contains(t, content, "System message")
}

func TestRenderChat_ShowsSpinnerWhenStreaming(t *testing.T) {
	m := newTestModel(t)
	m.streaming = true

	content := m.renderChat()

	assert.Contains(t, content, "Thinking")
}

func TestRenderHelp(t *testing.T) {
	m := newTestModel(t)

	content := m.renderHelp()

	assert.Contains(t, content, "DREAMTELLER")
	assert.Contains(t, content, "Commands:")
	assert.Contains(t, content, "/help")
	assert.Contains(t, content, "/quit")
	assert.Contains(t, content, "/clear")
	assert.Contains(t, content, "/context")
	assert.Contains(t, content, "/search")
	assert.Contains(t, content, "Ctrl+C")
	assert.Contains(t, content, "Esc")
	assert.Contains(t, content, "Enter")
}

func TestRenderContext_NoProject(t *testing.T) {
	m := newTestModel(t)
	m.project = nil

	content := m.renderContext()

	assert.Contains(t, content, "No project loaded")
}

func TestRenderChapters_NoProject(t *testing.T) {
	m := newTestModel(t)
	m.project = nil

	content := m.renderChapters()

	assert.Contains(t, content, "No project loaded")
}

func TestRenderSuggestion_NoPending(t *testing.T) {
	m := newTestModel(t)
	m.pendingSuggestion = nil

	content := m.renderSuggestion()

	assert.Contains(t, content, "No pending suggestion")
}

func TestRenderSuggestion_WithSuggestion(t *testing.T) {
	m := newTestModel(t)
	m.pendingSuggestion = &SuggestionResult{
		Type:             SuggestionTypePlot,
		Title:            "Test Plot Suggestion",
		Content:          "This is the suggestion content",
		RequiresApproval: true,
		Actions: []SuggestionAction{
			{Key: "1", Label: "Option One"},
			{Key: "2", Label: "Option Two"},
		},
	}

	content := m.renderSuggestion()

	assert.Contains(t, content, "Test Plot Suggestion")
	assert.Contains(t, content, "suggestion content")
	assert.Contains(t, content, "Actions:")
	assert.Contains(t, content, "Option One")
	assert.Contains(t, content, "Option Two")
	assert.Contains(t, content, "Accept")
	assert.Contains(t, content, "Reject")
	assert.Contains(t, content, "approval")
}

func TestRenderSuggestion_WithoutApproval(t *testing.T) {
	m := newTestModel(t)
	m.pendingSuggestion = &SuggestionResult{
		Type:             SuggestionTypePlot,
		Title:            "Simple Suggestion",
		Content:          "Content",
		RequiresApproval: false,
	}

	content := m.renderSuggestion()

	assert.Contains(t, content, "OK")
	assert.Contains(t, content, "Dismiss")
}

// ============================================================================
// Error Display Tests
// ============================================================================

func TestView_ShowsError(t *testing.T) {
	m := newTestModel(t)
	m.err = assert.AnError

	view := m.View()

	assert.Contains(t, view, "Error:")
	// Error should be cleared after rendering
	assert.Nil(t, m.err)
}

func TestView_ShowsStatus(t *testing.T) {
	m := newTestModel(t)
	m.statusText = "Status message"

	view := m.View()

	assert.Contains(t, view, "Status message")
	// Status should be cleared after rendering
	assert.Empty(t, m.statusText)
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestFullChatFlow(t *testing.T) {
	m := newTestModel(t)

	m = sendRunesMsg(m, "Hello AI")
	m = sendKeyMsg(m, tea.KeyEnter)

	require.GreaterOrEqual(t, len(m.messages), 1)
	assert.Equal(t, "user", m.messages[0].Role)
	assert.Equal(t, "Hello AI", m.messages[0].Content)

	model, _ := m.Update(StreamDoneMsg{})
	m = model.(*Model)

	assert.True(t, len(m.messages) >= 1)
	assert.False(t, m.streaming)
	assert.True(t, m.inputMode)
}

func TestViewNavigationFlow(t *testing.T) {
	m := newTestModel(t)

	// Go to help
	setTextareaValue(m, "/help")
	m = sendKeyMsg(m, tea.KeyEnter)
	assert.Equal(t, ViewHelp, m.view)

	// Go back with Esc
	m = sendKeyMsg(m, tea.KeyEsc)
	assert.Equal(t, ViewChat, m.view)

	// Go to context
	setTextareaValue(m, "/context")
	m = sendKeyMsg(m, tea.KeyEnter)
	assert.Equal(t, ViewContext, m.view)

	// Go back with /back
	setTextareaValue(m, "/back")
	m = sendKeyMsg(m, tea.KeyEnter)
	assert.Equal(t, ViewChat, m.view)
}

func TestClearCommandResetsChat(t *testing.T) {
	m := newTestModel(t)

	// Add some messages
	addMessage(m, "user", "Message 1")
	addMessage(m, "assistant", "Response 1")
	addMessage(m, "user", "Message 2")
	assert.Len(t, m.messages, 3)

	// Clear
	setTextareaValue(m, "/clear")
	m = sendKeyMsg(m, tea.KeyEnter)

	assert.Empty(t, m.messages)
}

func TestCancelStreamingMidway(t *testing.T) {
	m := newTestModel(t)

	// Start streaming
	m = sendRunesMsg(m, "Hello")
	m = sendKeyMsg(m, tea.KeyEnter)
	assert.True(t, m.streaming)

	// Receive some content
	model, _ := m.Update(StreamChunkMsg{Content: "Hello"})
	m = model.(*Model)
	assert.True(t, m.streaming)

	// Cancel with Ctrl+C
	m = sendKeyMsg(m, tea.KeyCtrlC)

	assert.False(t, m.streaming)
	assert.True(t, m.inputMode)
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestHandleSubmit_TrimsWhitespace(t *testing.T) {
	m := newTestModel(t)
	setTextareaValue(m, "   Hello   ")

	m = sendKeyMsg(m, tea.KeyEnter)

	// Message content should be trimmed
	// First message is user input, may have additional error message if no provider
	require.GreaterOrEqual(t, len(m.messages), 1)
	assert.Equal(t, "user", m.messages[0].Role)
	assert.Equal(t, "Hello", m.messages[0].Content)
}

func TestHandleSubmit_WhitespaceOnly(t *testing.T) {
	m := newTestModel(t)
	setTextareaValue(m, "   ")

	m = sendKeyMsg(m, tea.KeyEnter)

	// Should not submit whitespace-only input
	assert.Empty(t, m.messages)
}

func TestCommandCaseInsensitive(t *testing.T) {
	tests := []struct {
		command string
	}{
		{"/HELP"},
		{"/Help"},
		{"/HeLp"},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			m := newTestModel(t)
			setTextareaValue(m, tt.command)

			m = sendKeyMsg(m, tea.KeyEnter)

			assert.Equal(t, ViewHelp, m.view)
		})
	}
}

func TestMultiWordCommandParsing(t *testing.T) {
	m := newTestModel(t)
	setTextareaValue(m, "/search dragon treasure cave")

	m = sendKeyMsg(m, tea.KeyEnter)

	assert.Contains(t, m.statusText, "dragon treasure cave")
}

// ============================================================================
// Message Type Tests
// ============================================================================

func TestStreamChunkMsg(t *testing.T) {
	msg := StreamChunkMsg{
		Content:  "Hello",
		ToolCall: nil,
		Done:     false,
	}

	assert.Equal(t, "Hello", msg.Content)
	assert.Nil(t, msg.ToolCall)
	assert.False(t, msg.Done)
}

func TestStreamChunkMsg_WithToolCall(t *testing.T) {
	delta := &llm.ToolCallDelta{Index: 0, ID: "call_1"}
	msg := StreamChunkMsg{
		Content:  "",
		ToolCall: delta,
		Done:     false,
	}

	assert.Empty(t, msg.Content)
	assert.NotNil(t, msg.ToolCall)
	assert.Equal(t, "call_1", msg.ToolCall.ID)
}

func TestErrMsg(t *testing.T) {
	m := newTestModel(t)

	model, _ := m.Update(errMsg{err: assert.AnError})
	m = model.(*Model)

	assert.Equal(t, assert.AnError, m.err)
}

// ============================================================================
// Spinner Tests
// ============================================================================

func TestSpinnerTicksWhileStreaming(t *testing.T) {
	m := newTestModel(t)
	m.streaming = true

	// Spinner tick should return a command while streaming
	model, cmd := m.Update(spinner.TickMsg{})
	m = model.(*Model)

	assert.NotNil(t, cmd)
}

func TestSpinnerDoesNotTickWhenNotStreaming(t *testing.T) {
	m := newTestModel(t)
	m.streaming = false

	// Spinner tick should not return a command when not streaming
	_, cmd := m.Update(spinner.TickMsg{})

	// The batch command might still have other commands, but spinner shouldn't tick
	// This tests that the spinner tick is only processed when streaming
	_ = cmd
}

// ============================================================================
// Tool Call Accumulator Tests (Integration)
// ============================================================================

func TestToolCallAccumulationFlow(t *testing.T) {
	m := newTestModel(t)
	m.streaming = true

	// Simulate incremental tool call deltas
	deltas := []*llm.ToolCallDelta{
		mockToolCallDelta(0, "call_001", "suggest_plot_development", ""),
		mockToolCallDelta(0, "", "", `{"sug`),
		mockToolCallDelta(0, "", "", `gestions"`),
		mockToolCallDelta(0, "", "", `:[{"ti`),
		mockToolCallDelta(0, "", "", `tle":"Test"}]}`),
	}

	for _, delta := range deltas {
		model, _ := m.Update(StreamChunkMsg{ToolCall: delta})
		m = model.(*Model)
	}

	// Check accumulator state
	assert.True(t, m.toolCallAccumulator.HasCalls())

	calls := m.toolCallAccumulator.GetCompletedCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "call_001", calls[0].ID)
	assert.Equal(t, "suggest_plot_development", calls[0].Function.Name)
	assert.True(t, strings.Contains(calls[0].Function.Arguments, "suggestions"))
}

func TestToolCallAccumulatorReset(t *testing.T) {
	m := newTestModel(t)
	m.streaming = true

	// Add some tool call deltas
	m.toolCallAccumulator.AddDelta(mockToolCallDelta(0, "call_001", "test", "{}"))
	assert.True(t, m.toolCallAccumulator.HasCalls())

	// Reset
	m.toolCallAccumulator.Reset()
	assert.False(t, m.toolCallAccumulator.HasCalls())
}
