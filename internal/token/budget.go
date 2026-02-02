// Package token provides token counting and budget management.
package token

import (
	"github.com/azyu/dreamteller/pkg/types"
)

// ModelContextLimits maps model names to their maximum context window sizes.
var ModelContextLimits = map[string]int{
	// OpenAI models
	"gpt-4o":            128000,
	"gpt-4o-mini":       128000,
	"gpt-4-turbo":       128000,
	"gpt-4":             8192,
	"gpt-3.5-turbo":     16385,
	"gpt-3.5-turbo-16k": 16385,

	// Google Gemini models
	"gemini-2.0-flash":       1000000,
	"gemini-2.0-flash-lite":  1000000,
	"gemini-2.0-pro":         1000000,
	"gemini-1.5-pro":         2000000,
	"gemini-1.5-flash":       1000000,

	// Anthropic Claude models
	"claude-3-opus":   200000,
	"claude-3-sonnet": 200000,
	"claude-3-haiku":  200000,
}

// DefaultContextLimit is used when the model is not recognized.
const DefaultContextLimit = 8192

// DefaultBudgetRatios provides sensible default budget allocations.
var DefaultBudgetRatios = types.BudgetConfig{
	SystemPrompt: 0.20,
	Context:      0.40,
	History:      0.30,
	Response:     0.10,
}

// BudgetAllocation represents the concrete token allocations for each category.
type BudgetAllocation struct {
	SystemPrompt int
	Context      int
	History      int
	Response     int
	Total        int
}

// ContextChunk represents a piece of context with token count for budget calculations.
type ContextChunk struct {
	Content    string
	SourceType string
	SourcePath string
	Score      float64
	Tokens     int
}

// BudgetManager manages token budget policies for context window allocation.
type BudgetManager struct {
	model     string
	maxTokens int
	ratios    types.BudgetConfig
}

// NewBudgetManager creates a new BudgetManager for the specified model.
// It automatically sets appropriate context limits based on the model.
func NewBudgetManager(model string) *BudgetManager {
	maxTokens := getContextLimit(model)

	return &BudgetManager{
		model:     model,
		maxTokens: maxTokens,
		ratios:    DefaultBudgetRatios,
	}
}

// NewBudgetManagerWithRatios creates a BudgetManager with custom budget ratios.
func NewBudgetManagerWithRatios(model string, ratios types.BudgetConfig) *BudgetManager {
	maxTokens := getContextLimit(model)

	return &BudgetManager{
		model:     model,
		maxTokens: maxTokens,
		ratios:    ratios,
	}
}

// NewBudgetManagerWithConfig creates a BudgetManager with explicit max tokens and ratios.
func NewBudgetManagerWithConfig(model string, maxTokens int, ratios types.BudgetConfig) *BudgetManager {
	if maxTokens <= 0 {
		maxTokens = getContextLimit(model)
	}

	return &BudgetManager{
		model:     model,
		maxTokens: maxTokens,
		ratios:    ratios,
	}
}

// getContextLimit returns the context limit for a model, or default if unknown.
func getContextLimit(model string) int {
	if limit, ok := ModelContextLimits[model]; ok {
		return limit
	}
	return DefaultContextLimit
}

// GetBudget returns the token allocations for each category.
func (bm *BudgetManager) GetBudget() BudgetAllocation {
	return BudgetAllocation{
		SystemPrompt: int(float64(bm.maxTokens) * bm.ratios.SystemPrompt),
		Context:      int(float64(bm.maxTokens) * bm.ratios.Context),
		History:      int(float64(bm.maxTokens) * bm.ratios.History),
		Response:     int(float64(bm.maxTokens) * bm.ratios.Response),
		Total:        bm.maxTokens,
	}
}

// CanFit checks if the given token counts fit within the budget.
// Returns true if the sum of all tokens plus response budget fits in maxTokens.
func (bm *BudgetManager) CanFit(systemTokens, contextTokens, historyTokens int) bool {
	budget := bm.GetBudget()
	usedTokens := systemTokens + contextTokens + historyTokens
	availableForInput := bm.maxTokens - budget.Response

	return usedTokens <= availableForInput
}

// CanFitWithMargin checks if tokens fit with a safety margin percentage (0.0-1.0).
func (bm *BudgetManager) CanFitWithMargin(systemTokens, contextTokens, historyTokens int, margin float64) bool {
	budget := bm.GetBudget()
	usedTokens := systemTokens + contextTokens + historyTokens
	availableForInput := int(float64(bm.maxTokens-budget.Response) * (1.0 - margin))

	return usedTokens <= availableForInput
}

// RemainingForResponse calculates tokens remaining for the response.
func (bm *BudgetManager) RemainingForResponse(usedTokens int) int {
	remaining := bm.maxTokens - usedTokens

	if remaining < 0 {
		return 0
	}

	return remaining
}

// RemainingInCategory returns how many tokens are left in a specific category.
func (bm *BudgetManager) RemainingInCategory(category string, usedTokens int) int {
	budget := bm.GetBudget()

	var categoryBudget int
	switch category {
	case "system":
		categoryBudget = budget.SystemPrompt
	case "context":
		categoryBudget = budget.Context
	case "history":
		categoryBudget = budget.History
	case "response":
		categoryBudget = budget.Response
	default:
		return 0
	}

	remaining := categoryBudget - usedTokens
	if remaining < 0 {
		return 0
	}

	return remaining
}

// AdjustForContext selects chunks that fit within the context budget.
// Chunks should be pre-sorted by relevance score (highest first).
// Returns a new slice with selected chunks (immutable operation).
func (bm *BudgetManager) AdjustForContext(chunks []ContextChunk) []ContextChunk {
	budget := bm.GetBudget()
	return bm.SelectChunksWithBudget(chunks, budget.Context)
}

// SelectChunksWithBudget selects chunks that fit within a specified token budget.
// Chunks should be pre-sorted by relevance score (highest first).
// Returns a new slice with selected chunks (immutable operation).
func (bm *BudgetManager) SelectChunksWithBudget(chunks []ContextChunk, tokenBudget int) []ContextChunk {
	if len(chunks) == 0 {
		return nil
	}

	selected := make([]ContextChunk, 0, len(chunks))
	usedTokens := 0

	for _, chunk := range chunks {
		if usedTokens+chunk.Tokens > tokenBudget {
			continue
		}
		selected = append(selected, chunk)
		usedTokens += chunk.Tokens
	}

	return selected
}

// SelectChunksWithLimit selects chunks with both token and count limits.
// Returns a new slice with selected chunks (immutable operation).
func (bm *BudgetManager) SelectChunksWithLimit(chunks []ContextChunk, tokenBudget, maxChunks int) []ContextChunk {
	if len(chunks) == 0 {
		return nil
	}

	selected := make([]ContextChunk, 0, min(len(chunks), maxChunks))
	usedTokens := 0

	for _, chunk := range chunks {
		if len(selected) >= maxChunks {
			break
		}
		if usedTokens+chunk.Tokens > tokenBudget {
			continue
		}
		selected = append(selected, chunk)
		usedTokens += chunk.Tokens
	}

	return selected
}

// TotalTokens calculates the total tokens used by a slice of chunks.
func TotalTokens(chunks []ContextChunk) int {
	total := 0
	for _, chunk := range chunks {
		total += chunk.Tokens
	}
	return total
}

// Model returns the model name this budget manager was configured for.
func (bm *BudgetManager) Model() string {
	return bm.model
}

// MaxTokens returns the maximum context tokens for this manager.
func (bm *BudgetManager) MaxTokens() int {
	return bm.maxTokens
}

// Ratios returns the current budget ratios.
func (bm *BudgetManager) Ratios() types.BudgetConfig {
	return bm.ratios
}

// WithRatios returns a new BudgetManager with updated ratios (immutable operation).
func (bm *BudgetManager) WithRatios(ratios types.BudgetConfig) *BudgetManager {
	return &BudgetManager{
		model:     bm.model,
		maxTokens: bm.maxTokens,
		ratios:    ratios,
	}
}

// WithMaxTokens returns a new BudgetManager with updated max tokens (immutable operation).
func (bm *BudgetManager) WithMaxTokens(maxTokens int) *BudgetManager {
	return &BudgetManager{
		model:     bm.model,
		maxTokens: maxTokens,
		ratios:    bm.ratios,
	}
}

// ValidateRatios checks if the budget ratios are valid (sum to approximately 1.0).
func ValidateRatios(ratios types.BudgetConfig) bool {
	sum := ratios.SystemPrompt + ratios.Context + ratios.History + ratios.Response
	return sum >= 0.99 && sum <= 1.01
}

// NormalizeRatios adjusts ratios to sum exactly to 1.0.
// Returns a new BudgetConfig (immutable operation).
func NormalizeRatios(ratios types.BudgetConfig) types.BudgetConfig {
	sum := ratios.SystemPrompt + ratios.Context + ratios.History + ratios.Response
	if sum == 0 {
		return DefaultBudgetRatios
	}

	return types.BudgetConfig{
		SystemPrompt: ratios.SystemPrompt / sum,
		Context:      ratios.Context / sum,
		History:      ratios.History / sum,
		Response:     ratios.Response / sum,
	}
}
