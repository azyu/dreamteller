// Package llm provides LLM client implementations.
package llm

import (
	"fmt"
	"strings"

	"github.com/azyu/dreamteller/pkg/types"
)

// ContextManager manages context injection for LLM prompts.
type ContextManager struct {
	config    types.ContextConfig
	budget    types.BudgetConfig
	maxTokens int
	tokenizer TokenCounter
}

// TokenCounter interface for counting tokens.
type TokenCounter interface {
	Count(text string) int
}

// NewContextManager creates a new context manager.
func NewContextManager(config types.ContextConfig, budget types.BudgetConfig, maxTokens int, tokenizer TokenCounter) *ContextManager {
	return &ContextManager{
		config:    config,
		budget:    budget,
		maxTokens: maxTokens,
		tokenizer: tokenizer,
	}
}

// ContextBudget represents token allocations for different parts of the prompt.
type ContextBudget struct {
	SystemPrompt int
	Context      int
	History      int
	Response     int
	Total        int
}

// CalculateBudget calculates token budgets based on configuration.
func (cm *ContextManager) CalculateBudget() ContextBudget {
	return ContextBudget{
		SystemPrompt: int(float64(cm.maxTokens) * cm.budget.SystemPrompt),
		Context:      int(float64(cm.maxTokens) * cm.budget.Context),
		History:      int(float64(cm.maxTokens) * cm.budget.History),
		Response:     int(float64(cm.maxTokens) * cm.budget.Response),
		Total:        cm.maxTokens,
	}
}

// ContextChunk represents a piece of context to inject.
type ContextChunk struct {
	Content    string
	SourceType string
	SourcePath string
	Score      float64
	Tokens     int
}

// SelectChunks selects chunks that fit within the context budget.
func (cm *ContextManager) SelectChunks(chunks []ContextChunk, budget int) []ContextChunk {
	var selected []ContextChunk
	usedTokens := 0

	for _, chunk := range chunks {
		if usedTokens+chunk.Tokens > budget {
			continue
		}
		if len(selected) >= cm.config.MaxChunks {
			break
		}
		selected = append(selected, chunk)
		usedTokens += chunk.Tokens
	}

	return selected
}

// BuildContextPrompt builds the context section of the system prompt.
func (cm *ContextManager) BuildContextPrompt(chunks []ContextChunk) string {
	if len(chunks) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n\n## Relevant Context\n\n")

	// Group by source type
	byType := make(map[string][]ContextChunk)
	for _, chunk := range chunks {
		byType[chunk.SourceType] = append(byType[chunk.SourceType], chunk)
	}

	// Order: characters, settings, plot, chapters
	order := []string{"character", "setting", "plot", "chapter"}
	typeNames := map[string]string{
		"character": "Characters",
		"setting":   "Settings",
		"plot":      "Plot",
		"chapter":   "Previous Chapters",
	}

	for _, sourceType := range order {
		typeChunks, ok := byType[sourceType]
		if !ok || len(typeChunks) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("### %s\n\n", typeNames[sourceType]))
		for _, chunk := range typeChunks {
			sb.WriteString(chunk.Content)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// TruncateHistory truncates conversation history to fit within budget.
func (cm *ContextManager) TruncateHistory(messages []ChatMessage, budget int) []ChatMessage {
	if len(messages) == 0 {
		return messages
	}

	// Always keep the system message if present
	var systemMsg *ChatMessage
	var history []ChatMessage

	for _, msg := range messages {
		if msg.Role == RoleSystem {
			systemMsg = &ChatMessage{
				Role:    msg.Role,
				Content: msg.Content,
			}
		} else {
			history = append(history, msg)
		}
	}

	// Calculate tokens for history (most recent first)
	usedTokens := 0
	if systemMsg != nil {
		usedTokens = cm.tokenizer.Count(systemMsg.Content)
	}

	// Keep most recent messages that fit
	var kept []ChatMessage
	for i := len(history) - 1; i >= 0; i-- {
		msgTokens := cm.tokenizer.Count(history[i].Content)
		if usedTokens+msgTokens > budget {
			break
		}
		kept = append([]ChatMessage{history[i]}, kept...)
		usedTokens += msgTokens
	}

	// Prepend system message if present
	if systemMsg != nil {
		kept = append([]ChatMessage{*systemMsg}, kept...)
	}

	return kept
}

// SummarizeHistory creates a summary of old messages to preserve context.
func (cm *ContextManager) SummarizeHistory(messages []ChatMessage, maxMessages int) (summary string, remaining []ChatMessage) {
	if len(messages) <= maxMessages {
		return "", messages
	}

	// Keep the most recent maxMessages
	remaining = messages[len(messages)-maxMessages:]

	// Summarize the older messages (deterministic; no LLM call).
	oldMessages := messages[:len(messages)-maxMessages]
	var sb strings.Builder

	for _, msg := range oldMessages {
		switch msg.Role {
		case RoleUser:
			// Extract key points from user messages
			content := truncateString(msg.Content, 100)
			sb.WriteString(fmt.Sprintf("- 사용자: %s\n", content))
		case RoleAssistant:
			// Extract key points from assistant messages
			content := truncateString(msg.Content, 100)
			sb.WriteString(fmt.Sprintf("- 어시스턴트: %s\n", content))
		}
	}

	return sb.String(), remaining
}

// truncateString truncates a string to maxLen characters.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// SystemPromptBuilder helps build the system prompt.
type SystemPromptBuilder struct {
	parts []string
}

// NewSystemPromptBuilder creates a new system prompt builder.
func NewSystemPromptBuilder() *SystemPromptBuilder {
	return &SystemPromptBuilder{
		parts: []string{},
	}
}

// AddRole adds the AI's role description.
func (b *SystemPromptBuilder) AddRole(role string) *SystemPromptBuilder {
	b.parts = append(b.parts, role)
	return b
}

// AddProjectInfo adds project information.
func (b *SystemPromptBuilder) AddProjectInfo(name, genre string) *SystemPromptBuilder {
	info := fmt.Sprintf("You are helping write a %s novel titled \"%s\".", genre, name)
	b.parts = append(b.parts, info)
	return b
}

// AddWritingStyle adds writing style guidelines.
func (b *SystemPromptBuilder) AddWritingStyle(style types.WritingConfig) *SystemPromptBuilder {
	guidelines := fmt.Sprintf(`Writing Guidelines:
- Style: %s
- Point of View: %s
- Tense: %s`, style.Style, style.POV, style.Tense)
	b.parts = append(b.parts, guidelines)
	return b
}

// AddContext adds context information.
func (b *SystemPromptBuilder) AddContext(context string) *SystemPromptBuilder {
	if context != "" {
		b.parts = append(b.parts, context)
	}
	return b
}

// AddInstructions adds specific instructions.
func (b *SystemPromptBuilder) AddInstructions(instructions string) *SystemPromptBuilder {
	b.parts = append(b.parts, instructions)
	return b
}

// Build assembles the final system prompt.
func (b *SystemPromptBuilder) Build() string {
	return strings.Join(b.parts, "\n\n")
}

// DefaultNovelWritingPrompt returns the default system prompt for novel writing.
func DefaultNovelWritingPrompt() string {
	return `You are an AI assistant specialized in collaborative novel writing. Your role is to:

1. Help develop compelling characters, settings, and plots
2. Write prose that matches the author's style and tone
3. Maintain consistency with established story elements
4. Suggest creative directions when asked
5. Provide constructive feedback on writing

When writing prose:
- Match the established writing style and voice
- Maintain continuity with previous chapters
- Keep characters behaving consistently with their established traits
- Use vivid, engaging descriptions
- Write natural, character-appropriate dialogue

When asked for suggestions:
- Consider the established story elements
- Offer multiple options when appropriate
- Explain your reasoning briefly

Always remember:
- The user is the author; you are a collaborative assistant
- Respect the user's creative vision
- Ask for clarification when needed
- Be specific in your suggestions`
}
