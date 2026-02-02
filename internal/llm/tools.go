// Package llm provides LLM client implementations.
package llm

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Tool names for AI suggestions.
const (
	ToolSuggestPlotDevelopment   = "suggest_plot_development"
	ToolSuggestCharacterAction   = "suggest_character_action"
	ToolAskUserClarification     = "ask_user_clarification"
	ToolUpdateContext            = "update_context"
	ToolSearchContext            = "search_context"
	ToolExtractProjectSetup      = "extract_project_setup"
)

// PredefinedTools returns the tool definitions for novel writing.
func PredefinedTools() []ToolDefinition {
	return []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        ToolSuggestPlotDevelopment,
				Description: "Suggest plot development ideas based on current story context. Use this when the user needs ideas for advancing the story.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"suggestions": map[string]interface{}{
							"type":        "array",
							"description": "List of plot development suggestions",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"title": map[string]interface{}{
										"type":        "string",
										"description": "Short title for the suggestion",
									},
									"description": map[string]interface{}{
										"type":        "string",
										"description": "Detailed description of the plot development",
									},
									"impact": map[string]interface{}{
										"type":        "string",
										"description": "How this affects the story",
									},
								},
								"required": []string{"title", "description"},
							},
						},
					},
					"required": []string{"suggestions"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        ToolSuggestCharacterAction,
				Description: "Suggest character actions or behaviors based on their personality and the current situation.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"character": map[string]interface{}{
							"type":        "string",
							"description": "Name of the character",
						},
						"actions": map[string]interface{}{
							"type":        "array",
							"description": "List of suggested actions",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"action": map[string]interface{}{
										"type":        "string",
										"description": "The action the character could take",
									},
									"motivation": map[string]interface{}{
										"type":        "string",
										"description": "Why this action fits the character",
									},
									"dialogue": map[string]interface{}{
										"type":        "string",
										"description": "Optional dialogue for this action",
									},
								},
								"required": []string{"action", "motivation"},
							},
						},
					},
					"required": []string{"character", "actions"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        ToolAskUserClarification,
				Description: "Ask the user for clarification when something is unclear or when a decision is needed.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"question": map[string]interface{}{
							"type":        "string",
							"description": "The question to ask the user",
						},
						"options": map[string]interface{}{
							"type":        "array",
							"description": "Optional list of suggested answers",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
						"context": map[string]interface{}{
							"type":        "string",
							"description": "Additional context for why this question is being asked",
						},
					},
					"required": []string{"question"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        ToolUpdateContext,
				Description: "Suggest updates to context files (characters, settings, plot). Changes must be approved by the user.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"file_type": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"character", "setting", "plot"},
							"description": "Type of context file to update",
						},
						"file_name": map[string]interface{}{
							"type":        "string",
							"description": "Name of the file (without path or extension)",
						},
						"operation": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"create", "update", "append"},
							"description": "Type of operation",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The content to write or append",
						},
						"reason": map[string]interface{}{
							"type":        "string",
							"description": "Why this update is suggested",
						},
					},
					"required": []string{"file_type", "file_name", "operation", "content", "reason"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        ToolSearchContext,
				Description: "Search through context files to find relevant information for the current conversation.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Search query",
						},
						"filter_type": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"all", "character", "setting", "plot", "chapter"},
							"description": "Filter by content type",
						},
					},
					"required": []string{"query"},
				},
			},
		},
		{
			Type: "function",
			Function: FunctionDefinition{
				Name:        ToolExtractProjectSetup,
				Description: "Extract project setup information from a user's free-form description. Used for one-shot project creation.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"genre": map[string]interface{}{
							"type":        "string",
							"description": "Extracted genre (fantasy, sci-fi, romance, mystery, thriller, etc.)",
						},
						"setting": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"time_period": map[string]interface{}{
									"type":        "string",
									"description": "Time period (modern, medieval, future, etc.)",
								},
								"location": map[string]interface{}{
									"type":        "string",
									"description": "Primary location or world",
								},
								"description": map[string]interface{}{
									"type":        "string",
									"description": "Detailed setting description",
								},
							},
						},
						"characters": map[string]interface{}{
							"type":  "array",
							"items": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"name": map[string]interface{}{
										"type":        "string",
										"description": "Character name",
									},
									"role": map[string]interface{}{
										"type":        "string",
										"description": "Role (protagonist, antagonist, supporting)",
									},
									"description": map[string]interface{}{
										"type":        "string",
										"description": "Character description",
									},
									"traits": map[string]interface{}{
										"type":        "object",
										"description": "Character traits (blood type, birthday, etc.)",
										"additionalProperties": map[string]interface{}{
											"type": "string",
										},
									},
								},
								"required": []string{"name"},
							},
						},
						"plot_hints": map[string]interface{}{
							"type":        "array",
							"description": "Extracted plot ideas or hints",
							"items": map[string]interface{}{
								"type": "string",
							},
						},
						"style_guide": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"tone": map[string]interface{}{
									"type":        "string",
									"description": "Overall tone (serious, humorous, dark, etc.)",
								},
								"pacing": map[string]interface{}{
									"type":        "string",
									"description": "Pacing preference (fast, slow, balanced)",
								},
								"dialogue": map[string]interface{}{
									"type":        "string",
									"description": "Dialogue style preferences",
								},
								"vocabulary_notes": map[string]interface{}{
									"type":        "array",
									"description": "Notes about vocabulary or writing style",
									"items": map[string]interface{}{
										"type": "string",
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ToolResult represents the result of executing a tool.
type ToolResult struct {
	ToolCallID string
	Content    string
	IsError    bool
}

// PlotSuggestion represents a plot development suggestion.
type PlotSuggestion struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Impact      string `json:"impact,omitempty"`
}

// CharacterActionSuggestion represents a character action suggestion.
type CharacterActionSuggestion struct {
	Character string            `json:"character"`
	Actions   []CharacterAction `json:"actions"`
}

// CharacterAction represents a single character action.
type CharacterAction struct {
	Action     string `json:"action"`
	Motivation string `json:"motivation"`
	Dialogue   string `json:"dialogue,omitempty"`
}

// ClarificationQuestion represents a question for the user.
type ClarificationQuestion struct {
	Question string   `json:"question"`
	Options  []string `json:"options,omitempty"`
	Context  string   `json:"context,omitempty"`
}

// ContextUpdate represents a context file update request.
type ContextUpdate struct {
	FileType  string `json:"file_type"`
	FileName  string `json:"file_name"`
	Operation string `json:"operation"`
	Content   string `json:"content"`
	Reason    string `json:"reason"`
}

// SearchQuery represents a context search query.
type SearchQuery struct {
	Query      string `json:"query"`
	FilterType string `json:"filter_type,omitempty"`
}

// ParseToolCall parses a tool call's arguments into the appropriate struct.
func ParseToolCall(call ToolCall) (interface{}, error) {
	switch call.Function.Name {
	case ToolSuggestPlotDevelopment:
		var result struct {
			Suggestions []PlotSuggestion `json:"suggestions"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &result); err != nil {
			return nil, fmt.Errorf("failed to parse plot suggestions: %w", err)
		}
		return result.Suggestions, nil

	case ToolSuggestCharacterAction:
		var result CharacterActionSuggestion
		if err := json.Unmarshal([]byte(call.Function.Arguments), &result); err != nil {
			return nil, fmt.Errorf("failed to parse character actions: %w", err)
		}
		return result, nil

	case ToolAskUserClarification:
		var result ClarificationQuestion
		if err := json.Unmarshal([]byte(call.Function.Arguments), &result); err != nil {
			return nil, fmt.Errorf("failed to parse clarification question: %w", err)
		}
		return result, nil

	case ToolUpdateContext:
		var result ContextUpdate
		if err := json.Unmarshal([]byte(call.Function.Arguments), &result); err != nil {
			return nil, fmt.Errorf("failed to parse context update: %w", err)
		}
		return result, nil

	case ToolSearchContext:
		var result SearchQuery
		if err := json.Unmarshal([]byte(call.Function.Arguments), &result); err != nil {
			return nil, fmt.Errorf("failed to parse search query: %w", err)
		}
		return result, nil

	case ToolExtractProjectSetup:
		var result struct {
			Genre      string          `json:"genre"`
			Setting    interface{}     `json:"setting"`
			Characters []interface{}   `json:"characters"`
			PlotHints  []string        `json:"plot_hints"`
			StyleGuide interface{}     `json:"style_guide"`
		}
		if err := json.Unmarshal([]byte(call.Function.Arguments), &result); err != nil {
			return nil, fmt.Errorf("failed to parse project setup: %w", err)
		}
		return result, nil

	default:
		return nil, errors.New("unknown tool: " + call.Function.Name)
	}
}

// ValidateContextUpdatePath validates that a context update path is allowed.
func ValidateContextUpdatePath(fileType, fileName string) error {
	// Whitelist of allowed file types
	allowedTypes := map[string]bool{
		"character": true,
		"setting":   true,
		"plot":      true,
	}

	if !allowedTypes[fileType] {
		return fmt.Errorf("invalid file type: %s", fileType)
	}

	// Validate filename (no path traversal)
	if fileName == "" || fileName == "." || fileName == ".." {
		return fmt.Errorf("invalid file name: %s", fileName)
	}

	for _, c := range fileName {
		if c == '/' || c == '\\' || c == ':' {
			return fmt.Errorf("invalid character in file name: %c", c)
		}
	}

	return nil
}
