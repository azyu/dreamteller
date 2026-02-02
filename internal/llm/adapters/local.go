// Package adapters provides LLM provider implementations.
package adapters

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/azyu/dreamteller/internal/llm"
)

const (
	defaultTimeout     = 120 * time.Second
	defaultMaxTokens   = 2048
	defaultTemperature = 0.7
)

// LocalAdapter implements the Provider interface for local OpenAI-compatible APIs.
// It works with servers like Ollama, LM Studio, vLLM, and other compatible implementations.
type LocalAdapter struct {
	client  *http.Client
	baseURL string
	model   string
	timeout time.Duration
}

// LocalAdapterOption configures a LocalAdapter.
type LocalAdapterOption func(*LocalAdapter)

// WithTimeout sets a custom timeout for requests.
func WithTimeout(timeout time.Duration) LocalAdapterOption {
	return func(a *LocalAdapter) {
		a.timeout = timeout
		a.client.Timeout = timeout
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) LocalAdapterOption {
	return func(a *LocalAdapter) {
		a.client = client
	}
}

// NewLocalAdapter creates a new LocalAdapter for OpenAI-compatible local servers.
// The baseURL should point to the server (e.g., "http://localhost:11434" for Ollama).
// The model should be the model name to use (e.g., "llama3.2", "mistral").
func NewLocalAdapter(baseURL, model string, opts ...LocalAdapterOption) *LocalAdapter {
	// Normalize base URL - remove trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	adapter := &LocalAdapter{
		client: &http.Client{
			Timeout: defaultTimeout,
		},
		baseURL: baseURL,
		model:   model,
		timeout: defaultTimeout,
	}

	for _, opt := range opts {
		opt(adapter)
	}

	return adapter
}

// openAIChatRequest represents the OpenAI-compatible chat completion request.
type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	MaxTokens   int                 `json:"max_tokens,omitempty"`
	Temperature float64             `json:"temperature,omitempty"`
	Stream      bool                `json:"stream"`
	Stop        []string            `json:"stop,omitempty"`
}

// openAIChatMessage represents a message in the OpenAI format.
type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatResponse represents the OpenAI-compatible chat completion response.
type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int               `json:"index"`
		Message      openAIChatMessage `json:"message"`
		FinishReason string            `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// openAIStreamChunk represents a single chunk in the streaming response.
type openAIStreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage,omitempty"`
}

// openAIErrorResponse represents an error response from the API.
type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Chat sends a chat completion request and returns the complete response.
func (a *LocalAdapter) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	openAIReq := a.buildRequest(req, false)

	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("request timed out: %w", err)
		}
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("request canceled: %w", err)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, a.handleErrorResponse(resp)
	}

	var openAIResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if len(openAIResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := openAIResp.Choices[0]
	return &llm.ChatResponse{
		Message: llm.ChatMessage{
			Role:    choice.Message.Role,
			Content: choice.Message.Content,
		},
		Usage: llm.TokenUsage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		},
		FinishReason: choice.FinishReason,
		Model:        openAIResp.Model,
	}, nil
}

// Stream sends a chat completion request and returns a channel of streaming chunks.
func (a *LocalAdapter) Stream(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	openAIReq := a.buildRequest(req, true)

	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")
	httpReq.Header.Set("Connection", "keep-alive")

	// Use a client without timeout for streaming - context handles cancellation
	streamClient := &http.Client{}
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("request timed out: %w", err)
		}
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("request canceled: %w", err)
		}
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, a.handleErrorResponse(resp)
	}

	chunks := make(chan llm.StreamChunk, 100)

	go a.processStream(ctx, resp.Body, chunks)

	return chunks, nil
}

// processStream reads the SSE stream and sends chunks to the channel.
func (a *LocalAdapter) processStream(ctx context.Context, body io.ReadCloser, chunks chan<- llm.StreamChunk) {
	defer close(chunks)
	defer body.Close()

	reader := bufio.NewReader(body)

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

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				chunks <- llm.StreamChunk{Done: true}
				return
			}
			chunks <- llm.StreamChunk{
				Error: fmt.Errorf("failed to read stream: %w", err),
				Done:  true,
			}
			return
		}

		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Parse SSE data line
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for stream end
		if data == "[DONE]" {
			chunks <- llm.StreamChunk{Done: true}
			return
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Some implementations send malformed JSON at end, ignore
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		streamChunk := llm.StreamChunk{
			Delta:        choice.Delta.Content,
			FinishReason: choice.FinishReason,
			Done:         choice.FinishReason != "",
		}

		if chunk.Usage != nil {
			streamChunk.Usage = &llm.TokenUsage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
		}

		chunks <- streamChunk
	}
}

// Capabilities returns the provider's capabilities.
func (a *LocalAdapter) Capabilities() llm.Capabilities {
	return llm.Capabilities{
		SupportsTools:     false, // Most local models don't support tool calling
		SupportsStreaming: true,
		SupportsVision:    false, // Conservative default; varies by model
		MaxContextTokens:  8192,  // Conservative default; varies by model
		MaxOutputTokens:   2048,  // Conservative default; varies by model
		TokenizerType:     "",    // Unknown for local models
		Models:            []string{a.model},
	}
}

// Close releases resources held by the adapter.
func (a *LocalAdapter) Close() error {
	// No persistent resources to clean up
	return nil
}

// buildRequest converts our ChatRequest to the OpenAI-compatible format.
func (a *LocalAdapter) buildRequest(req llm.ChatRequest, stream bool) openAIChatRequest {
	messages := make([]openAIChatMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = openAIChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = defaultMaxTokens
	}

	temperature := req.Temperature
	if temperature == 0 {
		temperature = defaultTemperature
	}

	return openAIChatRequest{
		Model:       a.model,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		Stream:      stream,
		Stop:        req.Stop,
	}
}

// handleErrorResponse processes error responses from the API.
func (a *LocalAdapter) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp openAIErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error.Message != "" {
		return fmt.Errorf("%w: %s (code: %s)", llm.ErrAPIError, errResp.Error.Message, errResp.Error.Code)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return llm.ErrInvalidAPIKey
	case http.StatusNotFound:
		return fmt.Errorf("%w: model %q not found", llm.ErrModelNotFound, a.model)
	case http.StatusTooManyRequests:
		return llm.ErrRateLimited
	case http.StatusBadRequest:
		if bytes.Contains(body, []byte("context")) || bytes.Contains(body, []byte("token")) {
			return llm.ErrContextTooLong
		}
		return fmt.Errorf("%w: bad request - %s", llm.ErrAPIError, string(body))
	default:
		return fmt.Errorf("%w: HTTP %d - %s", llm.ErrAPIError, resp.StatusCode, string(body))
	}
}

// ModelName returns the name of the model being used.
func (a *LocalAdapter) ModelName() string {
	return a.model
}

// BaseURL returns the base URL of the server.
func (a *LocalAdapter) BaseURL() string {
	return a.baseURL
}

// Verify LocalAdapter implements Provider interface.
var _ llm.Provider = (*LocalAdapter)(nil)
