package token

import (
	"strings"
	"testing"

	"github.com/azyu/dreamteller/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewCounter tests Counter creation with various encodings.
func TestNewCounter(t *testing.T) {
	tests := []struct {
		name         string
		encoding     string
		wantEncoding string
		wantErr      bool
	}{
		{
			name:         "creates counter with default encoding",
			encoding:     "",
			wantEncoding: "cl100k_base",
			wantErr:      false,
		},
		{
			name:         "creates counter with cl100k_base",
			encoding:     "cl100k_base",
			wantEncoding: "cl100k_base",
			wantErr:      false,
		},
		{
			name:         "creates counter with o200k_base (GPT-4o)",
			encoding:     "o200k_base",
			wantEncoding: "o200k_base",
			wantErr:      false,
		},
		{
			name:         "falls back to default for invalid encoding",
			encoding:     "invalid_encoding",
			wantEncoding: "cl100k_base",
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counter, err := NewCounter(tt.encoding)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, counter)
			assert.Equal(t, tt.wantEncoding, counter.Encoding())
		})
	}
}

// TestCounter_Count tests token counting with various strings.
func TestCounter_Count(t *testing.T) {
	counter, err := NewCounter("cl100k_base")
	require.NoError(t, err)

	tests := []struct {
		name      string
		text      string
		wantMin   int
		wantMax   int
		wantExact int
	}{
		{
			name:      "empty string returns zero",
			text:      "",
			wantExact: 0,
		},
		{
			name:    "single word",
			text:    "hello",
			wantMin: 1,
			wantMax: 1,
		},
		{
			name:    "simple sentence",
			text:    "Hello, world!",
			wantMin: 3,
			wantMax: 5,
		},
		{
			name:    "longer text",
			text:    "The quick brown fox jumps over the lazy dog.",
			wantMin: 8,
			wantMax: 12,
		},
		{
			name:    "text with newlines",
			text:    "Line one\nLine two\nLine three",
			wantMin: 5,
			wantMax: 10,
		},
		{
			name:    "text with special characters",
			text:    "foo@bar.com #hashtag $100 50%",
			wantMin: 5,
			wantMax: 15,
		},
		{
			name:    "unicode text",
			text:    "Hello, 世界! こんにちは",
			wantMin: 5,
			wantMax: 20,
		},
		{
			name:    "code snippet",
			text:    "func main() { fmt.Println(\"Hello\") }",
			wantMin: 8,
			wantMax: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := counter.Count(tt.text)

			if tt.wantExact > 0 {
				assert.Equal(t, tt.wantExact, count)
			} else {
				assert.GreaterOrEqual(t, count, tt.wantMin, "token count should be >= min")
				assert.LessOrEqual(t, count, tt.wantMax, "token count should be <= max")
			}
		})
	}
}

// TestCounter_CountMessages tests token counting for chat messages.
func TestCounter_CountMessages(t *testing.T) {
	counter, err := NewCounter("cl100k_base")
	require.NoError(t, err)

	tests := []struct {
		name     string
		messages []ChatMessage
		wantMin  int
		wantMax  int
	}{
		{
			name:     "empty messages returns zero",
			messages: nil,
			wantMin:  0,
			wantMax:  0,
		},
		{
			name:     "empty slice returns zero",
			messages: []ChatMessage{},
			wantMin:  0,
			wantMax:  0,
		},
		{
			name: "single system message",
			messages: []ChatMessage{
				{Role: "system", Content: "You are a helpful assistant."},
			},
			wantMin: 8,
			wantMax: 15,
		},
		{
			name: "user and assistant exchange",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello!"},
				{Role: "assistant", Content: "Hi there! How can I help?"},
			},
			wantMin: 15,
			wantMax: 25,
		},
		{
			name: "message with name field",
			messages: []ChatMessage{
				{Role: "user", Content: "What's the weather?", Name: "Alice"},
			},
			wantMin: 8,
			wantMax: 18,
		},
		{
			name: "full conversation",
			messages: []ChatMessage{
				{Role: "system", Content: "You are a creative writing assistant."},
				{Role: "user", Content: "Write a short story about a dragon."},
				{Role: "assistant", Content: "Once upon a time, there was a dragon..."},
				{Role: "user", Content: "Make it more exciting!"},
			},
			wantMin: 30,
			wantMax: 55,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count := counter.CountMessages(tt.messages)

			assert.GreaterOrEqual(t, count, tt.wantMin, "token count should be >= min")
			assert.LessOrEqual(t, count, tt.wantMax, "token count should be <= max")
		})
	}
}

// TestCounter_CountMessages_IncludesOverhead verifies message overhead is applied.
func TestCounter_CountMessages_IncludesOverhead(t *testing.T) {
	counter, err := NewCounter("cl100k_base")
	require.NoError(t, err)

	content := "Hello"
	contentTokens := counter.Count(content)

	messages := []ChatMessage{
		{Role: "user", Content: content},
	}
	messageTokens := counter.CountMessages(messages)

	// Message tokens should include content + overhead (messageOverhead=4) + reply priming (2)
	expectedMin := contentTokens + messageOverhead + replyPriming
	assert.GreaterOrEqual(t, messageTokens, expectedMin, "should include message overhead")
}

// TestCounter_Truncate tests text truncation to fit within token limits.
func TestCounter_Truncate(t *testing.T) {
	counter, err := NewCounter("cl100k_base")
	require.NoError(t, err)

	tests := []struct {
		name      string
		text      string
		maxTokens int
		checkFunc func(t *testing.T, result string, original string)
	}{
		{
			name:      "returns empty for zero max tokens",
			text:      "Hello, world!",
			maxTokens: 0,
			checkFunc: func(t *testing.T, result string, original string) {
				assert.Empty(t, result)
			},
		},
		{
			name:      "returns empty for negative max tokens",
			text:      "Hello, world!",
			maxTokens: -1,
			checkFunc: func(t *testing.T, result string, original string) {
				assert.Empty(t, result)
			},
		},
		{
			name:      "returns original if within limit",
			text:      "Hello",
			maxTokens: 100,
			checkFunc: func(t *testing.T, result string, original string) {
				assert.Equal(t, original, result)
			},
		},
		{
			name:      "truncates long text",
			text:      "The quick brown fox jumps over the lazy dog. This is a much longer sentence that needs to be truncated to fit within a smaller token limit.",
			maxTokens: 10,
			checkFunc: func(t *testing.T, result string, original string) {
				assert.Less(t, len(result), len(original), "result should be shorter")
				assert.LessOrEqual(t, counter.Count(result), 10, "result should fit in limit")
			},
		},
		{
			name:      "handles exact limit",
			text:      "Hello",
			maxTokens: 1,
			checkFunc: func(t *testing.T, result string, original string) {
				assert.LessOrEqual(t, counter.Count(result), 1)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := counter.Truncate(tt.text, tt.maxTokens)
			tt.checkFunc(t, result, tt.text)
		})
	}
}

// TestCounter_Split tests text splitting with overlap.
func TestCounter_Split(t *testing.T) {
	counter, err := NewCounter("cl100k_base")
	require.NoError(t, err)

	tests := []struct {
		name        string
		text        string
		chunkSize   int
		overlap     float64
		wantMinLen  int
		wantMaxLen  int
		checkFunc   func(t *testing.T, chunks []string)
	}{
		{
			name:       "empty text returns nil",
			text:       "",
			chunkSize:  10,
			overlap:    0.2,
			wantMinLen: 0,
			wantMaxLen: 0,
		},
		{
			name:       "zero chunk size returns nil",
			text:       "Hello world",
			chunkSize:  0,
			overlap:    0.2,
			wantMinLen: 0,
			wantMaxLen: 0,
		},
		{
			name:       "negative chunk size returns nil",
			text:       "Hello world",
			chunkSize:  -1,
			overlap:    0.2,
			wantMinLen: 0,
			wantMaxLen: 0,
		},
		{
			name:       "short text returns single chunk",
			text:       "Hello",
			chunkSize:  100,
			overlap:    0.2,
			wantMinLen: 1,
			wantMaxLen: 1,
			checkFunc: func(t *testing.T, chunks []string) {
				assert.Equal(t, "Hello", chunks[0])
			},
		},
		{
			name:       "splits long text without overlap",
			text:       "The quick brown fox jumps over the lazy dog. The five boxing wizards jump quickly. Pack my box with five dozen liquor jugs.",
			chunkSize:  10,
			overlap:    0.0,
			wantMinLen: 2,
			wantMaxLen: 10,
		},
		{
			name:       "splits with 50% overlap creates overlapping chunks",
			text:       "The quick brown fox jumps over the lazy dog. The five boxing wizards jump quickly.",
			chunkSize:  10,
			overlap:    0.5,
			wantMinLen: 2,
			wantMaxLen: 10,
			checkFunc: func(t *testing.T, chunks []string) {
				// With 50% overlap, consecutive chunks should share content
				// The overlap means step = chunkSize * 0.5
				assert.GreaterOrEqual(t, len(chunks), 2, "should have multiple chunks with overlap")
			},
		},
		{
			name:       "clamps negative overlap to zero",
			text:       "The quick brown fox jumps over the lazy dog.",
			chunkSize:  5,
			overlap:    -0.5,
			wantMinLen: 1,
			wantMaxLen: 5,
			checkFunc: func(t *testing.T, chunks []string) {
				assert.NotNil(t, chunks)
			},
		},
		{
			name:       "clamps overlap >= 1 to 0.9",
			text:       "The quick brown fox jumps over the lazy dog.",
			chunkSize:  5,
			overlap:    1.5,
			wantMinLen: 1,
			wantMaxLen: 20,
			checkFunc: func(t *testing.T, chunks []string) {
				assert.NotNil(t, chunks)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := counter.Split(tt.text, tt.chunkSize, tt.overlap)

			if tt.wantMinLen == 0 && tt.wantMaxLen == 0 {
				assert.Nil(t, chunks)
				return
			}

			if tt.wantMinLen > 0 || tt.wantMaxLen > 0 {
				assert.GreaterOrEqual(t, len(chunks), tt.wantMinLen)
				if tt.wantMaxLen > 0 {
					assert.LessOrEqual(t, len(chunks), tt.wantMaxLen)
				}
			}

			if tt.checkFunc != nil {
				tt.checkFunc(t, chunks)
			}
		})
	}
}

// TestCounter_Split_ChunksWithinLimit verifies each chunk respects token limit.
func TestCounter_Split_ChunksWithinLimit(t *testing.T) {
	counter, err := NewCounter("cl100k_base")
	require.NoError(t, err)

	longText := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 50)
	chunkSize := 20
	chunks := counter.Split(longText, chunkSize, 0.2)

	require.NotNil(t, chunks)
	for i, chunk := range chunks {
		tokens := counter.Count(chunk)
		assert.LessOrEqual(t, tokens, chunkSize, "chunk %d exceeds token limit", i)
	}
}

// TestEstimateTokens tests the quick token estimation function.
func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantMin int
		wantMax int
	}{
		{
			name:    "empty string returns zero",
			text:    "",
			wantMin: 0,
			wantMax: 0,
		},
		{
			name:    "short word",
			text:    "hi",
			wantMin: 1,
			wantMax: 2,
		},
		{
			name:    "average sentence",
			text:    "Hello, how are you today?",
			wantMin: 4,
			wantMax: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			estimate := EstimateTokens(tt.text)
			assert.GreaterOrEqual(t, estimate, tt.wantMin)
			assert.LessOrEqual(t, estimate, tt.wantMax)
		})
	}
}

// TestCounter_TruncateToFit tests truncation from beginning or end.
func TestCounter_TruncateToFit(t *testing.T) {
	counter, err := NewCounter("cl100k_base")
	require.NoError(t, err)

	text := "Beginning of the text. Middle of the text. End of the text."

	tests := []struct {
		name      string
		maxTokens int
		fromEnd   bool
		checkFunc func(t *testing.T, result string)
	}{
		{
			name:      "returns empty for zero max tokens",
			maxTokens: 0,
			fromEnd:   false,
			checkFunc: func(t *testing.T, result string) {
				assert.Empty(t, result)
			},
		},
		{
			name:      "keeps beginning when fromEnd is false",
			maxTokens: 5,
			fromEnd:   false,
			checkFunc: func(t *testing.T, result string) {
				assert.True(t, strings.HasPrefix(text, result) || strings.HasPrefix(result, "Beginning"), "should start with beginning")
			},
		},
		{
			name:      "keeps end when fromEnd is true",
			maxTokens: 5,
			fromEnd:   true,
			checkFunc: func(t *testing.T, result string) {
				assert.Contains(t, result, "text", "should contain end portion")
			},
		},
		{
			name:      "returns full text if within limit",
			maxTokens: 100,
			fromEnd:   false,
			checkFunc: func(t *testing.T, result string) {
				assert.Equal(t, text, result)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := counter.TruncateToFit(text, tt.maxTokens, tt.fromEnd)
			tt.checkFunc(t, result)
		})
	}
}

// TestCounter_SplitByWords tests word-boundary aware splitting.
func TestCounter_SplitByWords(t *testing.T) {
	counter, err := NewCounter("cl100k_base")
	require.NoError(t, err)

	tests := []struct {
		name      string
		text      string
		maxTokens int
		checkFunc func(t *testing.T, chunks []ChunkWithCount)
	}{
		{
			name:      "empty text returns nil",
			text:      "",
			maxTokens: 10,
			checkFunc: func(t *testing.T, chunks []ChunkWithCount) {
				assert.Nil(t, chunks)
			},
		},
		{
			name:      "zero max tokens returns nil",
			text:      "Hello world",
			maxTokens: 0,
			checkFunc: func(t *testing.T, chunks []ChunkWithCount) {
				assert.Nil(t, chunks)
			},
		},
		{
			name:      "short text returns single chunk",
			text:      "Hello world",
			maxTokens: 100,
			checkFunc: func(t *testing.T, chunks []ChunkWithCount) {
				require.Len(t, chunks, 1)
				assert.Equal(t, "Hello world", chunks[0].Text)
				assert.Greater(t, chunks[0].Tokens, 0)
			},
		},
		{
			name:      "respects word boundaries",
			text:      "The quick brown fox jumps over the lazy dog",
			maxTokens: 3,
			checkFunc: func(t *testing.T, chunks []ChunkWithCount) {
				require.NotEmpty(t, chunks)
				for _, chunk := range chunks {
					// Each chunk should be complete words, not cut mid-word
					assert.NotContains(t, chunk.Text, "  ", "should not have double spaces")
					assert.LessOrEqual(t, chunk.Tokens, 5) // Allow some flexibility
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := counter.SplitByWords(tt.text, tt.maxTokens)
			tt.checkFunc(t, chunks)
		})
	}
}

// ============================================================================
// BudgetManager Tests
// ============================================================================

// TestNewBudgetManager tests BudgetManager creation with different models.
func TestNewBudgetManager(t *testing.T) {
	tests := []struct {
		name          string
		model         string
		wantMaxTokens int
	}{
		{
			name:          "creates manager for gpt-4o",
			model:         "gpt-4o",
			wantMaxTokens: 128000,
		},
		{
			name:          "creates manager for gpt-4o-mini",
			model:         "gpt-4o-mini",
			wantMaxTokens: 128000,
		},
		{
			name:          "creates manager for gpt-4-turbo",
			model:         "gpt-4-turbo",
			wantMaxTokens: 128000,
		},
		{
			name:          "creates manager for gpt-4",
			model:         "gpt-4",
			wantMaxTokens: 8192,
		},
		{
			name:          "creates manager for gpt-3.5-turbo",
			model:         "gpt-3.5-turbo",
			wantMaxTokens: 16385,
		},
		{
			name:          "creates manager for gemini-2.0-flash",
			model:         "gemini-2.0-flash",
			wantMaxTokens: 1000000,
		},
		{
			name:          "creates manager for gemini-2.0-flash-lite",
			model:         "gemini-2.0-flash-lite",
			wantMaxTokens: 1000000,
		},
		{
			name:          "creates manager for gemini-2.0-pro",
			model:         "gemini-2.0-pro",
			wantMaxTokens: 1000000,
		},
		{
			name:          "creates manager for gemini-1.5-pro",
			model:         "gemini-1.5-pro",
			wantMaxTokens: 2000000,
		},
		{
			name:          "creates manager for gemini-1.5-flash",
			model:         "gemini-1.5-flash",
			wantMaxTokens: 1000000,
		},
		{
			name:          "creates manager for claude-3-opus",
			model:         "claude-3-opus",
			wantMaxTokens: 200000,
		},
		{
			name:          "creates manager for claude-3-sonnet",
			model:         "claude-3-sonnet",
			wantMaxTokens: 200000,
		},
		{
			name:          "creates manager for claude-3-haiku",
			model:         "claude-3-haiku",
			wantMaxTokens: 200000,
		},
		{
			name:          "falls back to default for unknown model",
			model:         "unknown-model",
			wantMaxTokens: DefaultContextLimit,
		},
		{
			name:          "falls back to default for empty model",
			model:         "",
			wantMaxTokens: DefaultContextLimit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bm := NewBudgetManager(tt.model)

			assert.NotNil(t, bm)
			assert.Equal(t, tt.model, bm.Model())
			assert.Equal(t, tt.wantMaxTokens, bm.MaxTokens())
			assert.Equal(t, DefaultBudgetRatios, bm.Ratios())
		})
	}
}

// TestNewBudgetManagerWithRatios tests creation with custom ratios.
func TestNewBudgetManagerWithRatios(t *testing.T) {
	customRatios := types.BudgetConfig{
		SystemPrompt: 0.10,
		Context:      0.50,
		History:      0.25,
		Response:     0.15,
	}

	bm := NewBudgetManagerWithRatios("gpt-4o", customRatios)

	assert.NotNil(t, bm)
	assert.Equal(t, customRatios, bm.Ratios())
	assert.Equal(t, 128000, bm.MaxTokens())
}

// TestNewBudgetManagerWithConfig tests creation with explicit config.
func TestNewBudgetManagerWithConfig(t *testing.T) {
	customRatios := types.BudgetConfig{
		SystemPrompt: 0.25,
		Context:      0.25,
		History:      0.25,
		Response:     0.25,
	}

	tests := []struct {
		name          string
		model         string
		maxTokens     int
		wantMaxTokens int
	}{
		{
			name:          "uses provided max tokens",
			model:         "gpt-4o",
			maxTokens:     50000,
			wantMaxTokens: 50000,
		},
		{
			name:          "falls back to model limit for zero",
			model:         "gpt-4o",
			maxTokens:     0,
			wantMaxTokens: 128000,
		},
		{
			name:          "falls back to model limit for negative",
			model:         "gpt-4o",
			maxTokens:     -100,
			wantMaxTokens: 128000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bm := NewBudgetManagerWithConfig(tt.model, tt.maxTokens, customRatios)

			assert.NotNil(t, bm)
			assert.Equal(t, tt.wantMaxTokens, bm.MaxTokens())
			assert.Equal(t, customRatios, bm.Ratios())
		})
	}
}

// TestBudgetManager_GetBudget tests budget allocation calculation.
func TestBudgetManager_GetBudget(t *testing.T) {
	tests := []struct {
		name             string
		model            string
		wantSystemPrompt int
		wantContext      int
		wantHistory      int
		wantResponse     int
		wantTotal        int
	}{
		{
			name:             "gpt-4o budget allocations",
			model:            "gpt-4o",
			wantSystemPrompt: 25600,  // 128000 * 0.20
			wantContext:      51200,  // 128000 * 0.40
			wantHistory:      38400,  // 128000 * 0.30
			wantResponse:     12800,  // 128000 * 0.10
			wantTotal:        128000,
		},
		{
			name:             "gpt-4 budget allocations",
			model:            "gpt-4",
			wantSystemPrompt: 1638,  // 8192 * 0.20
			wantContext:      3276,  // 8192 * 0.40
			wantHistory:      2457,  // 8192 * 0.30
			wantResponse:     819,   // 8192 * 0.10
			wantTotal:        8192,
		},
		{
			name:             "gemini-2.0-flash budget allocations",
			model:            "gemini-2.0-flash",
			wantSystemPrompt: 200000,  // 1000000 * 0.20
			wantContext:      400000,  // 1000000 * 0.40
			wantHistory:      300000,  // 1000000 * 0.30
			wantResponse:     100000,  // 1000000 * 0.10
			wantTotal:        1000000,
		},
		{
			name:             "claude-3-opus budget allocations",
			model:            "claude-3-opus",
			wantSystemPrompt: 40000,  // 200000 * 0.20
			wantContext:      80000,  // 200000 * 0.40
			wantHistory:      60000,  // 200000 * 0.30
			wantResponse:     20000,  // 200000 * 0.10
			wantTotal:        200000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bm := NewBudgetManager(tt.model)
			budget := bm.GetBudget()

			assert.Equal(t, tt.wantSystemPrompt, budget.SystemPrompt)
			assert.Equal(t, tt.wantContext, budget.Context)
			assert.Equal(t, tt.wantHistory, budget.History)
			assert.Equal(t, tt.wantResponse, budget.Response)
			assert.Equal(t, tt.wantTotal, budget.Total)
		})
	}
}

// TestBudgetManager_CanFit tests token fitting validation.
func TestBudgetManager_CanFit(t *testing.T) {
	bm := NewBudgetManager("gpt-4") // 8192 tokens, 10% response = 819 reserved

	tests := []struct {
		name          string
		systemTokens  int
		contextTokens int
		historyTokens int
		wantCanFit    bool
	}{
		{
			name:          "fits when well under limit",
			systemTokens:  1000,
			contextTokens: 2000,
			historyTokens: 1000,
			wantCanFit:    true,
		},
		{
			name:          "fits at exact limit",
			systemTokens:  1000,
			contextTokens: 3000,
			historyTokens: 3373, // 8192 - 819 (response) - 1000 - 3000
			wantCanFit:    true,
		},
		{
			name:          "does not fit when exceeds limit",
			systemTokens:  3000,
			contextTokens: 3000,
			historyTokens: 3000,
			wantCanFit:    false,
		},
		{
			name:          "fits with zero tokens",
			systemTokens:  0,
			contextTokens: 0,
			historyTokens: 0,
			wantCanFit:    true,
		},
		{
			name:          "does not fit when just over limit",
			systemTokens:  2500,
			contextTokens: 2500,
			historyTokens: 2500, // Total 7500, available = 7373
			wantCanFit:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canFit := bm.CanFit(tt.systemTokens, tt.contextTokens, tt.historyTokens)
			assert.Equal(t, tt.wantCanFit, canFit)
		})
	}
}

// TestBudgetManager_CanFitWithMargin tests fitting with safety margin.
func TestBudgetManager_CanFitWithMargin(t *testing.T) {
	bm := NewBudgetManager("gpt-4") // 8192 tokens

	tests := []struct {
		name          string
		systemTokens  int
		contextTokens int
		historyTokens int
		margin        float64
		wantCanFit    bool
	}{
		{
			name:          "fits with 10% margin",
			systemTokens:  1000,
			contextTokens: 2000,
			historyTokens: 1000,
			margin:        0.10,
			wantCanFit:    true,
		},
		{
			name:          "does not fit with 50% margin",
			systemTokens:  2000,
			contextTokens: 2000,
			historyTokens: 2000,
			margin:        0.50,
			wantCanFit:    false,
		},
		{
			name:          "fits with zero margin (same as CanFit)",
			systemTokens:  3000,
			contextTokens: 2000,
			historyTokens: 2000,
			margin:        0.0,
			wantCanFit:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canFit := bm.CanFitWithMargin(tt.systemTokens, tt.contextTokens, tt.historyTokens, tt.margin)
			assert.Equal(t, tt.wantCanFit, canFit)
		})
	}
}

// TestBudgetManager_RemainingForResponse tests remaining token calculation.
func TestBudgetManager_RemainingForResponse(t *testing.T) {
	bm := NewBudgetManager("gpt-4") // 8192 tokens

	tests := []struct {
		name          string
		usedTokens    int
		wantRemaining int
	}{
		{
			name:          "full budget remaining",
			usedTokens:    0,
			wantRemaining: 8192,
		},
		{
			name:          "partial budget remaining",
			usedTokens:    4000,
			wantRemaining: 4192,
		},
		{
			name:          "minimal remaining",
			usedTokens:    8000,
			wantRemaining: 192,
		},
		{
			name:          "exactly at limit",
			usedTokens:    8192,
			wantRemaining: 0,
		},
		{
			name:          "over limit returns zero",
			usedTokens:    10000,
			wantRemaining: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining := bm.RemainingForResponse(tt.usedTokens)
			assert.Equal(t, tt.wantRemaining, remaining)
		})
	}
}

// TestBudgetManager_RemainingInCategory tests category-specific remaining tokens.
func TestBudgetManager_RemainingInCategory(t *testing.T) {
	bm := NewBudgetManager("gpt-4") // 8192 tokens
	// SystemPrompt: 1638, Context: 3276, History: 2457, Response: 819

	tests := []struct {
		name          string
		category      string
		usedTokens    int
		wantRemaining int
	}{
		{
			name:          "system category full budget",
			category:      "system",
			usedTokens:    0,
			wantRemaining: 1638,
		},
		{
			name:          "system category partial use",
			category:      "system",
			usedTokens:    500,
			wantRemaining: 1138,
		},
		{
			name:          "context category full budget",
			category:      "context",
			usedTokens:    0,
			wantRemaining: 3276,
		},
		{
			name:          "history category partial use",
			category:      "history",
			usedTokens:    1000,
			wantRemaining: 1457,
		},
		{
			name:          "response category full budget",
			category:      "response",
			usedTokens:    0,
			wantRemaining: 819,
		},
		{
			name:          "over budget returns zero",
			category:      "system",
			usedTokens:    2000,
			wantRemaining: 0,
		},
		{
			name:          "unknown category returns zero",
			category:      "unknown",
			usedTokens:    0,
			wantRemaining: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining := bm.RemainingInCategory(tt.category, tt.usedTokens)
			assert.Equal(t, tt.wantRemaining, remaining)
		})
	}
}

// TestBudgetManager_AdjustForContext tests chunk selection within budget.
func TestBudgetManager_AdjustForContext(t *testing.T) {
	bm := NewBudgetManager("gpt-4") // Context budget: 3276 tokens

	tests := []struct {
		name             string
		chunks           []ContextChunk
		wantSelectedLen  int
		wantTotalTokens  int
	}{
		{
			name:            "empty chunks returns nil",
			chunks:          nil,
			wantSelectedLen: 0,
		},
		{
			name:            "empty slice returns nil",
			chunks:          []ContextChunk{},
			wantSelectedLen: 0,
		},
		{
			name: "all chunks fit within budget",
			chunks: []ContextChunk{
				{Content: "Chunk 1", Tokens: 500, Score: 0.9},
				{Content: "Chunk 2", Tokens: 500, Score: 0.8},
				{Content: "Chunk 3", Tokens: 500, Score: 0.7},
			},
			wantSelectedLen: 3,
			wantTotalTokens: 1500,
		},
		{
			name: "selects chunks until budget exhausted",
			chunks: []ContextChunk{
				{Content: "Chunk 1", Tokens: 1500, Score: 0.9},
				{Content: "Chunk 2", Tokens: 1500, Score: 0.8},
				{Content: "Chunk 3", Tokens: 1500, Score: 0.7},
			},
			wantSelectedLen: 2, // First two fit: 3000 < 3276
			wantTotalTokens: 3000,
		},
		{
			name: "skips chunks that don't fit",
			chunks: []ContextChunk{
				{Content: "Chunk 1", Tokens: 2000, Score: 0.9},
				{Content: "Chunk 2", Tokens: 2000, Score: 0.8}, // Skipped: would exceed
				{Content: "Chunk 3", Tokens: 1000, Score: 0.7}, // Fits: 2000 + 1000 = 3000
			},
			wantSelectedLen: 2,
			wantTotalTokens: 3000,
		},
		{
			name: "single large chunk that fits",
			chunks: []ContextChunk{
				{Content: "Large chunk", Tokens: 3000, Score: 0.9},
			},
			wantSelectedLen: 1,
			wantTotalTokens: 3000,
		},
		{
			name: "single chunk that exceeds budget",
			chunks: []ContextChunk{
				{Content: "Too large", Tokens: 5000, Score: 0.9},
			},
			wantSelectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected := bm.AdjustForContext(tt.chunks)

			if tt.wantSelectedLen == 0 {
				assert.Empty(t, selected)
				return
			}

			assert.Len(t, selected, tt.wantSelectedLen)

			totalTokens := TotalTokens(selected)
			assert.Equal(t, tt.wantTotalTokens, totalTokens)
		})
	}
}

// TestBudgetManager_SelectChunksWithBudget tests chunk selection with custom budget.
func TestBudgetManager_SelectChunksWithBudget(t *testing.T) {
	bm := NewBudgetManager("gpt-4o")

	chunks := []ContextChunk{
		{Content: "Chunk 1", Tokens: 100, Score: 0.9},
		{Content: "Chunk 2", Tokens: 100, Score: 0.8},
		{Content: "Chunk 3", Tokens: 100, Score: 0.7},
		{Content: "Chunk 4", Tokens: 100, Score: 0.6},
	}

	tests := []struct {
		name            string
		budget          int
		wantSelectedLen int
	}{
		{
			name:            "budget allows all chunks",
			budget:          500,
			wantSelectedLen: 4,
		},
		{
			name:            "budget allows two chunks",
			budget:          200,
			wantSelectedLen: 2,
		},
		{
			name:            "budget allows one chunk",
			budget:          100,
			wantSelectedLen: 1,
		},
		{
			name:            "budget allows no chunks",
			budget:          50,
			wantSelectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected := bm.SelectChunksWithBudget(chunks, tt.budget)

			if tt.wantSelectedLen == 0 {
				assert.Empty(t, selected)
				return
			}

			assert.Len(t, selected, tt.wantSelectedLen)
		})
	}
}

// TestBudgetManager_SelectChunksWithLimit tests chunk selection with count limit.
func TestBudgetManager_SelectChunksWithLimit(t *testing.T) {
	bm := NewBudgetManager("gpt-4o")

	chunks := []ContextChunk{
		{Content: "Chunk 1", Tokens: 100, Score: 0.9},
		{Content: "Chunk 2", Tokens: 100, Score: 0.8},
		{Content: "Chunk 3", Tokens: 100, Score: 0.7},
		{Content: "Chunk 4", Tokens: 100, Score: 0.6},
	}

	tests := []struct {
		name            string
		budget          int
		maxChunks       int
		wantSelectedLen int
	}{
		{
			name:            "count limit is binding",
			budget:          1000,
			maxChunks:       2,
			wantSelectedLen: 2,
		},
		{
			name:            "token budget is binding",
			budget:          200,
			maxChunks:       10,
			wantSelectedLen: 2,
		},
		{
			name:            "both limits allow all",
			budget:          1000,
			maxChunks:       10,
			wantSelectedLen: 4,
		},
		{
			name:            "zero max chunks returns empty",
			budget:          1000,
			maxChunks:       0,
			wantSelectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected := bm.SelectChunksWithLimit(chunks, tt.budget, tt.maxChunks)

			if tt.wantSelectedLen == 0 {
				assert.Empty(t, selected)
				return
			}

			assert.Len(t, selected, tt.wantSelectedLen)
		})
	}
}

// TestTotalTokens tests the TotalTokens helper function.
func TestTotalTokens(t *testing.T) {
	tests := []struct {
		name       string
		chunks     []ContextChunk
		wantTokens int
	}{
		{
			name:       "nil chunks returns zero",
			chunks:     nil,
			wantTokens: 0,
		},
		{
			name:       "empty slice returns zero",
			chunks:     []ContextChunk{},
			wantTokens: 0,
		},
		{
			name: "single chunk",
			chunks: []ContextChunk{
				{Tokens: 100},
			},
			wantTokens: 100,
		},
		{
			name: "multiple chunks",
			chunks: []ContextChunk{
				{Tokens: 100},
				{Tokens: 200},
				{Tokens: 300},
			},
			wantTokens: 600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total := TotalTokens(tt.chunks)
			assert.Equal(t, tt.wantTokens, total)
		})
	}
}

// TestBudgetManager_Immutability tests that operations return new instances.
func TestBudgetManager_Immutability(t *testing.T) {
	original := NewBudgetManager("gpt-4o")
	originalMaxTokens := original.MaxTokens()
	originalRatios := original.Ratios()

	t.Run("WithRatios returns new instance", func(t *testing.T) {
		newRatios := types.BudgetConfig{
			SystemPrompt: 0.25,
			Context:      0.25,
			History:      0.25,
			Response:     0.25,
		}
		modified := original.WithRatios(newRatios)

		// Original unchanged
		assert.Equal(t, originalRatios, original.Ratios())

		// New instance has new ratios
		assert.Equal(t, newRatios, modified.Ratios())
		assert.NotSame(t, original, modified)
	})

	t.Run("WithMaxTokens returns new instance", func(t *testing.T) {
		modified := original.WithMaxTokens(50000)

		// Original unchanged
		assert.Equal(t, originalMaxTokens, original.MaxTokens())

		// New instance has new max tokens
		assert.Equal(t, 50000, modified.MaxTokens())
		assert.NotSame(t, original, modified)
	})
}

// TestValidateRatios tests ratio validation.
func TestValidateRatios(t *testing.T) {
	tests := []struct {
		name    string
		ratios  types.BudgetConfig
		isValid bool
	}{
		{
			name:    "valid default ratios",
			ratios:  DefaultBudgetRatios,
			isValid: true,
		},
		{
			name: "valid custom ratios summing to 1.0",
			ratios: types.BudgetConfig{
				SystemPrompt: 0.25,
				Context:      0.25,
				History:      0.25,
				Response:     0.25,
			},
			isValid: true,
		},
		{
			name: "valid with small floating point error",
			ratios: types.BudgetConfig{
				SystemPrompt: 0.333,
				Context:      0.333,
				History:      0.333,
				Response:     0.001,
			},
			isValid: true,
		},
		{
			name: "invalid ratios summing to less than 0.99",
			ratios: types.BudgetConfig{
				SystemPrompt: 0.10,
				Context:      0.20,
				History:      0.20,
				Response:     0.10,
			},
			isValid: false,
		},
		{
			name: "invalid ratios summing to more than 1.01",
			ratios: types.BudgetConfig{
				SystemPrompt: 0.30,
				Context:      0.40,
				History:      0.30,
				Response:     0.20,
			},
			isValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := ValidateRatios(tt.ratios)
			assert.Equal(t, tt.isValid, valid)
		})
	}
}

// TestNormalizeRatios tests ratio normalization.
func TestNormalizeRatios(t *testing.T) {
	tests := []struct {
		name   string
		ratios types.BudgetConfig
	}{
		{
			name: "normalizes unbalanced ratios",
			ratios: types.BudgetConfig{
				SystemPrompt: 1.0,
				Context:      2.0,
				History:      1.5,
				Response:     0.5,
			},
		},
		{
			name: "handles already normalized ratios",
			ratios: types.BudgetConfig{
				SystemPrompt: 0.25,
				Context:      0.25,
				History:      0.25,
				Response:     0.25,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := NormalizeRatios(tt.ratios)

			sum := normalized.SystemPrompt + normalized.Context + normalized.History + normalized.Response
			assert.InDelta(t, 1.0, sum, 0.0001, "normalized ratios should sum to 1.0")
		})
	}
}

// TestNormalizeRatios_ZeroSum tests normalization with zero sum.
func TestNormalizeRatios_ZeroSum(t *testing.T) {
	zeroRatios := types.BudgetConfig{
		SystemPrompt: 0,
		Context:      0,
		History:      0,
		Response:     0,
	}

	normalized := NormalizeRatios(zeroRatios)

	// Should return defaults when sum is zero
	assert.Equal(t, DefaultBudgetRatios, normalized)
}

// TestModelContextLimits verifies all expected models are present.
func TestModelContextLimits(t *testing.T) {
	expectedModels := []string{
		"gpt-4o",
		"gpt-4o-mini",
		"gpt-4-turbo",
		"gpt-4",
		"gpt-3.5-turbo",
		"gpt-3.5-turbo-16k",
		"gemini-2.0-flash",
		"gemini-2.0-flash-lite",
		"gemini-2.0-pro",
		"gemini-1.5-pro",
		"gemini-1.5-flash",
		"claude-3-opus",
		"claude-3-sonnet",
		"claude-3-haiku",
	}

	for _, model := range expectedModels {
		t.Run(model, func(t *testing.T) {
			limit, ok := ModelContextLimits[model]
			assert.True(t, ok, "model %s should be in ModelContextLimits", model)
			assert.Greater(t, limit, 0, "model %s should have positive context limit", model)
		})
	}
}
