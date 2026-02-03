package tui

import (
	"testing"
	"time"

	"github.com/azyu/dreamteller/internal/llm"
	"github.com/azyu/dreamteller/internal/project"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Disable colors for consistent test output across environments
	lipgloss.SetColorProfile(termenv.Ascii)
}

// testConfig holds common test configuration values.
var testConfig = struct {
	Width   int
	Height  int
	Timeout time.Duration
}{
	Width:   80,
	Height:  24,
	Timeout: 5 * time.Second,
}

// newTestModel creates a new TUI model for testing with default dimensions.
func newTestModel(t *testing.T) *Model {
	t.Helper()

	m := New(nil, nil, nil, "test-model", "", "")
	m.ready = true
	m.width = testConfig.Width
	m.height = testConfig.Height
	m.viewport = viewport.New(testConfig.Width, testConfig.Height-8)
	m.viewport.YPosition = 2
	m.textarea.SetWidth(testConfig.Width - 4)

	return m
}

// newTestModelWithProject creates a test model with a mock project.
func newTestModelWithProject(t *testing.T, proj *project.Project) *Model {
	t.Helper()

	m := New(proj, nil, nil, "test-model", "", "")
	m.ready = true
	m.width = testConfig.Width
	m.height = testConfig.Height
	m.viewport = viewport.New(testConfig.Width, testConfig.Height-8)
	m.viewport.YPosition = 2
	m.textarea.SetWidth(testConfig.Width - 4)

	return m
}

// sendKeyMsg sends a key message to the model and returns the updated model.
func sendKeyMsg(m *Model, keyType tea.KeyType) *Model {
	model, _ := m.Update(tea.KeyMsg{Type: keyType})
	return model.(*Model)
}

// sendRunesMsg sends runes (typed text) to the model and returns the updated model.
func sendRunesMsg(m *Model, s string) *Model {
	for _, r := range s {
		model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = model.(*Model)
	}
	return m
}

// sendWindowSize sends a window size message to the model.
func sendWindowSize(m *Model, width, height int) *Model {
	model, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return model.(*Model)
}

// typeAndSubmit types text and sends Enter to submit.
func typeAndSubmit(m *Model, text string) (*Model, tea.Cmd) {
	m = sendRunesMsg(m, text)
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return model.(*Model), cmd
}

// getTextareaValue returns the current value in the textarea.
func getTextareaValue(m *Model) string {
	return m.textarea.Value()
}

// setTextareaValue sets the textarea value directly (for test setup).
func setTextareaValue(m *Model, s string) {
	m.textarea.Reset()
	m.textarea.SetValue(s)
}

// addMessage adds a message to the model's message list.
func addMessage(m *Model, role, content string) {
	m.messages = append(m.messages, Message{Role: role, Content: content})
}

// mockToolCall creates a mock tool call for testing.
func mockToolCall(name, arguments string) llm.ToolCall {
	return llm.ToolCall{
		ID:   "test_call_001",
		Type: "function",
		Function: llm.FunctionCall{
			Name:      name,
			Arguments: arguments,
		},
	}
}

// mockToolCallDelta creates a mock tool call delta for streaming tests.
func mockToolCallDelta(index int, id, name, args string) *llm.ToolCallDelta {
	delta := &llm.ToolCallDelta{
		Index: index,
		ID:    id,
		Type:  "function",
	}
	if name != "" || args != "" {
		delta.Function = &llm.FunctionCallDelta{
			Name:      name,
			Arguments: args,
		}
	}
	return delta
}

// assertViewState checks that the model is in the expected view state.
func assertViewState(t *testing.T, m *Model, expected ViewState) {
	t.Helper()
	if m.view != expected {
		t.Errorf("expected view state %v, got %v", expected, m.view)
	}
}

// assertInputMode checks whether the model is in input mode.
func assertInputMode(t *testing.T, m *Model, expected bool) {
	t.Helper()
	if m.inputMode != expected {
		t.Errorf("expected inputMode=%v, got %v", expected, m.inputMode)
	}
}

// assertStreaming checks whether the model is streaming.
func assertStreaming(t *testing.T, m *Model, expected bool) {
	t.Helper()
	if m.streaming != expected {
		t.Errorf("expected streaming=%v, got %v", expected, m.streaming)
	}
}

// assertMessageCount checks the number of messages in the model.
func assertMessageCount(t *testing.T, m *Model, expected int) {
	t.Helper()
	if len(m.messages) != expected {
		t.Errorf("expected %d messages, got %d", expected, len(m.messages))
	}
}

// assertError checks that the model has an error.
func assertError(t *testing.T, m *Model) {
	t.Helper()
	if m.err == nil {
		t.Error("expected error, got nil")
	}
}

// assertNoError checks that the model has no error.
func assertNoError(t *testing.T, m *Model) {
	t.Helper()
	if m.err != nil {
		t.Errorf("expected no error, got %v", m.err)
	}
}

// assertContainsMessage checks that messages contain specific content.
func assertContainsMessage(t *testing.T, m *Model, role, content string) {
	t.Helper()
	for _, msg := range m.messages {
		if msg.Role == role && msg.Content == content {
			return
		}
	}
	t.Errorf("message with role=%q and content=%q not found", role, content)
}

// assertLastMessage checks the last message in the list.
func assertLastMessage(t *testing.T, m *Model, role, contentSubstring string) {
	t.Helper()
	if len(m.messages) == 0 {
		t.Error("expected messages, got none")
		return
	}
	last := m.messages[len(m.messages)-1]
	if last.Role != role {
		t.Errorf("expected last message role=%q, got %q", role, last.Role)
	}
	if contentSubstring != "" && !contains(last.Content, contentSubstring) {
		t.Errorf("expected last message content to contain %q, got %q", contentSubstring, last.Content)
	}
}

// contains checks if substr is in s.
func contains(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// resetTextarea is a helper to reset textarea state.
func resetTextarea(m *Model) {
	m.textarea = textarea.New()
	m.textarea.Placeholder = "Enter your message... (/help for commands)"
	m.textarea.Focus()
	m.textarea.CharLimit = 4000
	m.textarea.SetWidth(testConfig.Width - 4)
	m.textarea.SetHeight(3)
	m.textarea.ShowLineNumbers = false
	m.textarea.KeyMap.InsertNewline.SetEnabled(false)
}

// makeTestMessages creates a slice of test messages.
func makeTestMessages(pairs ...string) []Message {
	if len(pairs)%2 != 0 {
		panic("makeTestMessages requires role/content pairs")
	}
	messages := make([]Message, 0, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		messages = append(messages, Message{
			Role:    pairs[i],
			Content: pairs[i+1],
		})
	}
	return messages
}
