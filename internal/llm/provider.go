// Package llm provides abstractions for interacting with Large Language Models.
package llm

import (
	"context"
	"errors"
)

// Common errors returned by LLM providers.
var (
	// ErrContextTooLong is returned when the input exceeds the model's context window.
	ErrContextTooLong = errors.New("context length exceeds model maximum")

	// ErrRateLimited is returned when the API rate limit has been exceeded.
	ErrRateLimited = errors.New("rate limit exceeded")

	// ErrAPIError is returned when the API returns an unexpected error.
	ErrAPIError = errors.New("API error")

	// ErrInvalidAPIKey is returned when the API key is invalid or missing.
	ErrInvalidAPIKey = errors.New("invalid or missing API key")

	// ErrModelNotFound is returned when the requested model is not available.
	ErrModelNotFound = errors.New("model not found")

	// ErrStreamingNotSupported is returned when streaming is requested but not supported.
	ErrStreamingNotSupported = errors.New("streaming not supported by this provider")

	// ErrToolsNotSupported is returned when tools are requested but not supported.
	ErrToolsNotSupported = errors.New("tools not supported by this provider")
)

// Role constants for chat messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// FinishReason constants for response completion reasons.
const (
	FinishReasonStop      = "stop"
	FinishReasonLength    = "length"
	FinishReasonToolCalls = "tool_calls"
	FinishReasonError     = "error"
)

// Provider defines the interface for LLM providers.
// Implementations should be safe for concurrent use.
type Provider interface {
	// Chat sends a chat request and returns the complete response.
	// Returns ErrContextTooLong if the request exceeds context limits.
	// Returns ErrRateLimited if rate limits are exceeded.
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// Stream sends a chat request and returns a channel of streaming chunks.
	// The channel is closed when the response is complete or an error occurs.
	// Check StreamChunk.Error for any errors during streaming.
	// Returns ErrStreamingNotSupported if the provider doesn't support streaming.
	Stream(ctx context.Context, req ChatRequest) (<-chan StreamChunk, error)

	// Capabilities returns the capabilities of this provider.
	Capabilities() Capabilities

	// Close releases any resources held by the provider.
	Close() error
}

// ChatRequest represents a request to the chat API.
type ChatRequest struct {
	// Messages is the conversation history to send.
	Messages []ChatMessage

	// MaxTokens is the maximum number of tokens to generate.
	// If 0, the provider's default is used.
	MaxTokens int

	// Temperature controls randomness in the response (0.0-2.0).
	// Lower values produce more deterministic output.
	Temperature float64

	// Tools defines the available tools/functions the model can call.
	// Optional; only used if the provider supports tool calling.
	Tools []ToolDefinition

	// ToolChoice controls how the model uses tools.
	// Values: "auto", "none", "required", or a specific tool name.
	ToolChoice string

	// Stop sequences that will stop generation.
	Stop []string
}

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	// Role indicates the message author: system, user, assistant, or tool.
	Role string

	// Content is the text content of the message.
	Content string

	// ToolCalls contains tool invocations made by the assistant.
	// Only present when Role is "assistant" and the model requested tool calls.
	ToolCalls []ToolCall

	// ToolCallID is the ID of the tool call this message is responding to.
	// Only present when Role is "tool".
	ToolCallID string

	// Name is an optional name for the message author.
	// Used primarily for tool responses to identify the tool.
	Name string
}

// ChatResponse represents the complete response from a chat request.
type ChatResponse struct {
	// Message is the assistant's response message.
	Message ChatMessage

	// Usage contains token usage statistics.
	Usage TokenUsage

	// FinishReason indicates why generation stopped.
	// Values: "stop", "length", "tool_calls", "error".
	FinishReason string

	// Model is the actual model used (may differ from requested if aliased).
	Model string
}

// StreamChunk represents a single chunk in a streaming response.
type StreamChunk struct {
	// Delta is the incremental text content.
	Delta string

	// ToolCall contains an incremental tool call update.
	// Only present if the model is generating a tool call.
	ToolCall *ToolCallDelta

	// Done indicates this is the final chunk.
	Done bool

	// FinishReason is set on the final chunk to indicate why generation stopped.
	FinishReason string

	// Usage is optionally included in the final chunk.
	Usage *TokenUsage

	// Error contains any error that occurred during streaming.
	Error error
}

// ToolDefinition describes a tool that the model can call.
type ToolDefinition struct {
	// Type is the tool type (currently always "function").
	Type string

	// Function contains the function definition.
	Function FunctionDefinition
}

// FunctionDefinition describes a callable function.
type FunctionDefinition struct {
	// Name is the function name.
	Name string

	// Description explains what the function does.
	Description string

	// Parameters is a JSON Schema object describing the function parameters.
	Parameters map[string]interface{}

	// Strict enables strict mode for parameter validation (OpenAI-specific).
	Strict bool
}

// ToolCall represents a tool invocation by the assistant.
type ToolCall struct {
	// ID is a unique identifier for this tool call.
	ID string

	// Type is the tool type (currently always "function").
	Type string

	// Function contains the function call details.
	Function FunctionCall
}

// FunctionCall contains the details of a function invocation.
type FunctionCall struct {
	// Name is the function to call.
	Name string

	// Arguments is a JSON string of the function arguments.
	Arguments string
}

// ToolCallDelta represents an incremental update to a tool call during streaming.
type ToolCallDelta struct {
	// Index identifies which tool call is being updated (for parallel calls).
	Index int

	// ID is set on the first chunk for this tool call.
	ID string

	// Type is set on the first chunk for this tool call.
	Type string

	// Function contains incremental function call data.
	Function *FunctionCallDelta
}

// FunctionCallDelta represents incremental function call data.
type FunctionCallDelta struct {
	// Name is set on the first chunk for this function call.
	Name string

	// Arguments is the incremental JSON string of arguments.
	Arguments string
}

// TokenUsage contains token usage statistics for a request.
type TokenUsage struct {
	// PromptTokens is the number of tokens in the prompt.
	PromptTokens int

	// CompletionTokens is the number of tokens in the completion.
	CompletionTokens int

	// TotalTokens is the total number of tokens used.
	TotalTokens int

	// CachedTokens is the number of tokens served from cache (if applicable).
	CachedTokens int
}

// Capabilities describes what a provider supports.
type Capabilities struct {
	// SupportsTools indicates if the provider supports tool/function calling.
	SupportsTools bool

	// SupportsStreaming indicates if the provider supports streaming responses.
	SupportsStreaming bool

	// SupportsVision indicates if the provider supports image inputs.
	SupportsVision bool

	// MaxContextTokens is the maximum context window size.
	MaxContextTokens int

	// MaxOutputTokens is the maximum number of tokens the model can generate.
	MaxOutputTokens int

	// TokenizerType identifies the tokenizer to use for token counting.
	// Values: "cl100k_base" (GPT-4), "o200k_base" (GPT-4o), "claude", etc.
	TokenizerType string

	// Models lists the available model names.
	Models []string
}

// NewChatMessage creates a new ChatMessage with the specified role and content.
func NewChatMessage(role, content string) ChatMessage {
	return ChatMessage{
		Role:    role,
		Content: content,
	}
}

// NewSystemMessage creates a new system message.
func NewSystemMessage(content string) ChatMessage {
	return ChatMessage{
		Role:    RoleSystem,
		Content: content,
	}
}

// NewUserMessage creates a new user message.
func NewUserMessage(content string) ChatMessage {
	return ChatMessage{
		Role:    RoleUser,
		Content: content,
	}
}

// NewAssistantMessage creates a new assistant message.
func NewAssistantMessage(content string) ChatMessage {
	return ChatMessage{
		Role:    RoleAssistant,
		Content: content,
	}
}

// NewToolMessage creates a new tool response message.
func NewToolMessage(toolCallID, name, content string) ChatMessage {
	return ChatMessage{
		Role:       RoleTool,
		Content:    content,
		ToolCallID: toolCallID,
		Name:       name,
	}
}

// NewToolDefinition creates a new function tool definition.
func NewToolDefinition(name, description string, parameters map[string]interface{}) ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: FunctionDefinition{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}
}

// IsToolCallResponse returns true if this message is a response to a tool call.
func (m ChatMessage) IsToolCallResponse() bool {
	return m.Role == RoleTool && m.ToolCallID != ""
}

// HasToolCalls returns true if this message contains tool calls.
func (m ChatMessage) HasToolCalls() bool {
	return len(m.ToolCalls) > 0
}

// IsComplete returns true if this chunk indicates the stream is complete.
func (c StreamChunk) IsComplete() bool {
	return c.Done || c.Error != nil
}
