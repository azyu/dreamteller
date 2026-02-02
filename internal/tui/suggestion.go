// Package tui provides the terminal user interface using Bubble Tea.
package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/azyu/dreamteller/internal/llm"
	"github.com/azyu/dreamteller/internal/project"
	"github.com/azyu/dreamteller/internal/search"
	"github.com/azyu/dreamteller/internal/storage"
	"github.com/azyu/dreamteller/internal/tui/styles"
)

// SuggestionType identifies the kind of suggestion.
type SuggestionType string

const (
	SuggestionTypePlot            SuggestionType = "plot"
	SuggestionTypeCharacterAction SuggestionType = "character_action"
	SuggestionTypeClarification   SuggestionType = "clarification"
	SuggestionTypeContextUpdate   SuggestionType = "context_update"
	SuggestionTypeSearch          SuggestionType = "search"
)

// SuggestionAction represents an action the user can take on a suggestion.
type SuggestionAction struct {
	Label   string
	Key     string
	Handler func() error
}

// SuggestionResult holds the processed result of a tool call.
type SuggestionResult struct {
	Type             SuggestionType
	Title            string
	Content          string
	Actions          []SuggestionAction
	RequiresApproval bool
	ToolCallID       string
	ToolCall         llm.ToolCall
	ParsedData       interface{}
}

// SuggestionHandler processes AI tool calls and prepares them for display.
type SuggestionHandler struct {
	project      *project.Project
	searchEngine *search.FTSEngine
}

// NewSuggestionHandler creates a new suggestion handler.
func NewSuggestionHandler(proj *project.Project, searchEngine *search.FTSEngine) *SuggestionHandler {
	return &SuggestionHandler{
		project:      proj,
		searchEngine: searchEngine,
	}
}

// HandleToolCall processes a tool call and returns a displayable result.
func (h *SuggestionHandler) HandleToolCall(call llm.ToolCall) (*SuggestionResult, error) {
	parsed, err := llm.ParseToolCall(call)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tool call: %w", err)
	}

	switch call.Function.Name {
	case llm.ToolSuggestPlotDevelopment:
		suggestions, ok := parsed.([]llm.PlotSuggestion)
		if !ok {
			return nil, fmt.Errorf("unexpected type for plot suggestions")
		}
		return h.handlePlotSuggestion(call, suggestions)

	case llm.ToolSuggestCharacterAction:
		suggestion, ok := parsed.(llm.CharacterActionSuggestion)
		if !ok {
			return nil, fmt.Errorf("unexpected type for character action")
		}
		return h.handleCharacterAction(call, suggestion)

	case llm.ToolAskUserClarification:
		question, ok := parsed.(llm.ClarificationQuestion)
		if !ok {
			return nil, fmt.Errorf("unexpected type for clarification")
		}
		return h.handleClarification(call, question)

	case llm.ToolUpdateContext:
		update, ok := parsed.(llm.ContextUpdate)
		if !ok {
			return nil, fmt.Errorf("unexpected type for context update")
		}
		return h.handleContextUpdate(call, update)

	case llm.ToolSearchContext:
		query, ok := parsed.(llm.SearchQuery)
		if !ok {
			return nil, fmt.Errorf("unexpected type for search query")
		}
		return h.handleSearch(call, query)

	default:
		return nil, fmt.Errorf("unknown tool: %s", call.Function.Name)
	}
}

// handlePlotSuggestion formats plot development suggestions for display.
func (h *SuggestionHandler) handlePlotSuggestion(call llm.ToolCall, suggestions []llm.PlotSuggestion) (*SuggestionResult, error) {
	var sb strings.Builder

	for i, s := range suggestions {
		sb.WriteString(styles.Subtitle.Render(fmt.Sprintf("%d. %s", i+1, s.Title)))
		sb.WriteString("\n")
		sb.WriteString(styles.MutedText.Render(s.Description))
		sb.WriteString("\n")

		if s.Impact != "" {
			sb.WriteString(styles.InfoText.Render(fmt.Sprintf("   Impact: %s", s.Impact)))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	actions := make([]SuggestionAction, len(suggestions))
	for i, sugg := range suggestions {
		selectedSugg := sugg
		actions[i] = SuggestionAction{
			Label: fmt.Sprintf("Use suggestion %d", i+1),
			Key:   fmt.Sprintf("%d", i+1),
			Handler: func() error {
				// Handler returns nil; caller uses selectedSugg to continue
				_ = selectedSugg
				return nil
			},
		}
	}

	return &SuggestionResult{
		Type:             SuggestionTypePlot,
		Title:            "Plot Development Suggestions",
		Content:          sb.String(),
		Actions:          actions,
		RequiresApproval: false,
		ToolCallID:       call.ID,
		ToolCall:         call,
		ParsedData:       suggestions,
	}, nil
}

// handleCharacterAction formats character action suggestions for display.
func (h *SuggestionHandler) handleCharacterAction(call llm.ToolCall, suggestion llm.CharacterActionSuggestion) (*SuggestionResult, error) {
	var sb strings.Builder

	sb.WriteString(styles.Title.Render(fmt.Sprintf("Suggestions for %s", suggestion.Character)))
	sb.WriteString("\n\n")

	for i, action := range suggestion.Actions {
		sb.WriteString(styles.Subtitle.Render(fmt.Sprintf("%d. %s", i+1, action.Action)))
		sb.WriteString("\n")
		sb.WriteString(styles.MutedText.Render(fmt.Sprintf("   Motivation: %s", action.Motivation)))
		sb.WriteString("\n")

		if action.Dialogue != "" {
			sb.WriteString(styles.Quote.Render(fmt.Sprintf("   \"%s\"", action.Dialogue)))
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	actions := make([]SuggestionAction, len(suggestion.Actions))
	for i, act := range suggestion.Actions {
		selectedAct := act
		actions[i] = SuggestionAction{
			Label: fmt.Sprintf("Use action %d", i+1),
			Key:   fmt.Sprintf("%d", i+1),
			Handler: func() error {
				_ = selectedAct
				return nil
			},
		}
	}

	return &SuggestionResult{
		Type:             SuggestionTypeCharacterAction,
		Title:            fmt.Sprintf("Character Actions: %s", suggestion.Character),
		Content:          sb.String(),
		Actions:          actions,
		RequiresApproval: false,
		ToolCallID:       call.ID,
		ToolCall:         call,
		ParsedData:       suggestion,
	}, nil
}

// handleClarification formats a clarification question for display.
func (h *SuggestionHandler) handleClarification(call llm.ToolCall, question llm.ClarificationQuestion) (*SuggestionResult, error) {
	var sb strings.Builder

	sb.WriteString(styles.Title.Render("Question"))
	sb.WriteString("\n\n")
	sb.WriteString(question.Question)
	sb.WriteString("\n")

	if question.Context != "" {
		sb.WriteString("\n")
		sb.WriteString(styles.MutedText.Render(fmt.Sprintf("Context: %s", question.Context)))
		sb.WriteString("\n")
	}

	var actions []SuggestionAction

	if len(question.Options) > 0 {
		sb.WriteString("\n")
		sb.WriteString(styles.Subtitle.Render("Options:"))
		sb.WriteString("\n")

		for i, opt := range question.Options {
			key := string(rune('a' + i))
			sb.WriteString(fmt.Sprintf("  [%s] %s\n", key, opt))

			selectedOpt := opt
			actions = append(actions, SuggestionAction{
				Label: selectedOpt,
				Key:   key,
				Handler: func() error {
					_ = selectedOpt
					return nil
				},
			})
		}
	}

	return &SuggestionResult{
		Type:             SuggestionTypeClarification,
		Title:            "Clarification Needed",
		Content:          sb.String(),
		Actions:          actions,
		RequiresApproval: false,
		ToolCallID:       call.ID,
		ToolCall:         call,
		ParsedData:       question,
	}, nil
}

// handleContextUpdate validates and formats a context update for approval.
func (h *SuggestionHandler) handleContextUpdate(call llm.ToolCall, update llm.ContextUpdate) (*SuggestionResult, error) {
	// Validate the path for security
	if err := llm.ValidateContextUpdatePath(update.FileType, update.FileName); err != nil {
		return nil, fmt.Errorf("invalid context update path: %w", err)
	}

	var sb strings.Builder

	// Format the header
	operationLabel := formatOperation(update.Operation)
	sb.WriteString(styles.Title.Render(fmt.Sprintf("%s: %s/%s.md", operationLabel, update.FileType, update.FileName)))
	sb.WriteString("\n\n")

	// Show the reason
	sb.WriteString(styles.InfoText.Render("Reason: "))
	sb.WriteString(update.Reason)
	sb.WriteString("\n\n")

	// Build the file path for diff preview
	relativePath := filepath.Join("context", pluralizeFileType(update.FileType), update.FileName+".md")

	// Show diff preview based on operation
	switch update.Operation {
	case "create":
		sb.WriteString(styles.SuccessText.Render("+ New file will be created"))
		sb.WriteString("\n\n")
		sb.WriteString(formatContentPreview(update.Content, "+"))

	case "update":
		existingContent, err := h.readExistingContent(relativePath)
		if err != nil {
			sb.WriteString(styles.ErrorText.Render(fmt.Sprintf("Warning: Could not read existing file: %v", err)))
			sb.WriteString("\n\n")
		} else {
			sb.WriteString(styles.MutedText.Render("Current content:"))
			sb.WriteString("\n")
			sb.WriteString(formatContentPreview(existingContent, "-"))
			sb.WriteString("\n")
		}
		sb.WriteString(styles.SuccessText.Render("New content:"))
		sb.WriteString("\n")
		sb.WriteString(formatContentPreview(update.Content, "+"))

	case "append":
		sb.WriteString(styles.MutedText.Render("Content to append:"))
		sb.WriteString("\n")
		sb.WriteString(formatContentPreview(update.Content, "+"))
	}

	updateCopy := update
	actions := []SuggestionAction{
		{
			Label: "Accept",
			Key:   "a",
			Handler: func() error {
				return h.ExecuteContextUpdate(updateCopy)
			},
		},
		{
			Label: "Reject",
			Key:   "r",
			Handler: func() error {
				return nil
			},
		},
		{
			Label: "Edit before saving",
			Key:   "e",
			Handler: func() error {
				return nil
			},
		},
	}

	return &SuggestionResult{
		Type:             SuggestionTypeContextUpdate,
		Title:            fmt.Sprintf("Context Update: %s", update.FileName),
		Content:          sb.String(),
		Actions:          actions,
		RequiresApproval: true,
		ToolCallID:       call.ID,
		ToolCall:         call,
		ParsedData:       update,
	}, nil
}

// handleSearch executes a search query and formats the results.
func (h *SuggestionHandler) handleSearch(call llm.ToolCall, query llm.SearchQuery) (*SuggestionResult, error) {
	if h.searchEngine == nil {
		return nil, fmt.Errorf("search engine not initialized")
	}

	var results []search.FTSSearchResult
	var err error

	// Execute search based on filter type
	if query.FilterType != "" && query.FilterType != "all" {
		results, err = h.searchEngine.SearchWithFilter(query.Query, query.FilterType, 10)
	} else {
		results, err = h.searchEngine.Search(query.Query, 10)
	}

	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	var sb strings.Builder

	sb.WriteString(styles.Title.Render(fmt.Sprintf("Search: \"%s\"", query.Query)))
	sb.WriteString("\n")

	if query.FilterType != "" && query.FilterType != "all" {
		sb.WriteString(styles.MutedText.Render(fmt.Sprintf("Filtered by: %s", query.FilterType)))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	if len(results) == 0 {
		sb.WriteString(styles.MutedText.Render("No results found."))
		sb.WriteString("\n")
	} else {
		sb.WriteString(styles.InfoText.Render(fmt.Sprintf("Found %d result(s):", len(results))))
		sb.WriteString("\n\n")

		for i, result := range results {
			sb.WriteString(styles.Subtitle.Render(fmt.Sprintf("%d. [%s] %s", i+1, result.SourceType, result.SourcePath)))
			sb.WriteString("\n")

			// Show a snippet of the content (first 200 chars)
			snippet := truncateContent(result.Content, 200)
			sb.WriteString(styles.MutedText.Render(fmt.Sprintf("   %s", snippet)))
			sb.WriteString("\n\n")
		}
	}

	return &SuggestionResult{
		Type:             SuggestionTypeSearch,
		Title:            "Search Results",
		Content:          sb.String(),
		Actions:          nil, // Search results don't have actions
		RequiresApproval: false,
		ToolCallID:       call.ID,
		ToolCall:         call,
		ParsedData:       results,
	}, nil
}

// ExecuteContextUpdate applies the context update after user approval.
func (h *SuggestionHandler) ExecuteContextUpdate(update llm.ContextUpdate) error {
	// Re-validate for safety
	if err := llm.ValidateContextUpdatePath(update.FileType, update.FileName); err != nil {
		return fmt.Errorf("invalid context update path: %w", err)
	}

	if h.project == nil {
		return fmt.Errorf("no project loaded")
	}

	category := pluralizeFileType(update.FileType)
	relativePath := filepath.Join("context", category, update.FileName+".md")

	switch update.Operation {
	case "create":
		return h.createContextFile(relativePath, update.Content)

	case "update":
		return h.updateContextFile(relativePath, update.Content)

	case "append":
		return h.appendToContextFile(relativePath, update.Content)

	default:
		return fmt.Errorf("unknown operation: %s", update.Operation)
	}
}

// createContextFile creates a new context file.
func (h *SuggestionHandler) createContextFile(relativePath, content string) error {
	fullPath := filepath.Join(h.project.Path(), relativePath)

	// Check if file already exists
	if _, err := os.Stat(fullPath); err == nil {
		return fmt.Errorf("file already exists: %s", relativePath)
	}

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file atomically
	return storage.AtomicWriteFile(fullPath, []byte(content))
}

// updateContextFile replaces the content of an existing context file.
func (h *SuggestionHandler) updateContextFile(relativePath, content string) error {
	fullPath := filepath.Join(h.project.Path(), relativePath)

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", relativePath)
	}

	// Write file atomically
	return storage.AtomicWriteFile(fullPath, []byte(content))
}

// appendToContextFile appends content to an existing context file.
func (h *SuggestionHandler) appendToContextFile(relativePath, content string) error {
	fullPath := filepath.Join(h.project.Path(), relativePath)

	// Read existing content
	existing, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			// If file doesn't exist, create it
			return h.createContextFile(relativePath, content)
		}
		return fmt.Errorf("failed to read existing file: %w", err)
	}

	// Append new content with a separator
	newContent := string(existing)
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += "\n" + content

	// Write file atomically
	return storage.AtomicWriteFile(fullPath, []byte(newContent))
}

// readExistingContent reads the content of an existing context file.
func (h *SuggestionHandler) readExistingContent(relativePath string) (string, error) {
	if h.project == nil {
		return "", fmt.Errorf("no project loaded")
	}

	return h.project.FS.ReadMarkdown(relativePath)
}

// Helper functions

// formatOperation returns a human-readable label for the operation.
func formatOperation(op string) string {
	switch op {
	case "create":
		return "Create"
	case "update":
		return "Update"
	case "append":
		return "Append to"
	default:
		return strings.ToUpper(op[:1]) + op[1:]
	}
}

// pluralizeFileType converts singular file types to their plural directory names.
func pluralizeFileType(fileType string) string {
	switch fileType {
	case "character":
		return "characters"
	case "setting":
		return "settings"
	case "plot":
		return "plot"
	default:
		return fileType
	}
}

// formatContentPreview formats content with line prefixes for diff-style display.
func formatContentPreview(content string, prefix string) string {
	lines := strings.Split(content, "\n")
	maxLines := 20

	var sb strings.Builder
	for i, line := range lines {
		if i >= maxLines {
			remaining := len(lines) - maxLines
			sb.WriteString(styles.MutedText.Render(fmt.Sprintf("  ... (%d more lines)", remaining)))
			break
		}

		if prefix == "+" {
			sb.WriteString(styles.SuccessText.Render(fmt.Sprintf("  %s %s", prefix, line)))
		} else if prefix == "-" {
			sb.WriteString(styles.ErrorText.Render(fmt.Sprintf("  %s %s", prefix, line)))
		} else {
			sb.WriteString(fmt.Sprintf("  %s %s", prefix, line))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// truncateContent truncates content to a maximum length with ellipsis.
func truncateContent(content string, maxLen int) string {
	// Replace newlines with spaces for snippet display
	content = strings.ReplaceAll(content, "\n", " ")
	content = strings.ReplaceAll(content, "\r", "")

	// Collapse multiple spaces
	for strings.Contains(content, "  ") {
		content = strings.ReplaceAll(content, "  ", " ")
	}

	content = strings.TrimSpace(content)

	if len(content) <= maxLen {
		return content
	}

	return content[:maxLen-3] + "..."
}

// SuggestionMsg is sent when a suggestion is ready for display.
type SuggestionMsg struct {
	Suggestion *SuggestionResult
}

// ToolCallAccumulator accumulates streaming tool call deltas.
type ToolCallAccumulator struct {
	calls map[int]*accumulatedCall
}

type accumulatedCall struct {
	id        string
	callType  string
	name      string
	arguments strings.Builder
}

// NewToolCallAccumulator creates a new tool call accumulator.
func NewToolCallAccumulator() *ToolCallAccumulator {
	return &ToolCallAccumulator{
		calls: make(map[int]*accumulatedCall),
	}
}

// AddDelta adds a tool call delta to the accumulator.
func (a *ToolCallAccumulator) AddDelta(delta *llm.ToolCallDelta) {
	if delta == nil {
		return
	}

	call, exists := a.calls[delta.Index]
	if !exists {
		call = &accumulatedCall{}
		a.calls[delta.Index] = call
	}

	if delta.ID != "" {
		call.id = delta.ID
	}
	if delta.Type != "" {
		call.callType = delta.Type
	}
	if delta.Function != nil {
		if delta.Function.Name != "" {
			call.name = delta.Function.Name
		}
		call.arguments.WriteString(delta.Function.Arguments)
	}
}

// GetCompletedCalls returns all accumulated tool calls.
func (a *ToolCallAccumulator) GetCompletedCalls() []llm.ToolCall {
	result := make([]llm.ToolCall, 0, len(a.calls))
	for _, call := range a.calls {
		result = append(result, llm.ToolCall{
			ID:   call.id,
			Type: call.callType,
			Function: llm.FunctionCall{
				Name:      call.name,
				Arguments: call.arguments.String(),
			},
		})
	}
	return result
}

// Reset clears the accumulator.
func (a *ToolCallAccumulator) Reset() {
	a.calls = make(map[int]*accumulatedCall)
}

// HasCalls returns true if there are accumulated calls.
func (a *ToolCallAccumulator) HasCalls() bool {
	return len(a.calls) > 0
}
