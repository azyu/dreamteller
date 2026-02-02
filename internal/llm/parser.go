// Package llm provides LLM client implementations.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/azyu/dreamteller/pkg/types"
)

// Parser errors.
var (
	// ErrNoToolCall is returned when the AI response contains no tool calls.
	ErrNoToolCall = errors.New("no tool call in response")

	// ErrWrongTool is returned when the AI called the wrong tool.
	ErrWrongTool = errors.New("unexpected tool called")

	// ErrInvalidArguments is returned when tool call arguments are invalid.
	ErrInvalidArguments = errors.New("invalid tool call arguments")

	// ErrMissingRequiredField is returned when a required field is missing.
	ErrMissingRequiredField = errors.New("missing required field")

	// ErrEmptyPrompt is returned when the input prompt is empty.
	ErrEmptyPrompt = errors.New("prompt cannot be empty")
)

// PromptParser handles parsing of free-form prompts into structured project setup data.
type PromptParser struct {
	provider Provider
	model    string
}

// NewPromptParser creates a new PromptParser with the given provider.
func NewPromptParser(provider Provider) *PromptParser {
	return &PromptParser{
		provider: provider,
		model:    "",
	}
}

// NewPromptParserWithModel creates a new PromptParser with a specific model.
func NewPromptParserWithModel(provider Provider, model string) *PromptParser {
	return &PromptParser{
		provider: provider,
		model:    model,
	}
}

// ParseSetupPrompt parses a free-form prompt and extracts project setup information.
// It uses the AI to analyze the prompt and extract genre, characters, setting,
// plot hints, and writing style preferences.
func (p *PromptParser) ParseSetupPrompt(ctx context.Context, prompt string) (*types.ParsePromptResult, error) {
	if prompt == "" {
		return nil, ErrEmptyPrompt
	}

	systemPrompt := buildExtractionSystemPrompt()

	messages := []ChatMessage{
		NewSystemMessage(systemPrompt),
		NewUserMessage(prompt),
	}

	tools := []ToolDefinition{
		extractProjectSetupTool(),
	}

	req := ChatRequest{
		Messages:    messages,
		Tools:       tools,
		ToolChoice:  "required",
		Temperature: 0.3,
		MaxTokens:   2000,
	}

	resp, err := p.provider.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("provider error: %w", err)
	}

	return parseToolCallResponse(resp)
}

// buildExtractionSystemPrompt creates the system prompt for extraction.
func buildExtractionSystemPrompt() string {
	return `You are a specialized AI assistant for extracting structured project setup information from free-form novel descriptions.

Your task is to analyze the user's description of their novel idea and extract the following information:

1. **Genre**: Identify the primary genre (fantasy, sci-fi, romance, mystery, thriller, horror, literary fiction, historical fiction, etc.)

2. **Setting**: Extract information about:
   - Time period (modern, medieval, future, specific era, etc.)
   - Location (city, country, fictional world, etc.)
   - A detailed description of the world/environment

3. **Characters**: Identify all mentioned characters with:
   - Name (if provided, otherwise suggest a placeholder like "Unnamed Protagonist")
   - Role (protagonist, antagonist, supporting, mentor, love interest, etc.)
   - Description (appearance, background, occupation)
   - Traits (personality traits, abilities, relationships)

4. **Plot Hints**: Extract any plot elements, conflicts, themes, or story directions mentioned

5. **Writing Style**: Infer from the description:
   - Tone (serious, humorous, dark, whimsical, gritty, romantic, etc.)
   - Pacing (fast-paced action, slow character study, balanced, etc.)
   - Dialogue style preferences (formal, casual, period-appropriate, witty, etc.)
   - Vocabulary notes (simple, literary, technical jargon, archaic, etc.)

Be thorough but don't fabricate information that isn't implied by the user's description. If something is not mentioned or cannot be reasonably inferred, leave it empty or provide a sensible default based on the genre.

You MUST use the extract_project_setup tool to provide your response in a structured format.`
}

// extractProjectSetupTool returns just the extract_project_setup tool definition.
func extractProjectSetupTool() ToolDefinition {
	for _, tool := range PredefinedTools() {
		if tool.Function.Name == ToolExtractProjectSetup {
			return tool
		}
	}

	// Fallback definition if not found in predefined tools
	return ToolDefinition{
		Type: "function",
		Function: FunctionDefinition{
			Name:        ToolExtractProjectSetup,
			Description: "Extract project setup information from a user's free-form description.",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"genre": map[string]interface{}{
						"type":        "string",
						"description": "Extracted genre",
					},
					"setting": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"time_period": map[string]interface{}{"type": "string"},
							"location":    map[string]interface{}{"type": "string"},
							"description": map[string]interface{}{"type": "string"},
						},
					},
					"characters": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"name":        map[string]interface{}{"type": "string"},
								"role":        map[string]interface{}{"type": "string"},
								"description": map[string]interface{}{"type": "string"},
								"traits":      map[string]interface{}{"type": "object"},
							},
						},
					},
					"plot_hints": map[string]interface{}{
						"type":  "array",
						"items": map[string]interface{}{"type": "string"},
					},
					"style_guide": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"tone":             map[string]interface{}{"type": "string"},
							"pacing":           map[string]interface{}{"type": "string"},
							"dialogue":         map[string]interface{}{"type": "string"},
							"vocabulary_notes": map[string]interface{}{"type": "array"},
						},
					},
				},
			},
		},
	}
}

// parseToolCallResponse extracts the ParsePromptResult from the AI response.
func parseToolCallResponse(resp *ChatResponse) (*types.ParsePromptResult, error) {
	if !resp.Message.HasToolCalls() {
		return nil, ErrNoToolCall
	}

	toolCall := resp.Message.ToolCalls[0]
	if toolCall.Function.Name != ToolExtractProjectSetup {
		return nil, fmt.Errorf("%w: expected %s, got %s",
			ErrWrongTool, ToolExtractProjectSetup, toolCall.Function.Name)
	}

	return parseExtractedData(toolCall.Function.Arguments)
}

// rawExtractedData matches the JSON structure from the AI tool call.
type rawExtractedData struct {
	Genre   string `json:"genre"`
	Setting struct {
		TimePeriod  string `json:"time_period"`
		Location    string `json:"location"`
		Description string `json:"description"`
	} `json:"setting"`
	Characters []struct {
		Name        string            `json:"name"`
		Role        string            `json:"role"`
		Description string            `json:"description"`
		Traits      map[string]string `json:"traits"`
	} `json:"characters"`
	PlotHints  []string `json:"plot_hints"`
	StyleGuide struct {
		Tone       string   `json:"tone"`
		Pacing     string   `json:"pacing"`
		Dialogue   string   `json:"dialogue"`
		Vocabulary []string `json:"vocabulary_notes"`
	} `json:"style_guide"`
}

// parseExtractedData parses the JSON arguments from the tool call.
func parseExtractedData(arguments string) (*types.ParsePromptResult, error) {
	var raw rawExtractedData
	if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArguments, err)
	}

	result := &types.ParsePromptResult{
		Genre: raw.Genre,
		Setting: types.SettingInfo{
			TimePeriod:  raw.Setting.TimePeriod,
			Location:    raw.Setting.Location,
			Description: raw.Setting.Description,
		},
		Characters: make([]types.CharacterInfo, 0, len(raw.Characters)),
		PlotHints:  raw.PlotHints,
		StyleGuide: types.StyleInfo{
			Tone:       raw.StyleGuide.Tone,
			Pacing:     raw.StyleGuide.Pacing,
			Dialogue:   raw.StyleGuide.Dialogue,
			Vocabulary: raw.StyleGuide.Vocabulary,
		},
	}

	for _, c := range raw.Characters {
		charInfo := types.CharacterInfo{
			Name:        c.Name,
			Role:        c.Role,
			Description: c.Description,
			Traits:      c.Traits,
		}
		if charInfo.Traits == nil {
			charInfo.Traits = make(map[string]string)
		}
		result.Characters = append(result.Characters, charInfo)
	}

	if result.PlotHints == nil {
		result.PlotHints = []string{}
	}

	if result.StyleGuide.Vocabulary == nil {
		result.StyleGuide.Vocabulary = []string{}
	}

	if err := validateResult(result); err != nil {
		return nil, err
	}

	return result, nil
}

// validateResult checks that required fields are present.
func validateResult(result *types.ParsePromptResult) error {
	if result.Genre == "" {
		return fmt.Errorf("%w: genre", ErrMissingRequiredField)
	}
	return nil
}

// ParsePromptFromFile reads a prompt from a file and parses it.
func ParsePromptFromFile(parser *PromptParser, filepath string) (*types.ParsePromptResult, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompt file: %w", err)
	}

	prompt := string(content)
	if prompt == "" {
		return nil, ErrEmptyPrompt
	}

	return parser.ParseSetupPrompt(context.Background(), prompt)
}

// ParsePromptFromFileWithContext reads a prompt from a file and parses it with context.
func ParsePromptFromFileWithContext(ctx context.Context, parser *PromptParser, filepath string) (*types.ParsePromptResult, error) {
	content, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompt file: %w", err)
	}

	prompt := string(content)
	if prompt == "" {
		return nil, ErrEmptyPrompt
	}

	return parser.ParseSetupPrompt(ctx, prompt)
}
