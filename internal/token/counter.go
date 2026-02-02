// Package token provides token counting utilities for LLM context management.
package token

import (
	"strings"
	"unicode/utf8"

	"github.com/pkoukk/tiktoken-go"
)

// Counter wraps a tiktoken encoder for token counting operations.
type Counter struct {
	encoder  *tiktoken.Tiktoken
	encoding string
}

// Default encoding for fallback.
const defaultEncoding = "cl100k_base"

// Message overhead constants for chat message token counting.
// These are based on OpenAI's chat format overhead.
const (
	// Tokens added per message for role and formatting.
	messageOverhead = 4
	// Tokens added for the assistant reply priming.
	replyPriming = 2
)

// NewCounter creates a new token counter with the specified encoding.
// Supported encodings include:
//   - "cl100k_base" (GPT-4, GPT-4-turbo, GPT-3.5-turbo)
//   - "p50k_base" (GPT-3, Codex)
//   - "o200k_base" (GPT-4o)
//   - "r50k_base" (older models)
//
// Falls back to cl100k_base if the specified encoding is not found.
func NewCounter(encoding string) (*Counter, error) {
	if encoding == "" {
		encoding = defaultEncoding
	}

	encoder, err := tiktoken.GetEncoding(encoding)
	if err != nil {
		// Fallback to default encoding
		encoder, err = tiktoken.GetEncoding(defaultEncoding)
		if err != nil {
			return nil, err
		}
		encoding = defaultEncoding
	}

	return &Counter{
		encoder:  encoder,
		encoding: encoding,
	}, nil
}

// Encoding returns the current encoding name.
func (c *Counter) Encoding() string {
	return c.encoding
}

// Count returns the number of tokens in the given text.
func (c *Counter) Count(text string) int {
	if text == "" {
		return 0
	}
	tokens := c.encoder.Encode(text, nil, nil)
	return len(tokens)
}

// CountMessages counts the total tokens in a slice of chat messages,
// including per-message overhead for role and formatting.
// This follows OpenAI's token counting convention for chat messages.
func (c *Counter) CountMessages(messages []ChatMessage) int {
	if len(messages) == 0 {
		return 0
	}

	total := 0
	for _, msg := range messages {
		// Each message has overhead for role and formatting
		total += messageOverhead
		total += c.Count(msg.Content)

		// Add tokens for role name if present
		if msg.Name != "" {
			total += c.Count(msg.Name) + 1
		}
	}

	// Add priming tokens for assistant reply
	total += replyPriming

	return total
}

// Truncate truncates the given text to fit within the specified token limit.
// Returns the truncated text. If the text is already within the limit,
// it is returned unchanged.
func (c *Counter) Truncate(text string, maxTokens int) string {
	if maxTokens <= 0 {
		return ""
	}

	tokens := c.encoder.Encode(text, nil, nil)
	if len(tokens) <= maxTokens {
		return text
	}

	// Decode only the tokens that fit
	truncatedTokens := tokens[:maxTokens]
	return c.encoder.Decode(truncatedTokens)
}

// Split divides the text into overlapping chunks of approximately chunkSize tokens.
// The overlap parameter specifies the fraction of overlap between consecutive chunks
// (0.0 = no overlap, 0.5 = 50% overlap).
// Returns a slice of text chunks.
func (c *Counter) Split(text string, chunkSize int, overlap float64) []string {
	if text == "" || chunkSize <= 0 {
		return nil
	}

	// Clamp overlap to valid range
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= 1 {
		overlap = 0.9
	}

	tokens := c.encoder.Encode(text, nil, nil)
	if len(tokens) <= chunkSize {
		return []string{text}
	}

	overlapSize := int(float64(chunkSize) * overlap)
	step := chunkSize - overlapSize
	if step <= 0 {
		step = 1
	}

	var chunks []string
	for i := 0; i < len(tokens); i += step {
		end := i + chunkSize
		if end > len(tokens) {
			end = len(tokens)
		}

		chunk := c.encoder.Decode(tokens[i:end])
		chunks = append(chunks, chunk)

		// Stop if we've reached the end
		if end >= len(tokens) {
			break
		}
	}

	return chunks
}

// TruncateToFit truncates text from the beginning or end to fit within maxTokens.
// If fromEnd is true, keeps the end of the text; otherwise keeps the beginning.
func (c *Counter) TruncateToFit(text string, maxTokens int, fromEnd bool) string {
	if maxTokens <= 0 {
		return ""
	}

	tokens := c.encoder.Encode(text, nil, nil)
	if len(tokens) <= maxTokens {
		return text
	}

	if fromEnd {
		// Keep the last maxTokens
		startIdx := len(tokens) - maxTokens
		return c.encoder.Decode(tokens[startIdx:])
	}

	// Keep the first maxTokens
	return c.encoder.Decode(tokens[:maxTokens])
}

// EstimateTokens provides a quick estimate of token count without encoding.
// This is less accurate but faster, useful for rough estimates.
// Uses a heuristic of approximately 4 characters per token for English text.
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	// Average of 4 characters per token for English
	// This is a rough estimate and actual count may vary
	runeCount := utf8.RuneCountInString(text)
	return (runeCount + 3) / 4
}

// SplitByWords splits text into chunks trying to respect word boundaries.
// Each chunk will be at most maxTokens. Returns chunks with approximate token counts.
func (c *Counter) SplitByWords(text string, maxTokens int) []ChunkWithCount {
	if text == "" || maxTokens <= 0 {
		return nil
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var chunks []ChunkWithCount
	var currentChunk strings.Builder
	currentTokens := 0

	for _, word := range words {
		wordTokens := c.Count(word)
		spaceTokens := 0
		if currentChunk.Len() > 0 {
			spaceTokens = 1 // Approximate token for space
		}

		if currentTokens+wordTokens+spaceTokens > maxTokens && currentChunk.Len() > 0 {
			// Save current chunk and start a new one
			chunks = append(chunks, ChunkWithCount{
				Text:   strings.TrimSpace(currentChunk.String()),
				Tokens: currentTokens,
			})
			currentChunk.Reset()
			currentTokens = 0
		}

		if currentChunk.Len() > 0 {
			currentChunk.WriteString(" ")
		}
		currentChunk.WriteString(word)
		currentTokens = c.Count(currentChunk.String())
	}

	// Add the last chunk if not empty
	if currentChunk.Len() > 0 {
		chunks = append(chunks, ChunkWithCount{
			Text:   strings.TrimSpace(currentChunk.String()),
			Tokens: currentTokens,
		})
	}

	return chunks
}

// ChunkWithCount represents a text chunk with its token count.
type ChunkWithCount struct {
	Text   string
	Tokens int
}

// ChatMessage mirrors the llm.ChatMessage for use in token counting.
// This avoids circular imports while maintaining the same structure.
type ChatMessage struct {
	Role       string
	Content    string
	Name       string
	ToolCallID string
}
