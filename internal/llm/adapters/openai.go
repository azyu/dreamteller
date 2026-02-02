// Package adapters provides LLM provider implementations.
package adapters

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/azyu/dreamteller/internal/llm"
	openai "github.com/sashabaranov/go-openai"
)

// modelCapabilities maps model names to their capabilities.
var modelCapabilities = map[string]llm.Capabilities{
	"gpt-4o": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    true,
		MaxContextTokens:  128000,
		MaxOutputTokens:   16384,
		TokenizerType:     "o200k_base",
	},
	"gpt-4o-mini": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    true,
		MaxContextTokens:  128000,
		MaxOutputTokens:   16384,
		TokenizerType:     "o200k_base",
	},
	"gpt-4-turbo": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    true,
		MaxContextTokens:  128000,
		MaxOutputTokens:   4096,
		TokenizerType:     "cl100k_base",
	},
	"gpt-4-turbo-preview": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    false,
		MaxContextTokens:  128000,
		MaxOutputTokens:   4096,
		TokenizerType:     "cl100k_base",
	},
	"gpt-4": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    false,
		MaxContextTokens:  8192,
		MaxOutputTokens:   4096,
		TokenizerType:     "cl100k_base",
	},
	"gpt-4-32k": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    false,
		MaxContextTokens:  32768,
		MaxOutputTokens:   4096,
		TokenizerType:     "cl100k_base",
	},
	"gpt-3.5-turbo": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    false,
		MaxContextTokens:  16385,
		MaxOutputTokens:   4096,
		TokenizerType:     "cl100k_base",
	},
	"gpt-3.5-turbo-16k": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    false,
		MaxContextTokens:  16385,
		MaxOutputTokens:   4096,
		TokenizerType:     "cl100k_base",
	},
	"o1": {
		SupportsTools:     false,
		SupportsStreaming: false,
		SupportsVision:    true,
		MaxContextTokens:  200000,
		MaxOutputTokens:   100000,
		TokenizerType:     "o200k_base",
	},
	"o1-mini": {
		SupportsTools:     false,
		SupportsStreaming: false,
		SupportsVision:    false,
		MaxContextTokens:  128000,
		MaxOutputTokens:   65536,
		TokenizerType:     "o200k_base",
	},
	"o1-preview": {
		SupportsTools:     false,
		SupportsStreaming: false,
		SupportsVision:    false,
		MaxContextTokens:  128000,
		MaxOutputTokens:   32768,
		TokenizerType:     "o200k_base",
	},
}

// defaultCapabilities is used for unknown models.
var defaultCapabilities = llm.Capabilities{
	SupportsTools:     true,
	SupportsStreaming: true,
	SupportsVision:    false,
	MaxContextTokens:  128000,
	MaxOutputTokens:   4096,
	TokenizerType:     "cl100k_base",
}

// OpenAIAdapter implements the Provider interface for OpenAI API.
type OpenAIAdapter struct {
	client *openai.Client
	model  string
	config OpenAIConfig
}

// OpenAIConfig holds configuration for the OpenAI adapter.
type OpenAIConfig struct {
	// APIKey is the OpenAI API key.
	APIKey string

	// Model is the model to use for completions.
	Model string

	// BaseURL overrides the default API URL (for Azure or compatible APIs).
	BaseURL string

	// Organization is the optional OpenAI organization ID.
	Organization string

	// Timeout is the request timeout duration.
	Timeout time.Duration

	// MaxRetries is the number of retries for rate-limited requests.
	MaxRetries int

	// RetryDelay is the initial delay between retries.
	RetryDelay time.Duration
}

// OpenAIOption configures an OpenAIAdapter.
type OpenAIOption func(*OpenAIConfig)

// WithOpenAIBaseURL sets a custom base URL.
func WithOpenAIBaseURL(baseURL string) OpenAIOption {
	return func(c *OpenAIConfig) {
		c.BaseURL = baseURL
	}
}

// WithOpenAIOrganization sets the organization ID.
func WithOpenAIOrganization(org string) OpenAIOption {
	return func(c *OpenAIConfig) {
		c.Organization = org
	}
}

// WithOpenAITimeout sets the request timeout.
func WithOpenAITimeout(timeout time.Duration) OpenAIOption {
	return func(c *OpenAIConfig) {
		c.Timeout = timeout
	}
}

// WithOpenAIRetry sets retry configuration.
func WithOpenAIRetry(maxRetries int, retryDelay time.Duration) OpenAIOption {
	return func(c *OpenAIConfig) {
		c.MaxRetries = maxRetries
		c.RetryDelay = retryDelay
	}
}

// NewOpenAIAdapter creates a new OpenAI adapter.
func NewOpenAIAdapter(apiKey, model string, opts ...OpenAIOption) (*OpenAIAdapter, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("%w: API key is required", llm.ErrInvalidAPIKey)
	}

	if model == "" {
		model = "gpt-4o"
	}

	config := OpenAIConfig{
		APIKey:     apiKey,
		Model:      model,
		Timeout:    120 * time.Second,
		MaxRetries: 3,
		RetryDelay: 1 * time.Second,
	}

	for _, opt := range opts {
		opt(&config)
	}

	clientConfig := openai.DefaultConfig(apiKey)

	if config.BaseURL != "" {
		clientConfig.BaseURL = config.BaseURL
	}

	if config.Organization != "" {
		clientConfig.OrgID = config.Organization
	}

	client := openai.NewClientWithConfig(clientConfig)

	return &OpenAIAdapter{
		client: client,
		model:  model,
		config: config,
	}, nil
}

// Chat sends a chat completion request and returns the complete response.
func (a *OpenAIAdapter) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	openAIReq := a.buildRequest(req)

	var lastErr error
	for attempt := 0; attempt <= a.config.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(a.config.RetryDelay * time.Duration(attempt)):
			}
		}

		resp, err := a.client.CreateChatCompletion(ctx, openAIReq)
		if err != nil {
			lastErr = a.handleError(err)
			if !a.isRetryable(lastErr) {
				return nil, lastErr
			}
			continue
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("%w: no choices in response", llm.ErrAPIError)
		}

		return a.buildResponse(resp), nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// Stream sends a chat completion request and streams the response.
func (a *OpenAIAdapter) Stream(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	caps := a.Capabilities()
	if !caps.SupportsStreaming {
		return nil, llm.ErrStreamingNotSupported
	}

	openAIReq := a.buildRequest(req)
	openAIReq.Stream = true

	stream, err := a.client.CreateChatCompletionStream(ctx, openAIReq)
	if err != nil {
		return nil, a.handleError(err)
	}

	chunks := make(chan llm.StreamChunk, 100)

	go a.processStream(ctx, stream, chunks)

	return chunks, nil
}

// processStream reads from the OpenAI stream and sends chunks to the channel.
func (a *OpenAIAdapter) processStream(ctx context.Context, stream *openai.ChatCompletionStream, chunks chan<- llm.StreamChunk) {
	defer close(chunks)
	defer stream.Close()

	// Track tool calls being built across chunks
	toolCalls := make(map[int]*llm.ToolCallDelta)

	for {
		select {
		case <-ctx.Done():
			chunks <- llm.StreamChunk{
				Error: ctx.Err(),
				Done:  true,
			}
			return
		default:
		}

		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			chunks <- llm.StreamChunk{Done: true}
			return
		}

		if err != nil {
			chunks <- llm.StreamChunk{
				Error: a.handleError(err),
				Done:  true,
			}
			return
		}

		if len(resp.Choices) == 0 {
			continue
		}

		choice := resp.Choices[0]
		chunk := llm.StreamChunk{
			Delta:        choice.Delta.Content,
			FinishReason: string(choice.FinishReason),
			Done:         choice.FinishReason != "",
		}

		// Handle tool call deltas
		if len(choice.Delta.ToolCalls) > 0 {
			tc := choice.Delta.ToolCalls[0]

			// Index is a pointer, default to 0 if nil
			index := 0
			if tc.Index != nil {
				index = *tc.Index
			}

			if _, exists := toolCalls[index]; !exists {
				toolCalls[index] = &llm.ToolCallDelta{
					Index: index,
				}
			}

			delta := toolCalls[index]

			if tc.ID != "" {
				delta.ID = tc.ID
			}
			if tc.Type != "" {
				delta.Type = string(tc.Type)
			}

			if tc.Function.Name != "" || tc.Function.Arguments != "" {
				if delta.Function == nil {
					delta.Function = &llm.FunctionCallDelta{}
				}
				if tc.Function.Name != "" {
					delta.Function.Name = tc.Function.Name
				}
				if tc.Function.Arguments != "" {
					delta.Function.Arguments = tc.Function.Arguments
				}
			}

			chunk.ToolCall = &llm.ToolCallDelta{
				Index: index,
				ID:    tc.ID,
				Type:  string(tc.Type),
			}
			if tc.Function.Name != "" || tc.Function.Arguments != "" {
				chunk.ToolCall.Function = &llm.FunctionCallDelta{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				}
			}
		}

		// Include usage if available (final chunk)
		if resp.Usage != nil {
			chunk.Usage = &llm.TokenUsage{
				PromptTokens:     resp.Usage.PromptTokens,
				CompletionTokens: resp.Usage.CompletionTokens,
				TotalTokens:      resp.Usage.TotalTokens,
			}
		}

		chunks <- chunk
	}
}

// Capabilities returns the provider's capabilities.
func (a *OpenAIAdapter) Capabilities() llm.Capabilities {
	if caps, ok := modelCapabilities[a.model]; ok {
		caps.Models = a.availableModels()
		return caps
	}
	caps := defaultCapabilities
	caps.Models = a.availableModels()
	return caps
}

// Close releases resources held by the adapter.
func (a *OpenAIAdapter) Close() error {
	// No persistent resources to clean up
	return nil
}

// Model returns the current model name.
func (a *OpenAIAdapter) Model() string {
	return a.model
}

// buildRequest converts our ChatRequest to the OpenAI format.
func (a *OpenAIAdapter) buildRequest(req llm.ChatRequest) openai.ChatCompletionRequest {
	messages := make([]openai.ChatCompletionMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = a.convertMessage(msg)
	}

	openAIReq := openai.ChatCompletionRequest{
		Model:    a.model,
		Messages: messages,
		Stop:     req.Stop,
	}

	if req.MaxTokens > 0 {
		openAIReq.MaxTokens = req.MaxTokens
	}

	if req.Temperature > 0 {
		openAIReq.Temperature = float32(req.Temperature)
	}

	// Add tools if provided and supported
	if len(req.Tools) > 0 {
		caps := a.Capabilities()
		if caps.SupportsTools {
			openAIReq.Tools = a.convertTools(req.Tools)

			if req.ToolChoice != "" {
				switch req.ToolChoice {
				case "auto":
					openAIReq.ToolChoice = "auto"
				case "none":
					openAIReq.ToolChoice = "none"
				case "required":
					openAIReq.ToolChoice = "required"
				default:
					// Specific tool name
					openAIReq.ToolChoice = openai.ToolChoice{
						Type: openai.ToolTypeFunction,
						Function: openai.ToolFunction{
							Name: req.ToolChoice,
						},
					}
				}
			}
		}
	}

	return openAIReq
}

// convertMessage converts our ChatMessage to OpenAI format.
func (a *OpenAIAdapter) convertMessage(msg llm.ChatMessage) openai.ChatCompletionMessage {
	openAIMsg := openai.ChatCompletionMessage{
		Role:    msg.Role,
		Content: msg.Content,
	}

	if msg.Name != "" {
		openAIMsg.Name = msg.Name
	}

	if msg.ToolCallID != "" {
		openAIMsg.ToolCallID = msg.ToolCallID
	}

	// Convert tool calls
	if len(msg.ToolCalls) > 0 {
		openAIMsg.ToolCalls = make([]openai.ToolCall, len(msg.ToolCalls))
		for i, tc := range msg.ToolCalls {
			openAIMsg.ToolCalls[i] = openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolType(tc.Type),
				Function: openai.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	return openAIMsg
}

// convertTools converts our ToolDefinition slice to OpenAI format.
func (a *OpenAIAdapter) convertTools(tools []llm.ToolDefinition) []openai.Tool {
	openAITools := make([]openai.Tool, len(tools))
	for i, tool := range tools {
		openAITools[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
				Strict:      tool.Function.Strict,
			},
		}
	}
	return openAITools
}

// buildResponse converts OpenAI response to our ChatResponse.
func (a *OpenAIAdapter) buildResponse(resp openai.ChatCompletionResponse) *llm.ChatResponse {
	choice := resp.Choices[0]

	message := llm.ChatMessage{
		Role:    choice.Message.Role,
		Content: choice.Message.Content,
	}

	// Convert tool calls if present
	if len(choice.Message.ToolCalls) > 0 {
		message.ToolCalls = make([]llm.ToolCall, len(choice.Message.ToolCalls))
		for i, tc := range choice.Message.ToolCalls {
			message.ToolCalls[i] = llm.ToolCall{
				ID:   tc.ID,
				Type: string(tc.Type),
				Function: llm.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			}
		}
	}

	return &llm.ChatResponse{
		Message: message,
		Usage: llm.TokenUsage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
		FinishReason: string(choice.FinishReason),
		Model:        resp.Model,
	}
}

// handleError converts OpenAI errors to our error types.
func (a *OpenAIAdapter) handleError(err error) error {
	if err == nil {
		return nil
	}

	// Check for context errors
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("request canceled: %w", err)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("request timed out: %w", err)
	}

	// Check for OpenAI API errors
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case 401:
			return fmt.Errorf("%w: %s", llm.ErrInvalidAPIKey, apiErr.Message)
		case 404:
			return fmt.Errorf("%w: %s", llm.ErrModelNotFound, apiErr.Message)
		case 429:
			return fmt.Errorf("%w: %s", llm.ErrRateLimited, apiErr.Message)
		case 400:
			// Check for context length errors
			if apiErr.Code == "context_length_exceeded" {
				return fmt.Errorf("%w: %s", llm.ErrContextTooLong, apiErr.Message)
			}
			return fmt.Errorf("%w: %s", llm.ErrAPIError, apiErr.Message)
		case 500, 502, 503, 504:
			return fmt.Errorf("%w: server error - %s", llm.ErrAPIError, apiErr.Message)
		default:
			return fmt.Errorf("%w: HTTP %d - %s", llm.ErrAPIError, apiErr.HTTPStatusCode, apiErr.Message)
		}
	}

	// Check for request errors
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		return fmt.Errorf("%w: %s", llm.ErrAPIError, reqErr.Error())
	}

	return fmt.Errorf("%w: %s", llm.ErrAPIError, err.Error())
}

// isRetryable returns true if the error is retryable.
func (a *OpenAIAdapter) isRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Rate limit errors are retryable
	if errors.Is(err, llm.ErrRateLimited) {
		return true
	}

	// Check for OpenAI API errors
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case 429, 500, 502, 503, 504:
			return true
		}
	}

	return false
}

// availableModels returns the list of available OpenAI models.
func (a *OpenAIAdapter) availableModels() []string {
	return []string{
		"gpt-4o",
		"gpt-4o-mini",
		"gpt-4-turbo",
		"gpt-4-turbo-preview",
		"gpt-4",
		"gpt-4-32k",
		"gpt-3.5-turbo",
		"gpt-3.5-turbo-16k",
		"o1",
		"o1-mini",
		"o1-preview",
	}
}

// Verify OpenAIAdapter implements Provider interface.
var _ llm.Provider = (*OpenAIAdapter)(nil)
