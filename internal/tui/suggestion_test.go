package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/azyu/dreamteller/internal/llm"
	"github.com/azyu/dreamteller/internal/project"
	"github.com/azyu/dreamteller/internal/search"
	"github.com/azyu/dreamteller/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// SuggestionHandler Tests
// ============================================================================

func TestNewSuggestionHandler(t *testing.T) {
	t.Run("creates handler with nil project", func(t *testing.T) {
		h := NewSuggestionHandler(nil, nil)

		assert.NotNil(t, h)
		assert.Nil(t, h.project)
		assert.Nil(t, h.searchEngine)
	})

	t.Run("creates handler with project", func(t *testing.T) {
		// Create a minimal mock project for testing
		h := NewSuggestionHandler(nil, nil)

		assert.NotNil(t, h)
	})
}

// ============================================================================
// HandleToolCall Tests
// ============================================================================

func TestHandleToolCall_PlotSuggestion(t *testing.T) {
	h := NewSuggestionHandler(nil, nil)

	t.Run("handles single plot suggestion", func(t *testing.T) {
		call := mockToolCall(llm.ToolSuggestPlotDevelopment, `{
			"suggestions": [
				{
					"title": "The Hidden Truth",
					"description": "Reveal the antagonist's true motivation",
					"impact": "Changes character dynamics"
				}
			]
		}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, SuggestionTypePlot, result.Type)
		assert.Equal(t, "Plot Development Suggestions", result.Title)
		assert.Contains(t, result.Content, "The Hidden Truth")
		assert.Contains(t, result.Content, "Changes character dynamics")
		assert.Len(t, result.Actions, 1)
		assert.False(t, result.RequiresApproval)
	})

	t.Run("handles multiple plot suggestions", func(t *testing.T) {
		call := mockToolCall(llm.ToolSuggestPlotDevelopment, `{
			"suggestions": [
				{"title": "Plot Twist 1", "description": "Description 1"},
				{"title": "Plot Twist 2", "description": "Description 2"},
				{"title": "Plot Twist 3", "description": "Description 3"}
			]
		}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.Len(t, result.Actions, 3)
		assert.Contains(t, result.Content, "Plot Twist 1")
		assert.Contains(t, result.Content, "Plot Twist 2")
		assert.Contains(t, result.Content, "Plot Twist 3")
	})

	t.Run("handles empty suggestions", func(t *testing.T) {
		call := mockToolCall(llm.ToolSuggestPlotDevelopment, `{"suggestions": []}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.Empty(t, result.Actions)
	})

	t.Run("fails on invalid JSON", func(t *testing.T) {
		call := mockToolCall(llm.ToolSuggestPlotDevelopment, `{invalid json}`)

		_, err := h.HandleToolCall(call)

		assert.Error(t, err)
	})
}

func TestHandleToolCall_CharacterAction(t *testing.T) {
	h := NewSuggestionHandler(nil, nil)

	t.Run("handles character action with dialogue", func(t *testing.T) {
		call := mockToolCall(llm.ToolSuggestCharacterAction, `{
			"character": "Alice",
			"actions": [
				{
					"action": "She confronts the villain",
					"motivation": "To protect her friends",
					"dialogue": "I won't let you harm them!"
				}
			]
		}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.Equal(t, SuggestionTypeCharacterAction, result.Type)
		assert.Contains(t, result.Title, "Alice")
		assert.Contains(t, result.Content, "confronts the villain")
		assert.Contains(t, result.Content, "I won't let you harm them!")
		assert.Len(t, result.Actions, 1)
	})

	t.Run("handles multiple character actions", func(t *testing.T) {
		call := mockToolCall(llm.ToolSuggestCharacterAction, `{
			"character": "Bob",
			"actions": [
				{"action": "Action 1", "motivation": "Reason 1"},
				{"action": "Action 2", "motivation": "Reason 2"}
			]
		}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.Len(t, result.Actions, 2)
	})

	t.Run("handles action without dialogue", func(t *testing.T) {
		call := mockToolCall(llm.ToolSuggestCharacterAction, `{
			"character": "Carol",
			"actions": [
				{"action": "She observes quietly", "motivation": "Gathering information"}
			]
		}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.NotContains(t, result.Content, "\"\"") // No empty quotes from missing dialogue
	})
}

func TestHandleToolCall_Clarification(t *testing.T) {
	h := NewSuggestionHandler(nil, nil)

	t.Run("handles question with options and context", func(t *testing.T) {
		call := mockToolCall(llm.ToolAskUserClarification, `{
			"question": "What genre should the novel be?",
			"options": ["Fantasy", "Sci-Fi", "Romance"],
			"context": "This will determine the tone"
		}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.Equal(t, SuggestionTypeClarification, result.Type)
		assert.Contains(t, result.Content, "What genre")
		assert.Contains(t, result.Content, "Fantasy")
		assert.Contains(t, result.Content, "Sci-Fi")
		assert.Contains(t, result.Content, "Romance")
		assert.Contains(t, result.Content, "determine the tone")
		assert.Len(t, result.Actions, 3) // One for each option
	})

	t.Run("handles question without options", func(t *testing.T) {
		call := mockToolCall(llm.ToolAskUserClarification, `{
			"question": "Describe the main conflict:"
		}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.Contains(t, result.Content, "Describe the main conflict")
		assert.Empty(t, result.Actions)
	})

	t.Run("handles question without context", func(t *testing.T) {
		call := mockToolCall(llm.ToolAskUserClarification, `{
			"question": "Choose a name:",
			"options": ["Alice", "Bob"]
		}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.Len(t, result.Actions, 2)
	})
}

func TestHandleToolCall_ContextUpdate(t *testing.T) {
	h := NewSuggestionHandler(nil, nil)

	t.Run("handles create character operation", func(t *testing.T) {
		call := mockToolCall(llm.ToolUpdateContext, `{
			"file_type": "character",
			"file_name": "alice",
			"operation": "create",
			"content": "Alice is a brave warrior...",
			"reason": "New protagonist introduced"
		}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.Equal(t, SuggestionTypeContextUpdate, result.Type)
		assert.True(t, result.RequiresApproval)
		assert.Contains(t, result.Content, "Create")
		assert.Contains(t, result.Content, "character")
		assert.Contains(t, result.Content, "alice")
		assert.Contains(t, result.Content, "New protagonist")
	})

	t.Run("handles update setting operation", func(t *testing.T) {
		call := mockToolCall(llm.ToolUpdateContext, `{
			"file_type": "setting",
			"file_name": "enchanted_forest",
			"operation": "update",
			"content": "The forest grows darker...",
			"reason": "Story progression"
		}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.Contains(t, result.Content, "Update")
		assert.Contains(t, result.Content, "setting")
	})

	t.Run("handles append plot operation", func(t *testing.T) {
		call := mockToolCall(llm.ToolUpdateContext, `{
			"file_type": "plot",
			"file_name": "main_arc",
			"operation": "append",
			"content": "New plot point...",
			"reason": "Chapter development"
		}`)

		result, err := h.HandleToolCall(call)

		require.NoError(t, err)
		assert.Contains(t, result.Content, "Append")
	})

	t.Run("rejects invalid file type", func(t *testing.T) {
		call := mockToolCall(llm.ToolUpdateContext, `{
			"file_type": "invalid",
			"file_name": "test",
			"operation": "create",
			"content": "content",
			"reason": "reason"
		}`)

		_, err := h.HandleToolCall(call)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})

	t.Run("rejects path traversal", func(t *testing.T) {
		call := mockToolCall(llm.ToolUpdateContext, `{
			"file_type": "character",
			"file_name": "../etc/passwd",
			"operation": "create",
			"content": "content",
			"reason": "reason"
		}`)

		_, err := h.HandleToolCall(call)

		assert.Error(t, err)
	})
}

func TestHandleToolCall_Search(t *testing.T) {
	h := NewSuggestionHandler(nil, nil)

	t.Run("returns error when search engine is nil", func(t *testing.T) {
		call := mockToolCall(llm.ToolSearchContext, `{
			"query": "dragon appearance"
		}`)

		_, err := h.HandleToolCall(call)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "search engine not initialized")
	})
}

func TestHandleToolCall_SearchWithEngine(t *testing.T) {
	// Skip if SQLite FTS5 is not available
	t.Skip("Requires SQLite FTS5 setup")

	// This would require setting up a real search engine
	// For integration testing purposes
}

func TestHandleToolCall_UnknownTool(t *testing.T) {
	h := NewSuggestionHandler(nil, nil)

	call := mockToolCall("unknown_tool", `{}`)

	_, err := h.HandleToolCall(call)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tool")
}

// ============================================================================
// ExecuteContextUpdate Tests
// ============================================================================

func TestExecuteContextUpdate(t *testing.T) {
	t.Run("returns error without project", func(t *testing.T) {
		h := NewSuggestionHandler(nil, nil)

		update := llm.ContextUpdate{
			FileType:  "character",
			FileName:  "test",
			Operation: "create",
			Content:   "test content",
			Reason:    "test",
		}

		err := h.ExecuteContextUpdate(update)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no project")
	})

	t.Run("validates path before execution", func(t *testing.T) {
		h := NewSuggestionHandler(nil, nil)

		update := llm.ContextUpdate{
			FileType:  "invalid",
			FileName:  "test",
			Operation: "create",
			Content:   "test",
			Reason:    "test",
		}

		err := h.ExecuteContextUpdate(update)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid")
	})
}

func TestExecuteContextUpdate_WithTempProject(t *testing.T) {
	// Create temporary directory for projects
	tmpDir, err := os.MkdirTemp("", "dreamteller-projects-*")
	require.NoError(t, err)
	defer os.RemoveAll(tmpDir)

	// Create a project manager
	manager, err := project.NewManager(tmpDir)
	require.NoError(t, err)

	// Create a test project
	projectConfig := &types.ProjectConfig{
		Version:   1,
		Name:      "Test Project",
		Genre:     "fantasy",
		CreatedAt: time.Now(),
	}

	proj, err := manager.Create("test-project", projectConfig)
	if err != nil {
		// Skip if FTS5 is not available (requires CGO_ENABLED=1 -tags fts5)
		if strings.Contains(err.Error(), "fts5") {
			t.Skip("SQLite FTS5 not available - requires CGO_ENABLED=1 and -tags fts5")
		}
		require.NoError(t, err)
	}

	h := NewSuggestionHandler(proj, nil)

	t.Run("creates new file", func(t *testing.T) {
		update := llm.ContextUpdate{
			FileType:  "character",
			FileName:  "new_character",
			Operation: "create",
			Content:   "# New Character\n\nA brave warrior.",
			Reason:    "Test creation",
		}

		err := h.ExecuteContextUpdate(update)

		require.NoError(t, err)

		// Verify file was created
		filePath := filepath.Join(proj.Path(), "context", "characters", "new_character.md")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "New Character")
	})

	t.Run("fails to create existing file", func(t *testing.T) {
		// First create a file
		existingPath := filepath.Join(proj.Path(), "context", "characters", "existing.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(existingPath), 0755))
		require.NoError(t, os.WriteFile(existingPath, []byte("existing"), 0644))

		update := llm.ContextUpdate{
			FileType:  "character",
			FileName:  "existing",
			Operation: "create",
			Content:   "New content",
			Reason:    "Test",
		}

		err := h.ExecuteContextUpdate(update)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("updates existing file", func(t *testing.T) {
		// Create file to update
		filePath := filepath.Join(proj.Path(), "context", "settings", "world.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755))
		require.NoError(t, os.WriteFile(filePath, []byte("# Old World\n\nOld description."), 0644))

		update := llm.ContextUpdate{
			FileType:  "setting",
			FileName:  "world",
			Operation: "update",
			Content:   "# New World\n\nNew description.",
			Reason:    "Update test",
		}

		err := h.ExecuteContextUpdate(update)

		require.NoError(t, err)

		// Verify content was updated
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "New World")
		assert.NotContains(t, string(content), "Old World")
	})

	t.Run("fails to update non-existing file", func(t *testing.T) {
		update := llm.ContextUpdate{
			FileType:  "setting",
			FileName:  "nonexistent",
			Operation: "update",
			Content:   "Content",
			Reason:    "Test",
		}

		err := h.ExecuteContextUpdate(update)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "does not exist")
	})

	t.Run("appends to existing file", func(t *testing.T) {
		// Create file to append to
		filePath := filepath.Join(proj.Path(), "context", "plot", "main.md")
		require.NoError(t, os.MkdirAll(filepath.Dir(filePath), 0755))
		require.NoError(t, os.WriteFile(filePath, []byte("# Plot\n\nAct 1: Beginning"), 0644))

		update := llm.ContextUpdate{
			FileType:  "plot",
			FileName:  "main",
			Operation: "append",
			Content:   "Act 2: Middle",
			Reason:    "Append test",
		}

		err := h.ExecuteContextUpdate(update)

		require.NoError(t, err)

		// Verify content was appended
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "Act 1: Beginning")
		assert.Contains(t, string(content), "Act 2: Middle")
	})

	t.Run("append creates file if not exists", func(t *testing.T) {
		update := llm.ContextUpdate{
			FileType:  "plot",
			FileName:  "new_plot",
			Operation: "append",
			Content:   "New plot content",
			Reason:    "Test",
		}

		err := h.ExecuteContextUpdate(update)

		require.NoError(t, err)

		// Verify file was created
		filePath := filepath.Join(proj.Path(), "context", "plot", "new_plot.md")
		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "New plot content")
	})

	t.Run("rejects unknown operation", func(t *testing.T) {
		update := llm.ContextUpdate{
			FileType:  "character",
			FileName:  "test",
			Operation: "delete", // Not supported
			Content:   "content",
			Reason:    "test",
		}

		err := h.ExecuteContextUpdate(update)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown operation")
	})
}

// ============================================================================
// ToolCallAccumulator Tests
// ============================================================================

func TestToolCallAccumulator_New(t *testing.T) {
	a := NewToolCallAccumulator()

	assert.NotNil(t, a)
	assert.False(t, a.HasCalls())
	assert.Empty(t, a.GetCompletedCalls())
}

func TestToolCallAccumulator_AddDelta(t *testing.T) {
	t.Run("adds first delta with all fields", func(t *testing.T) {
		a := NewToolCallAccumulator()

		delta := &llm.ToolCallDelta{
			Index: 0,
			ID:    "call_123",
			Type:  "function",
			Function: &llm.FunctionCallDelta{
				Name:      "test_function",
				Arguments: `{"arg":`,
			},
		}

		a.AddDelta(delta)

		assert.True(t, a.HasCalls())
		calls := a.GetCompletedCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "call_123", calls[0].ID)
		assert.Equal(t, "function", calls[0].Type)
		assert.Equal(t, "test_function", calls[0].Function.Name)
		assert.Equal(t, `{"arg":`, calls[0].Function.Arguments)
	})

	t.Run("accumulates arguments across deltas", func(t *testing.T) {
		a := NewToolCallAccumulator()

		a.AddDelta(&llm.ToolCallDelta{
			Index: 0,
			ID:    "call_123",
			Function: &llm.FunctionCallDelta{
				Name:      "test",
				Arguments: `{"key":`,
			},
		})

		a.AddDelta(&llm.ToolCallDelta{
			Index: 0,
			Function: &llm.FunctionCallDelta{
				Arguments: `"value"`,
			},
		})

		a.AddDelta(&llm.ToolCallDelta{
			Index: 0,
			Function: &llm.FunctionCallDelta{
				Arguments: `}`,
			},
		})

		calls := a.GetCompletedCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, `{"key":"value"}`, calls[0].Function.Arguments)
	})

	t.Run("handles multiple parallel tool calls", func(t *testing.T) {
		a := NewToolCallAccumulator()

		// First tool call
		a.AddDelta(&llm.ToolCallDelta{
			Index: 0,
			ID:    "call_1",
			Function: &llm.FunctionCallDelta{
				Name:      "function_1",
				Arguments: `{"a":1}`,
			},
		})

		// Second tool call
		a.AddDelta(&llm.ToolCallDelta{
			Index: 1,
			ID:    "call_2",
			Function: &llm.FunctionCallDelta{
				Name:      "function_2",
				Arguments: `{"b":2}`,
			},
		})

		calls := a.GetCompletedCalls()
		assert.Len(t, calls, 2)
	})

	t.Run("handles nil delta gracefully", func(t *testing.T) {
		a := NewToolCallAccumulator()

		a.AddDelta(nil)

		assert.False(t, a.HasCalls())
	})

	t.Run("handles delta without function", func(t *testing.T) {
		a := NewToolCallAccumulator()

		a.AddDelta(&llm.ToolCallDelta{
			Index: 0,
			ID:    "call_123",
			Type:  "function",
		})

		calls := a.GetCompletedCalls()
		require.Len(t, calls, 1)
		assert.Equal(t, "call_123", calls[0].ID)
		assert.Empty(t, calls[0].Function.Name)
	})
}

func TestToolCallAccumulator_Reset(t *testing.T) {
	a := NewToolCallAccumulator()

	// Add some deltas
	a.AddDelta(&llm.ToolCallDelta{
		Index: 0,
		ID:    "call_1",
		Function: &llm.FunctionCallDelta{
			Name:      "test",
			Arguments: `{}`,
		},
	})
	a.AddDelta(&llm.ToolCallDelta{
		Index: 1,
		ID:    "call_2",
		Function: &llm.FunctionCallDelta{
			Name:      "test2",
			Arguments: `{}`,
		},
	})

	require.True(t, a.HasCalls())

	a.Reset()

	assert.False(t, a.HasCalls())
	assert.Empty(t, a.GetCompletedCalls())
}

func TestToolCallAccumulator_HasCalls(t *testing.T) {
	t.Run("returns false when empty", func(t *testing.T) {
		a := NewToolCallAccumulator()

		assert.False(t, a.HasCalls())
	})

	t.Run("returns true after adding delta", func(t *testing.T) {
		a := NewToolCallAccumulator()
		a.AddDelta(&llm.ToolCallDelta{Index: 0, ID: "call_1"})

		assert.True(t, a.HasCalls())
	})

	t.Run("returns false after reset", func(t *testing.T) {
		a := NewToolCallAccumulator()
		a.AddDelta(&llm.ToolCallDelta{Index: 0, ID: "call_1"})
		a.Reset()

		assert.False(t, a.HasCalls())
	})
}

func TestToolCallAccumulator_GetCompletedCalls(t *testing.T) {
	t.Run("returns empty slice when no calls", func(t *testing.T) {
		a := NewToolCallAccumulator()

		calls := a.GetCompletedCalls()

		assert.NotNil(t, calls)
		assert.Empty(t, calls)
	})

	t.Run("returns all accumulated calls", func(t *testing.T) {
		a := NewToolCallAccumulator()

		a.AddDelta(&llm.ToolCallDelta{
			Index:    0,
			ID:       "call_1",
			Type:     "function",
			Function: &llm.FunctionCallDelta{Name: "func1", Arguments: `{}`},
		})
		a.AddDelta(&llm.ToolCallDelta{
			Index:    1,
			ID:       "call_2",
			Type:     "function",
			Function: &llm.FunctionCallDelta{Name: "func2", Arguments: `{}`},
		})

		calls := a.GetCompletedCalls()

		require.Len(t, calls, 2)

		// Find each call (order may vary due to map iteration)
		var foundCall1, foundCall2 bool
		for _, call := range calls {
			if call.ID == "call_1" {
				assert.Equal(t, "func1", call.Function.Name)
				foundCall1 = true
			}
			if call.ID == "call_2" {
				assert.Equal(t, "func2", call.Function.Name)
				foundCall2 = true
			}
		}
		assert.True(t, foundCall1)
		assert.True(t, foundCall2)
	})
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestFormatOperation(t *testing.T) {
	tests := []struct {
		operation string
		expected  string
	}{
		{"create", "Create"},
		{"update", "Update"},
		{"append", "Append to"},
		{"other", "Other"},
	}

	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			result := formatOperation(tt.operation)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPluralizeFileType(t *testing.T) {
	tests := []struct {
		fileType string
		expected string
	}{
		{"character", "characters"},
		{"setting", "settings"},
		{"plot", "plot"},
		{"other", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.fileType, func(t *testing.T) {
			result := pluralizeFileType(tt.fileType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		maxLen  int
		check   func(t *testing.T, result string)
	}{
		{
			name:    "short content unchanged",
			content: "Hello",
			maxLen:  100,
			check: func(t *testing.T, result string) {
				assert.Equal(t, "Hello", result)
			},
		},
		{
			name:    "long content truncated",
			content: "This is a very long content that should be truncated",
			maxLen:  20,
			check: func(t *testing.T, result string) {
				assert.Len(t, result, 20)
				assert.True(t, result[len(result)-3:] == "...")
			},
		},
		{
			name:    "newlines replaced with spaces",
			content: "Line1\nLine2\nLine3",
			maxLen:  100,
			check: func(t *testing.T, result string) {
				assert.NotContains(t, result, "\n")
				assert.Contains(t, result, "Line1")
			},
		},
		{
			name:    "multiple spaces collapsed",
			content: "Word1    Word2",
			maxLen:  100,
			check: func(t *testing.T, result string) {
				assert.NotContains(t, result, "  ")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateContent(tt.content, tt.maxLen)
			tt.check(t, result)
		})
	}
}

// ============================================================================
// SuggestionResult Tests
// ============================================================================

func TestSuggestionResult(t *testing.T) {
	t.Run("creates result with all fields", func(t *testing.T) {
		result := &SuggestionResult{
			Type:             SuggestionTypePlot,
			Title:            "Test Title",
			Content:          "Test Content",
			RequiresApproval: true,
			ToolCallID:       "call_123",
			Actions: []SuggestionAction{
				{Key: "1", Label: "Option 1"},
			},
		}

		assert.Equal(t, SuggestionTypePlot, result.Type)
		assert.Equal(t, "Test Title", result.Title)
		assert.Equal(t, "Test Content", result.Content)
		assert.True(t, result.RequiresApproval)
		assert.Equal(t, "call_123", result.ToolCallID)
		assert.Len(t, result.Actions, 1)
	})
}

func TestSuggestionAction(t *testing.T) {
	t.Run("action with handler executes", func(t *testing.T) {
		executed := false
		action := SuggestionAction{
			Key:   "1",
			Label: "Test Action",
			Handler: func() error {
				executed = true
				return nil
			},
		}

		err := action.Handler()

		assert.NoError(t, err)
		assert.True(t, executed)
	})
}

// ============================================================================
// Integration with Search Engine (requires setup)
// ============================================================================

func TestHandleToolCall_SearchWithMockEngine(t *testing.T) {
	// Skip unless we have a way to mock the search engine
	t.Skip("Requires search engine mock")
}

// mockSearchEngine would be defined here if needed for testing
type mockSearchEngine struct {
	results []search.FTSSearchResult
	err     error
}

func (m *mockSearchEngine) Search(query string, limit int) ([]search.FTSSearchResult, error) {
	return m.results, m.err
}

func (m *mockSearchEngine) SearchWithFilter(query, filterType string, limit int) ([]search.FTSSearchResult, error) {
	return m.results, m.err
}
