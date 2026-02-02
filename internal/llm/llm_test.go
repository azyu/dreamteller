package llm

import (
	"testing"

	"github.com/azyu/dreamteller/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ChatMessage Helper Tests
// ============================================================================

// TestNewSystemMessage tests creation of system messages.
func TestNewSystemMessage(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "creates system message with content",
			content: "You are a helpful assistant.",
		},
		{
			name:    "creates system message with empty content",
			content: "",
		},
		{
			name:    "creates system message with long content",
			content: "This is a very long system prompt that contains detailed instructions for the AI assistant to follow when generating responses.",
		},
		{
			name:    "creates system message with special characters",
			content: "Handle these: <xml> & \"quotes\" 'apostrophes' \n\ttabs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewSystemMessage(tt.content)

			assert.Equal(t, RoleSystem, msg.Role)
			assert.Equal(t, tt.content, msg.Content)
			assert.Empty(t, msg.ToolCalls)
			assert.Empty(t, msg.ToolCallID)
			assert.Empty(t, msg.Name)
		})
	}
}

// TestNewUserMessage tests creation of user messages.
func TestNewUserMessage(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "creates user message with content",
			content: "Hello, how are you?",
		},
		{
			name:    "creates user message with empty content",
			content: "",
		},
		{
			name:    "creates user message with multiline content",
			content: "Line 1\nLine 2\nLine 3",
		},
		{
			name:    "creates user message with unicode",
			content: "Hello, 世界! こんにちは 🌍",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewUserMessage(tt.content)

			assert.Equal(t, RoleUser, msg.Role)
			assert.Equal(t, tt.content, msg.Content)
			assert.Empty(t, msg.ToolCalls)
			assert.Empty(t, msg.ToolCallID)
			assert.Empty(t, msg.Name)
		})
	}
}

// TestNewAssistantMessage tests creation of assistant messages.
func TestNewAssistantMessage(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name:    "creates assistant message with content",
			content: "I'm doing well, thank you for asking!",
		},
		{
			name:    "creates assistant message with empty content",
			content: "",
		},
		{
			name:    "creates assistant message with code block",
			content: "Here's an example:\n```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewAssistantMessage(tt.content)

			assert.Equal(t, RoleAssistant, msg.Role)
			assert.Equal(t, tt.content, msg.Content)
			assert.Empty(t, msg.ToolCalls)
			assert.Empty(t, msg.ToolCallID)
			assert.Empty(t, msg.Name)
		})
	}
}

// TestNewToolMessage tests creation of tool response messages.
func TestNewToolMessage(t *testing.T) {
	tests := []struct {
		name       string
		toolCallID string
		toolName   string
		content    string
	}{
		{
			name:       "creates tool message with all fields",
			toolCallID: "call_abc123",
			toolName:   "search_context",
			content:    `{"results": ["item1", "item2"]}`,
		},
		{
			name:       "creates tool message with empty content",
			toolCallID: "call_xyz789",
			toolName:   "suggest_plot_development",
			content:    "",
		},
		{
			name:       "creates tool message with error content",
			toolCallID: "call_error",
			toolName:   "update_context",
			content:    `{"error": "Permission denied"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewToolMessage(tt.toolCallID, tt.toolName, tt.content)

			assert.Equal(t, RoleTool, msg.Role)
			assert.Equal(t, tt.content, msg.Content)
			assert.Equal(t, tt.toolCallID, msg.ToolCallID)
			assert.Equal(t, tt.toolName, msg.Name)
			assert.Empty(t, msg.ToolCalls)
		})
	}
}

// TestNewChatMessage tests the generic message creator.
func TestNewChatMessage(t *testing.T) {
	tests := []struct {
		name        string
		role        string
		content     string
		wantRole    string
		wantContent string
	}{
		{
			name:        "creates message with system role",
			role:        RoleSystem,
			content:     "System content",
			wantRole:    RoleSystem,
			wantContent: "System content",
		},
		{
			name:        "creates message with user role",
			role:        RoleUser,
			content:     "User content",
			wantRole:    RoleUser,
			wantContent: "User content",
		},
		{
			name:        "creates message with assistant role",
			role:        RoleAssistant,
			content:     "Assistant content",
			wantRole:    RoleAssistant,
			wantContent: "Assistant content",
		},
		{
			name:        "creates message with custom role",
			role:        "custom",
			content:     "Custom content",
			wantRole:    "custom",
			wantContent: "Custom content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewChatMessage(tt.role, tt.content)

			assert.Equal(t, tt.wantRole, msg.Role)
			assert.Equal(t, tt.wantContent, msg.Content)
		})
	}
}

// TestChatMessage_IsToolCallResponse tests tool call response detection.
func TestChatMessage_IsToolCallResponse(t *testing.T) {
	tests := []struct {
		name    string
		message ChatMessage
		want    bool
	}{
		{
			name: "returns true for tool response with ID",
			message: ChatMessage{
				Role:       RoleTool,
				ToolCallID: "call_123",
				Content:    "result",
			},
			want: true,
		},
		{
			name: "returns false for tool role without ID",
			message: ChatMessage{
				Role:    RoleTool,
				Content: "result",
			},
			want: false,
		},
		{
			name: "returns false for user message",
			message: ChatMessage{
				Role:    RoleUser,
				Content: "hello",
			},
			want: false,
		},
		{
			name: "returns false for assistant message",
			message: ChatMessage{
				Role:    RoleAssistant,
				Content: "hello",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.message.IsToolCallResponse()
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestChatMessage_HasToolCalls tests tool call presence detection.
func TestChatMessage_HasToolCalls(t *testing.T) {
	tests := []struct {
		name    string
		message ChatMessage
		want    bool
	}{
		{
			name: "returns true when tool calls present",
			message: ChatMessage{
				Role: RoleAssistant,
				ToolCalls: []ToolCall{
					{ID: "call_1", Type: "function"},
				},
			},
			want: true,
		},
		{
			name: "returns true with multiple tool calls",
			message: ChatMessage{
				Role: RoleAssistant,
				ToolCalls: []ToolCall{
					{ID: "call_1", Type: "function"},
					{ID: "call_2", Type: "function"},
				},
			},
			want: true,
		},
		{
			name: "returns false when tool calls empty",
			message: ChatMessage{
				Role:      RoleAssistant,
				ToolCalls: []ToolCall{},
			},
			want: false,
		},
		{
			name: "returns false when tool calls nil",
			message: ChatMessage{
				Role: RoleAssistant,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.message.HasToolCalls()
			assert.Equal(t, tt.want, result)
		})
	}
}

// ============================================================================
// ToolDefinition Tests
// ============================================================================

// TestNewToolDefinition tests tool definition creation.
func TestNewToolDefinition(t *testing.T) {
	tests := []struct {
		name        string
		toolName    string
		description string
		parameters  map[string]interface{}
	}{
		{
			name:        "creates tool with basic parameters",
			toolName:    "search",
			description: "Search for content",
			parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Search query",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			name:        "creates tool with empty parameters",
			toolName:    "get_status",
			description: "Get current status",
			parameters:  map[string]interface{}{},
		},
		{
			name:        "creates tool with nil parameters",
			toolName:    "ping",
			description: "Ping the service",
			parameters:  nil,
		},
		{
			name:        "creates tool with complex parameters",
			toolName:    "create_character",
			description: "Create a new character",
			parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type": "string",
					},
					"traits": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
					},
					"age": map[string]interface{}{
						"type": "integer",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := NewToolDefinition(tt.toolName, tt.description, tt.parameters)

			assert.Equal(t, "function", tool.Type)
			assert.Equal(t, tt.toolName, tool.Function.Name)
			assert.Equal(t, tt.description, tool.Function.Description)
			assert.Equal(t, tt.parameters, tool.Function.Parameters)
		})
	}
}

// TestPredefinedTools tests that predefined tools are returned correctly.
func TestPredefinedTools(t *testing.T) {
	tools := PredefinedTools()

	require.NotEmpty(t, tools, "PredefinedTools should return tools")

	expectedToolNames := []string{
		ToolSuggestPlotDevelopment,
		ToolSuggestCharacterAction,
		ToolAskUserClarification,
		ToolUpdateContext,
		ToolSearchContext,
		ToolExtractProjectSetup,
	}

	t.Run("contains all expected tools", func(t *testing.T) {
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Function.Name] = true
		}

		for _, expected := range expectedToolNames {
			assert.True(t, toolNames[expected], "missing tool: %s", expected)
		}
	})

	t.Run("all tools have valid structure", func(t *testing.T) {
		for _, tool := range tools {
			assert.Equal(t, "function", tool.Type, "tool %s should have type 'function'", tool.Function.Name)
			assert.NotEmpty(t, tool.Function.Name, "tool should have a name")
			assert.NotEmpty(t, tool.Function.Description, "tool %s should have a description", tool.Function.Name)
			assert.NotNil(t, tool.Function.Parameters, "tool %s should have parameters", tool.Function.Name)
		}
	})

	t.Run("suggest_plot_development has correct structure", func(t *testing.T) {
		var plotTool *ToolDefinition
		for _, tool := range tools {
			if tool.Function.Name == ToolSuggestPlotDevelopment {
				plotTool = &tool
				break
			}
		}
		require.NotNil(t, plotTool)

		params := plotTool.Function.Parameters
		props, ok := params["properties"].(map[string]interface{})
		require.True(t, ok)

		_, hasSuggestions := props["suggestions"]
		assert.True(t, hasSuggestions, "should have suggestions property")
	})

	t.Run("update_context has file_type enum", func(t *testing.T) {
		var updateTool *ToolDefinition
		for _, tool := range tools {
			if tool.Function.Name == ToolUpdateContext {
				updateTool = &tool
				break
			}
		}
		require.NotNil(t, updateTool)

		params := updateTool.Function.Parameters
		props, ok := params["properties"].(map[string]interface{})
		require.True(t, ok)

		fileType, ok := props["file_type"].(map[string]interface{})
		require.True(t, ok)

		enum, ok := fileType["enum"].([]string)
		require.True(t, ok)
		assert.Contains(t, enum, "character")
		assert.Contains(t, enum, "setting")
		assert.Contains(t, enum, "plot")
	})
}

// ============================================================================
// ParseToolCall Tests
// ============================================================================

// TestParseToolCall_PlotSuggestions tests parsing plot development suggestions.
func TestParseToolCall_PlotSuggestions(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
		wantLen   int
		wantErr   bool
	}{
		{
			name: "parses single suggestion",
			arguments: `{
				"suggestions": [
					{
						"title": "The Hidden Truth",
						"description": "Reveal the antagonist's true motivation",
						"impact": "Changes character dynamics"
					}
				]
			}`,
			wantLen: 1,
			wantErr: false,
		},
		{
			name: "parses multiple suggestions",
			arguments: `{
				"suggestions": [
					{"title": "Plot twist 1", "description": "Description 1"},
					{"title": "Plot twist 2", "description": "Description 2", "impact": "High impact"},
					{"title": "Plot twist 3", "description": "Description 3"}
				]
			}`,
			wantLen: 3,
			wantErr: false,
		},
		{
			name:      "parses empty suggestions",
			arguments: `{"suggestions": []}`,
			wantLen:   0,
			wantErr:   false,
		},
		{
			name:      "fails on invalid JSON",
			arguments: `{"suggestions": [invalid]}`,
			wantErr:   true,
		},
		{
			name:      "fails on missing suggestions field",
			arguments: `{"other": "value"}`,
			wantLen:   0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := ToolCall{
				ID:   "call_123",
				Type: "function",
				Function: FunctionCall{
					Name:      ToolSuggestPlotDevelopment,
					Arguments: tt.arguments,
				},
			}

			result, err := ParseToolCall(call)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			suggestions, ok := result.([]PlotSuggestion)
			require.True(t, ok)
			assert.Len(t, suggestions, tt.wantLen)
		})
	}
}

// TestParseToolCall_CharacterActions tests parsing character action suggestions.
func TestParseToolCall_CharacterActions(t *testing.T) {
	tests := []struct {
		name          string
		arguments     string
		wantCharacter string
		wantActions   int
		wantErr       bool
	}{
		{
			name: "parses character with single action",
			arguments: `{
				"character": "Alice",
				"actions": [
					{
						"action": "She confronts the villain",
						"motivation": "To protect her friends",
						"dialogue": "I won't let you harm them!"
					}
				]
			}`,
			wantCharacter: "Alice",
			wantActions:   1,
			wantErr:       false,
		},
		{
			name: "parses character with multiple actions",
			arguments: `{
				"character": "Bob",
				"actions": [
					{"action": "Action 1", "motivation": "Reason 1"},
					{"action": "Action 2", "motivation": "Reason 2"},
					{"action": "Action 3", "motivation": "Reason 3"}
				]
			}`,
			wantCharacter: "Bob",
			wantActions:   3,
			wantErr:       false,
		},
		{
			name: "parses action without optional dialogue",
			arguments: `{
				"character": "Carol",
				"actions": [
					{"action": "She observes quietly", "motivation": "Gathering information"}
				]
			}`,
			wantCharacter: "Carol",
			wantActions:   1,
			wantErr:       false,
		},
		{
			name:      "fails on invalid JSON",
			arguments: `{invalid}`,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := ToolCall{
				ID:   "call_456",
				Type: "function",
				Function: FunctionCall{
					Name:      ToolSuggestCharacterAction,
					Arguments: tt.arguments,
				},
			}

			result, err := ParseToolCall(call)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			suggestion, ok := result.(CharacterActionSuggestion)
			require.True(t, ok)
			assert.Equal(t, tt.wantCharacter, suggestion.Character)
			assert.Len(t, suggestion.Actions, tt.wantActions)
		})
	}
}

// TestParseToolCall_ClarificationQuestions tests parsing clarification questions.
func TestParseToolCall_ClarificationQuestions(t *testing.T) {
	tests := []struct {
		name         string
		arguments    string
		wantQuestion string
		wantOptions  int
		wantContext  string
		wantErr      bool
	}{
		{
			name: "parses question with options and context",
			arguments: `{
				"question": "What genre should the novel be?",
				"options": ["Fantasy", "Sci-Fi", "Romance"],
				"context": "This will help determine the tone and setting"
			}`,
			wantQuestion: "What genre should the novel be?",
			wantOptions:  3,
			wantContext:  "This will help determine the tone and setting",
			wantErr:      false,
		},
		{
			name: "parses question with options only",
			arguments: `{
				"question": "Choose a protagonist name:",
				"options": ["Alice", "Bob"]
			}`,
			wantQuestion: "Choose a protagonist name:",
			wantOptions:  2,
			wantContext:  "",
			wantErr:      false,
		},
		{
			name: "parses question without options",
			arguments: `{
				"question": "Describe the main conflict:"
			}`,
			wantQuestion: "Describe the main conflict:",
			wantOptions:  0,
			wantContext:  "",
			wantErr:      false,
		},
		{
			name:      "fails on invalid JSON",
			arguments: `{"question": }`,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := ToolCall{
				ID:   "call_789",
				Type: "function",
				Function: FunctionCall{
					Name:      ToolAskUserClarification,
					Arguments: tt.arguments,
				},
			}

			result, err := ParseToolCall(call)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			question, ok := result.(ClarificationQuestion)
			require.True(t, ok)
			assert.Equal(t, tt.wantQuestion, question.Question)
			assert.Len(t, question.Options, tt.wantOptions)
			assert.Equal(t, tt.wantContext, question.Context)
		})
	}
}

// TestParseToolCall_ContextUpdates tests parsing context update requests.
func TestParseToolCall_ContextUpdates(t *testing.T) {
	tests := []struct {
		name          string
		arguments     string
		wantFileType  string
		wantFileName  string
		wantOperation string
		wantErr       bool
	}{
		{
			name: "parses create character operation",
			arguments: `{
				"file_type": "character",
				"file_name": "alice",
				"operation": "create",
				"content": "Alice is a brave warrior...",
				"reason": "New protagonist introduced"
			}`,
			wantFileType:  "character",
			wantFileName:  "alice",
			wantOperation: "create",
			wantErr:       false,
		},
		{
			name: "parses update setting operation",
			arguments: `{
				"file_type": "setting",
				"file_name": "enchanted_forest",
				"operation": "update",
				"content": "The forest grows darker...",
				"reason": "Story progression"
			}`,
			wantFileType:  "setting",
			wantFileName:  "enchanted_forest",
			wantOperation: "update",
			wantErr:       false,
		},
		{
			name: "parses append plot operation",
			arguments: `{
				"file_type": "plot",
				"file_name": "main_arc",
				"operation": "append",
				"content": "New plot point...",
				"reason": "Chapter development"
			}`,
			wantFileType:  "plot",
			wantFileName:  "main_arc",
			wantOperation: "append",
			wantErr:       false,
		},
		{
			name:      "fails on invalid JSON",
			arguments: `{"file_type": character}`,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := ToolCall{
				ID:   "call_update",
				Type: "function",
				Function: FunctionCall{
					Name:      ToolUpdateContext,
					Arguments: tt.arguments,
				},
			}

			result, err := ParseToolCall(call)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			update, ok := result.(ContextUpdate)
			require.True(t, ok)
			assert.Equal(t, tt.wantFileType, update.FileType)
			assert.Equal(t, tt.wantFileName, update.FileName)
			assert.Equal(t, tt.wantOperation, update.Operation)
			assert.NotEmpty(t, update.Content)
			assert.NotEmpty(t, update.Reason)
		})
	}
}

// TestParseToolCall_UnknownTool tests error on unknown tool.
func TestParseToolCall_UnknownTool(t *testing.T) {
	call := ToolCall{
		ID:   "call_unknown",
		Type: "function",
		Function: FunctionCall{
			Name:      "unknown_tool",
			Arguments: `{}`,
		},
	}

	result, err := ParseToolCall(call)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "unknown tool")
}

// TestParseToolCall_SearchContext tests parsing search queries.
func TestParseToolCall_SearchContext(t *testing.T) {
	tests := []struct {
		name           string
		arguments      string
		wantQuery      string
		wantFilterType string
		wantErr        bool
	}{
		{
			name: "parses search with filter",
			arguments: `{
				"query": "dragon appearance",
				"filter_type": "character"
			}`,
			wantQuery:      "dragon appearance",
			wantFilterType: "character",
			wantErr:        false,
		},
		{
			name: "parses search without filter",
			arguments: `{
				"query": "magical artifacts"
			}`,
			wantQuery:      "magical artifacts",
			wantFilterType: "",
			wantErr:        false,
		},
		{
			name:      "fails on invalid JSON",
			arguments: `{query: invalid}`,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call := ToolCall{
				ID:   "call_search",
				Type: "function",
				Function: FunctionCall{
					Name:      ToolSearchContext,
					Arguments: tt.arguments,
				},
			}

			result, err := ParseToolCall(call)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			query, ok := result.(SearchQuery)
			require.True(t, ok)
			assert.Equal(t, tt.wantQuery, query.Query)
			assert.Equal(t, tt.wantFilterType, query.FilterType)
		})
	}
}

// TestParseToolCall_ExtractProjectSetup tests parsing project setup extractions.
func TestParseToolCall_ExtractProjectSetup(t *testing.T) {
	call := ToolCall{
		ID:   "call_extract",
		Type: "function",
		Function: FunctionCall{
			Name: ToolExtractProjectSetup,
			Arguments: `{
				"genre": "fantasy",
				"setting": {
					"time_period": "medieval",
					"location": "enchanted kingdom",
					"description": "A magical realm..."
				},
				"characters": [
					{"name": "Hero", "role": "protagonist", "description": "Brave warrior"}
				],
				"plot_hints": ["Quest for the artifact"],
				"style_guide": {
					"tone": "epic",
					"pacing": "fast"
				}
			}`,
		},
	}

	result, err := ParseToolCall(call)

	require.NoError(t, err)
	require.NotNil(t, result)

	// The result is a struct with these fields
	extracted, ok := result.(struct {
		Genre      string        `json:"genre"`
		Setting    interface{}   `json:"setting"`
		Characters []interface{} `json:"characters"`
		PlotHints  []string      `json:"plot_hints"`
		StyleGuide interface{}   `json:"style_guide"`
	})
	require.True(t, ok)
	assert.Equal(t, "fantasy", extracted.Genre)
	assert.Len(t, extracted.PlotHints, 1)
}

// ============================================================================
// ValidateContextUpdatePath Tests
// ============================================================================

// TestValidateContextUpdatePath_ValidPaths tests valid path acceptance.
func TestValidateContextUpdatePath_ValidPaths(t *testing.T) {
	tests := []struct {
		name     string
		fileType string
		fileName string
	}{
		{
			name:     "valid character file",
			fileType: "character",
			fileName: "alice",
		},
		{
			name:     "valid setting file",
			fileType: "setting",
			fileName: "enchanted_forest",
		},
		{
			name:     "valid plot file",
			fileType: "plot",
			fileName: "main_arc",
		},
		{
			name:     "file with numbers",
			fileType: "character",
			fileName: "character01",
		},
		{
			name:     "file with dashes",
			fileType: "setting",
			fileName: "dark-castle",
		},
		{
			name:     "file with underscores",
			fileType: "plot",
			fileName: "side_quest_1",
		},
		{
			name:     "mixed case",
			fileType: "character",
			fileName: "ElvenQueen",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContextUpdatePath(tt.fileType, tt.fileName)
			assert.NoError(t, err)
		})
	}
}

// TestValidateContextUpdatePath_InvalidFileTypes tests invalid file type rejection.
func TestValidateContextUpdatePath_InvalidFileTypes(t *testing.T) {
	tests := []struct {
		name     string
		fileType string
		fileName string
	}{
		{
			name:     "invalid file type chapter",
			fileType: "chapter",
			fileName: "chapter1",
		},
		{
			name:     "invalid file type system",
			fileType: "system",
			fileName: "config",
		},
		{
			name:     "empty file type",
			fileType: "",
			fileName: "test",
		},
		{
			name:     "invalid file type with valid name",
			fileType: "arbitrary",
			fileName: "valid_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContextUpdatePath(tt.fileType, tt.fileName)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid file type")
		})
	}
}

// TestValidateContextUpdatePath_PathTraversal tests path traversal rejection.
func TestValidateContextUpdatePath_PathTraversal(t *testing.T) {
	tests := []struct {
		name     string
		fileType string
		fileName string
		errMsg   string
	}{
		{
			name:     "forward slash traversal",
			fileType: "character",
			fileName: "../alice",
			errMsg:   "invalid character",
		},
		{
			name:     "backslash traversal",
			fileType: "setting",
			fileName: "..\\forest",
			errMsg:   "invalid character",
		},
		{
			name:     "double dot only",
			fileType: "plot",
			fileName: "..",
			errMsg:   "invalid file name",
		},
		{
			name:     "single dot only",
			fileType: "character",
			fileName: ".",
			errMsg:   "invalid file name",
		},
		{
			name:     "empty filename",
			fileType: "setting",
			fileName: "",
			errMsg:   "invalid file name",
		},
		{
			name:     "colon in filename",
			fileType: "character",
			fileName: "c:alice",
			errMsg:   "invalid character",
		},
		{
			name:     "nested path with slashes",
			fileType: "setting",
			fileName: "subdir/filename",
			errMsg:   "invalid character",
		},
		{
			name:     "absolute path attempt",
			fileType: "plot",
			fileName: "/etc/passwd",
			errMsg:   "invalid character",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateContextUpdatePath(tt.fileType, tt.fileName)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// ============================================================================
// ContextManager Tests
// ============================================================================

// MockTokenCounter is a mock implementation of TokenCounter for testing.
type MockTokenCounter struct {
	tokensPerChar float64
}

// NewMockTokenCounter creates a new mock token counter.
func NewMockTokenCounter(tokensPerChar float64) *MockTokenCounter {
	return &MockTokenCounter{tokensPerChar: tokensPerChar}
}

// Count returns a mock token count based on string length.
func (m *MockTokenCounter) Count(text string) int {
	return int(float64(len(text)) * m.tokensPerChar)
}

// TestNewContextManager tests ContextManager creation.
func TestNewContextManager(t *testing.T) {
	config := types.ContextConfig{
		MaxChunks:    5,
		ChunkSize:    800,
		ChunkOverlap: 0.15,
	}
	budget := types.BudgetConfig{
		SystemPrompt: 0.20,
		Context:      0.40,
		History:      0.30,
		Response:     0.10,
	}
	maxTokens := 100000
	tokenizer := NewMockTokenCounter(0.25)

	cm := NewContextManager(config, budget, maxTokens, tokenizer)

	assert.NotNil(t, cm)
}

// TestContextManager_CalculateBudget tests budget calculation.
func TestContextManager_CalculateBudget(t *testing.T) {
	tests := []struct {
		name             string
		budget           types.BudgetConfig
		maxTokens        int
		wantSystemPrompt int
		wantContext      int
		wantHistory      int
		wantResponse     int
		wantTotal        int
	}{
		{
			name: "default ratios with 100000 tokens",
			budget: types.BudgetConfig{
				SystemPrompt: 0.20,
				Context:      0.40,
				History:      0.30,
				Response:     0.10,
			},
			maxTokens:        100000,
			wantSystemPrompt: 20000,
			wantContext:      40000,
			wantHistory:      30000,
			wantResponse:     10000,
			wantTotal:        100000,
		},
		{
			name: "equal ratios with 8000 tokens",
			budget: types.BudgetConfig{
				SystemPrompt: 0.25,
				Context:      0.25,
				History:      0.25,
				Response:     0.25,
			},
			maxTokens:        8000,
			wantSystemPrompt: 2000,
			wantContext:      2000,
			wantHistory:      2000,
			wantResponse:     2000,
			wantTotal:        8000,
		},
		{
			name: "context-heavy budget",
			budget: types.BudgetConfig{
				SystemPrompt: 0.10,
				Context:      0.60,
				History:      0.20,
				Response:     0.10,
			},
			maxTokens:        50000,
			wantSystemPrompt: 5000,
			wantContext:      30000,
			wantHistory:      10000,
			wantResponse:     5000,
			wantTotal:        50000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := types.ContextConfig{MaxChunks: 5}
			tokenizer := NewMockTokenCounter(0.25)
			cm := NewContextManager(config, tt.budget, tt.maxTokens, tokenizer)

			budget := cm.CalculateBudget()

			assert.Equal(t, tt.wantSystemPrompt, budget.SystemPrompt)
			assert.Equal(t, tt.wantContext, budget.Context)
			assert.Equal(t, tt.wantHistory, budget.History)
			assert.Equal(t, tt.wantResponse, budget.Response)
			assert.Equal(t, tt.wantTotal, budget.Total)
		})
	}
}

// TestContextManager_SelectChunks tests chunk selection within budget.
func TestContextManager_SelectChunks(t *testing.T) {
	config := types.ContextConfig{MaxChunks: 3}
	budget := types.BudgetConfig{
		SystemPrompt: 0.20,
		Context:      0.40,
		History:      0.30,
		Response:     0.10,
	}
	tokenizer := NewMockTokenCounter(0.25)
	cm := NewContextManager(config, budget, 100000, tokenizer)

	tests := []struct {
		name        string
		chunks      []ContextChunk
		budget      int
		wantLen     int
		wantTokens  int
	}{
		{
			name:       "empty chunks returns empty",
			chunks:     []ContextChunk{},
			budget:     1000,
			wantLen:    0,
			wantTokens: 0,
		},
		{
			name: "all chunks fit within budget",
			chunks: []ContextChunk{
				{Content: "Chunk 1", Tokens: 100, Score: 0.9},
				{Content: "Chunk 2", Tokens: 100, Score: 0.8},
			},
			budget:     500,
			wantLen:    2,
			wantTokens: 200,
		},
		{
			name: "budget limits selection",
			chunks: []ContextChunk{
				{Content: "Chunk 1", Tokens: 300, Score: 0.9},
				{Content: "Chunk 2", Tokens: 300, Score: 0.8},
				{Content: "Chunk 3", Tokens: 300, Score: 0.7},
			},
			budget:     500,
			wantLen:    1,
			wantTokens: 300,
		},
		{
			name: "skips chunks that exceed budget",
			chunks: []ContextChunk{
				{Content: "Chunk 1", Tokens: 200, Score: 0.9},
				{Content: "Chunk 2", Tokens: 400, Score: 0.8}, // skipped
				{Content: "Chunk 3", Tokens: 200, Score: 0.7},
			},
			budget:     500,
			wantLen:    2,
			wantTokens: 400,
		},
		{
			name: "respects MaxChunks limit",
			chunks: []ContextChunk{
				{Content: "Chunk 1", Tokens: 50, Score: 0.9},
				{Content: "Chunk 2", Tokens: 50, Score: 0.8},
				{Content: "Chunk 3", Tokens: 50, Score: 0.7},
				{Content: "Chunk 4", Tokens: 50, Score: 0.6},
				{Content: "Chunk 5", Tokens: 50, Score: 0.5},
			},
			budget:     10000,
			wantLen:    3, // MaxChunks = 3
			wantTokens: 150,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected := cm.SelectChunks(tt.chunks, tt.budget)

			assert.Len(t, selected, tt.wantLen)

			totalTokens := 0
			for _, c := range selected {
				totalTokens += c.Tokens
			}
			assert.Equal(t, tt.wantTokens, totalTokens)
		})
	}
}

// TestContextManager_BuildContextPrompt tests context prompt building.
func TestContextManager_BuildContextPrompt(t *testing.T) {
	config := types.ContextConfig{MaxChunks: 10}
	budget := types.BudgetConfig{
		SystemPrompt: 0.20,
		Context:      0.40,
		History:      0.30,
		Response:     0.10,
	}
	tokenizer := NewMockTokenCounter(0.25)
	cm := NewContextManager(config, budget, 100000, tokenizer)

	tests := []struct {
		name            string
		chunks          []ContextChunk
		wantEmpty       bool
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:      "empty chunks returns empty string",
			chunks:    []ContextChunk{},
			wantEmpty: true,
		},
		{
			name: "single character chunk",
			chunks: []ContextChunk{
				{Content: "Alice is a brave warrior.", SourceType: "character"},
			},
			wantEmpty:    false,
			wantContains: []string{"Characters", "Alice is a brave warrior"},
		},
		{
			name: "multiple types organized correctly",
			chunks: []ContextChunk{
				{Content: "Character description", SourceType: "character"},
				{Content: "Setting description", SourceType: "setting"},
				{Content: "Plot description", SourceType: "plot"},
			},
			wantEmpty:    false,
			wantContains: []string{"Characters", "Settings", "Plot", "Relevant Context"},
		},
		{
			name: "chapter type included",
			chunks: []ContextChunk{
				{Content: "Chapter content here", SourceType: "chapter"},
			},
			wantEmpty:    false,
			wantContains: []string{"Previous Chapters", "Chapter content"},
		},
		{
			name: "preserves order of types",
			chunks: []ContextChunk{
				{Content: "Plot first", SourceType: "plot"},
				{Content: "Character second", SourceType: "character"},
				{Content: "Setting third", SourceType: "setting"},
			},
			wantEmpty:    false,
			wantContains: []string{"Characters", "Settings", "Plot"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cm.BuildContextPrompt(tt.chunks)

			if tt.wantEmpty {
				assert.Empty(t, result)
				return
			}

			assert.NotEmpty(t, result)
			for _, want := range tt.wantContains {
				assert.Contains(t, result, want)
			}
			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, result, notWant)
			}
		})
	}
}

// TestContextManager_TruncateHistory tests history truncation.
func TestContextManager_TruncateHistory(t *testing.T) {
	config := types.ContextConfig{MaxChunks: 5}
	budget := types.BudgetConfig{
		SystemPrompt: 0.20,
		Context:      0.40,
		History:      0.30,
		Response:     0.10,
	}
	// 1 token per 4 characters
	tokenizer := NewMockTokenCounter(0.25)
	cm := NewContextManager(config, budget, 100000, tokenizer)

	tests := []struct {
		name          string
		messages      []ChatMessage
		budget        int
		wantLen       int
		wantHasSystem bool
		wantLastMsg   string
	}{
		{
			name:     "empty messages returns empty",
			messages: []ChatMessage{},
			budget:   1000,
			wantLen:  0,
		},
		{
			name: "all messages fit within budget",
			messages: []ChatMessage{
				{Role: RoleUser, Content: "Hi"},
				{Role: RoleAssistant, Content: "Hello"},
			},
			budget:      1000,
			wantLen:     2,
			wantLastMsg: "Hello",
		},
		{
			name: "preserves system message",
			messages: []ChatMessage{
				{Role: RoleSystem, Content: "You are helpful."},
				{Role: RoleUser, Content: "Hello"},
				{Role: RoleAssistant, Content: "Hi there!"},
			},
			budget:        1000,
			wantLen:       3,
			wantHasSystem: true,
			wantLastMsg:   "Hi there!",
		},
		{
			name: "keeps most recent when truncating",
			messages: []ChatMessage{
				{Role: RoleSystem, Content: "System prompt"},
				{Role: RoleUser, Content: "Old message 1"},
				{Role: RoleAssistant, Content: "Old response 1"},
				{Role: RoleUser, Content: "Recent message"},
				{Role: RoleAssistant, Content: "Recent response"},
			},
			budget:        30, // Very limited budget
			wantHasSystem: true,
			wantLastMsg:   "Recent response",
		},
		{
			name: "system message counts against budget",
			messages: []ChatMessage{
				{Role: RoleSystem, Content: "A very long system message that takes many tokens"},
				{Role: RoleUser, Content: "Short"},
			},
			budget:        20,
			wantHasSystem: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cm.TruncateHistory(tt.messages, tt.budget)

			if tt.wantLen > 0 {
				assert.Len(t, result, tt.wantLen)
			}

			if tt.wantHasSystem {
				assert.Equal(t, RoleSystem, result[0].Role)
			}

			if tt.wantLastMsg != "" && len(result) > 0 {
				assert.Equal(t, tt.wantLastMsg, result[len(result)-1].Content)
			}
		})
	}
}

// TestContextManager_TruncateHistory_MaintainsOrder tests that truncation preserves message order.
func TestContextManager_TruncateHistory_MaintainsOrder(t *testing.T) {
	config := types.ContextConfig{MaxChunks: 5}
	budget := types.BudgetConfig{
		SystemPrompt: 0.20,
		Context:      0.40,
		History:      0.30,
		Response:     0.10,
	}
	tokenizer := NewMockTokenCounter(0.25)
	cm := NewContextManager(config, budget, 100000, tokenizer)

	messages := []ChatMessage{
		{Role: RoleSystem, Content: "System"},
		{Role: RoleUser, Content: "User 1"},
		{Role: RoleAssistant, Content: "Assistant 1"},
		{Role: RoleUser, Content: "User 2"},
		{Role: RoleAssistant, Content: "Assistant 2"},
	}

	result := cm.TruncateHistory(messages, 1000)

	// Verify order is preserved
	require.Len(t, result, 5)
	assert.Equal(t, RoleSystem, result[0].Role)
	assert.Equal(t, RoleUser, result[1].Role)
	assert.Equal(t, RoleAssistant, result[2].Role)
	assert.Equal(t, RoleUser, result[3].Role)
	assert.Equal(t, RoleAssistant, result[4].Role)
}

// ============================================================================
// StreamChunk Tests
// ============================================================================

// TestStreamChunk_IsComplete tests stream completion detection.
func TestStreamChunk_IsComplete(t *testing.T) {
	tests := []struct {
		name  string
		chunk StreamChunk
		want  bool
	}{
		{
			name:  "complete when Done is true",
			chunk: StreamChunk{Done: true},
			want:  true,
		},
		{
			name:  "complete when Error is set",
			chunk: StreamChunk{Error: ErrAPIError},
			want:  true,
		},
		{
			name:  "complete when both Done and Error",
			chunk: StreamChunk{Done: true, Error: ErrAPIError},
			want:  true,
		},
		{
			name:  "not complete for regular chunk",
			chunk: StreamChunk{Delta: "Hello"},
			want:  false,
		},
		{
			name:  "not complete for chunk with tool call",
			chunk: StreamChunk{ToolCall: &ToolCallDelta{Index: 0}},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.chunk.IsComplete()
			assert.Equal(t, tt.want, result)
		})
	}
}

// ============================================================================
// SystemPromptBuilder Tests
// ============================================================================

// TestSystemPromptBuilder tests the system prompt builder.
func TestSystemPromptBuilder(t *testing.T) {
	t.Run("builds prompt with all parts", func(t *testing.T) {
		builder := NewSystemPromptBuilder()

		result := builder.
			AddRole("You are a helpful writing assistant.").
			AddProjectInfo("The Dragon's Quest", "fantasy").
			AddWritingStyle(types.WritingConfig{
				Style: "descriptive",
				POV:   "third-person",
				Tense: "past",
			}).
			AddContext("Character: Alice is a brave warrior.").
			AddInstructions("Focus on dialogue.").
			Build()

		assert.Contains(t, result, "You are a helpful writing assistant.")
		assert.Contains(t, result, "fantasy novel")
		assert.Contains(t, result, "The Dragon's Quest")
		assert.Contains(t, result, "descriptive")
		assert.Contains(t, result, "third-person")
		assert.Contains(t, result, "past")
		assert.Contains(t, result, "Alice is a brave warrior")
		assert.Contains(t, result, "Focus on dialogue")
	})

	t.Run("skips empty context", func(t *testing.T) {
		builder := NewSystemPromptBuilder()

		result := builder.
			AddRole("Assistant").
			AddContext("").
			AddInstructions("Do something.").
			Build()

		assert.Contains(t, result, "Assistant")
		assert.Contains(t, result, "Do something")
		// Empty context should not add extra separators
	})

	t.Run("parts separated by double newlines", func(t *testing.T) {
		builder := NewSystemPromptBuilder()

		result := builder.
			AddRole("Role").
			AddInstructions("Instructions").
			Build()

		assert.Contains(t, result, "Role\n\nInstructions")
	})
}

// TestDefaultNovelWritingPrompt tests the default prompt.
func TestDefaultNovelWritingPrompt(t *testing.T) {
	prompt := DefaultNovelWritingPrompt()

	assert.NotEmpty(t, prompt)
	assert.Contains(t, prompt, "collaborative novel writing")
	assert.Contains(t, prompt, "characters")
	assert.Contains(t, prompt, "settings")
	assert.Contains(t, prompt, "plots")
}

// ============================================================================
// Role and FinishReason Constants Tests
// ============================================================================

// TestRoleConstants verifies role constant values.
func TestRoleConstants(t *testing.T) {
	assert.Equal(t, "system", RoleSystem)
	assert.Equal(t, "user", RoleUser)
	assert.Equal(t, "assistant", RoleAssistant)
	assert.Equal(t, "tool", RoleTool)
}

// TestFinishReasonConstants verifies finish reason constant values.
func TestFinishReasonConstants(t *testing.T) {
	assert.Equal(t, "stop", FinishReasonStop)
	assert.Equal(t, "length", FinishReasonLength)
	assert.Equal(t, "tool_calls", FinishReasonToolCalls)
	assert.Equal(t, "error", FinishReasonError)
}

// TestToolNameConstants verifies tool name constant values.
func TestToolNameConstants(t *testing.T) {
	assert.Equal(t, "suggest_plot_development", ToolSuggestPlotDevelopment)
	assert.Equal(t, "suggest_character_action", ToolSuggestCharacterAction)
	assert.Equal(t, "ask_user_clarification", ToolAskUserClarification)
	assert.Equal(t, "update_context", ToolUpdateContext)
	assert.Equal(t, "search_context", ToolSearchContext)
	assert.Equal(t, "extract_project_setup", ToolExtractProjectSetup)
}
