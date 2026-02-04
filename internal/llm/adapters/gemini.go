// Package adapters provides LLM provider implementations.
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/azyu/dreamteller/internal/llm"
	"google.golang.org/genai"
)

// geminiModelCapabilities maps model names to their capabilities.
var geminiModelCapabilities = map[string]llm.Capabilities{
	"gemini-2.0-flash": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    true,
		MaxContextTokens:  1048576,
		MaxOutputTokens:   8192,
		TokenizerType:     "gemini",
		Models:            []string{"gemini-2.0-flash"},
	},
	"gemini-2.0-flash-lite": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    true,
		MaxContextTokens:  1048576,
		MaxOutputTokens:   8192,
		TokenizerType:     "gemini",
		Models:            []string{"gemini-2.0-flash-lite"},
	},
	"gemini-2.5-pro": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    true,
		MaxContextTokens:  1048576,
		MaxOutputTokens:   65536,
		TokenizerType:     "gemini",
		Models:            []string{"gemini-2.5-pro"},
	},
	"gemini-2.5-flash": {
		SupportsTools:     true,
		SupportsStreaming: true,
		SupportsVision:    true,
		MaxContextTokens:  1048576,
		MaxOutputTokens:   65536,
		TokenizerType:     "gemini",
		Models:            []string{"gemini-2.5-flash"},
	},
}

// defaultGeminiCapabilities are used when the model is not in the known list.
var defaultGeminiCapabilities = llm.Capabilities{
	SupportsTools:     true,
	SupportsStreaming: true,
	SupportsVision:    true,
	MaxContextTokens:  128000,
	MaxOutputTokens:   8192,
	TokenizerType:     "gemini",
}

// GeminiAdapter implements the Provider interface for Google's Gemini API.
type GeminiAdapter struct {
	client *genai.Client
	model  string
}

// GeminiAdapterOption configures a GeminiAdapter.
type GeminiAdapterOption func(*geminiConfig)

type geminiConfig struct {
	// Additional configuration options can be added here
}

// NewGeminiAdapter creates a new GeminiAdapter for Google's Gemini API.
// The apiKey should be a valid Gemini API key.
// The model should be the model name to use (e.g., "gemini-2.5-flash").
func NewGeminiAdapter(ctx context.Context, apiKey, model string, opts ...GeminiAdapterOption) (*GeminiAdapter, error) {
	if apiKey == "" {
		return nil, llm.ErrInvalidAPIKey
	}

	cfg := &geminiConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &GeminiAdapter{
		client: client,
		model:  model,
	}, nil
}

// Chat sends a chat completion request and returns the complete response.
func (a *GeminiAdapter) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	contents, systemInstruction := a.convertMessages(req.Messages)
	config := a.buildConfig(req, systemInstruction)

	result, err := a.client.Models.GenerateContent(ctx, a.model, contents, config)
	if err != nil {
		return nil, a.wrapError(err)
	}

	return a.convertResponse(result)
}

// Stream sends a chat completion request and returns a channel of streaming chunks.
func (a *GeminiAdapter) Stream(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	contents, systemInstruction := a.convertMessages(req.Messages)
	config := a.buildConfig(req, systemInstruction)

	chunks := make(chan llm.StreamChunk, 100)

	go a.processStream(ctx, contents, config, chunks)

	return chunks, nil
}

// processStream handles the streaming response from Gemini.
func (a *GeminiAdapter) processStream(ctx context.Context, contents []*genai.Content, config *genai.GenerateContentConfig, chunks chan<- llm.StreamChunk) {
	defer close(chunks)

	iter := a.client.Models.GenerateContentStream(ctx, a.model, contents, config)

	for result, err := range iter {
		select {
		case <-ctx.Done():
			chunks <- llm.StreamChunk{
				Error: ctx.Err(),
				Done:  true,
			}
			return
		default:
		}

		if err != nil {
			chunks <- llm.StreamChunk{
				Error: a.wrapError(err),
				Done:  true,
			}
			return
		}

		chunk := a.convertStreamChunk(result)
		chunks <- chunk

		if chunk.Done {
			return
		}
	}

	// Send final done chunk
	chunks <- llm.StreamChunk{Done: true}
}

// Capabilities returns the provider's capabilities.
func (a *GeminiAdapter) Capabilities() llm.Capabilities {
	// Check for exact match first
	if caps, ok := geminiModelCapabilities[a.model]; ok {
		return caps
	}

	// Check for partial matches (e.g., "gemini-2.5-flash-preview" matches "gemini-2.5-flash")
	for modelPrefix, caps := range geminiModelCapabilities {
		if strings.HasPrefix(a.model, modelPrefix) {
			capsWithModel := caps
			capsWithModel.Models = []string{a.model}
			return capsWithModel
		}
	}

	// Return default capabilities for unknown models
	caps := defaultGeminiCapabilities
	caps.Models = []string{a.model}
	return caps
}

// Close releases resources held by the adapter.
func (a *GeminiAdapter) Close() error {
	// The genai client doesn't have a Close method, so nothing to clean up
	return nil
}

// convertMessages converts our ChatMessage slice to Gemini's Content format.
// Returns the contents and an optional system instruction.
func (a *GeminiAdapter) convertMessages(messages []llm.ChatMessage) ([]*genai.Content, *genai.Content) {
	var systemInstruction *genai.Content
	var contents []*genai.Content

	for _, msg := range messages {
		switch msg.Role {
		case llm.RoleSystem:
			// Gemini uses SystemInstruction for system messages
			systemInstruction = &genai.Content{
				Parts: []*genai.Part{
					{Text: msg.Content},
				},
			}

		case llm.RoleUser:
			contents = append(contents, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{
					{Text: msg.Content},
				},
			})

		case llm.RoleAssistant:
			content := &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{},
			}

			if msg.Content != "" {
				content.Parts = append(content.Parts, &genai.Part{Text: msg.Content})
			}

			// Handle tool calls
			for _, tc := range msg.ToolCalls {
				var args map[string]any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					args = make(map[string]any)
				}
				content.Parts = append(content.Parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						Name: tc.Function.Name,
						Args: args,
					},
				})
			}

			contents = append(contents, content)

		case llm.RoleTool:
			// Gemini expects function responses in a specific format
			var responseMap map[string]any
			if err := json.Unmarshal([]byte(msg.Content), &responseMap); err != nil {
				// If not valid JSON, wrap the content in a map
				responseMap = map[string]any{"output": msg.Content}
			}

			contents = append(contents, &genai.Content{
				Role: "user",
				Parts: []*genai.Part{
					{
						FunctionResponse: &genai.FunctionResponse{
							Name:     msg.Name,
							Response: responseMap,
						},
					},
				},
			})
		}
	}

	return contents, systemInstruction
}

// buildConfig creates the GenerateContentConfig from our ChatRequest.
func (a *GeminiAdapter) buildConfig(req llm.ChatRequest, systemInstruction *genai.Content) *genai.GenerateContentConfig {
	config := &genai.GenerateContentConfig{}

	if systemInstruction != nil {
		config.SystemInstruction = systemInstruction
	}

	if req.MaxTokens > 0 {
		config.MaxOutputTokens = int32(req.MaxTokens)
	}

	if req.Temperature > 0 {
		config.Temperature = genai.Ptr(float32(req.Temperature))
	}

	if len(req.Stop) > 0 {
		config.StopSequences = req.Stop
	}

	config.SafetySettings = []*genai.SafetySetting{
		{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockThresholdBlockNone},
	}

	if len(req.Tools) > 0 {
		config.Tools = a.convertTools(req.Tools)
	}

	return config
}

// convertTools converts our ToolDefinition slice to Gemini's Tool format.
func (a *GeminiAdapter) convertTools(tools []llm.ToolDefinition) []*genai.Tool {
	var geminiTools []*genai.Tool

	for _, tool := range tools {
		if tool.Type != "function" {
			continue
		}

		funcDecl := &genai.FunctionDeclaration{
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
		}

		// Convert parameters to JSON schema format expected by Gemini
		if tool.Function.Parameters != nil {
			funcDecl.ParametersJsonSchema = tool.Function.Parameters
		}

		geminiTools = append(geminiTools, &genai.Tool{
			FunctionDeclarations: []*genai.FunctionDeclaration{funcDecl},
		})
	}

	return geminiTools
}

// convertResponse converts Gemini's response to our ChatResponse format.
func (a *GeminiAdapter) convertResponse(result *genai.GenerateContentResponse) (*llm.ChatResponse, error) {
	if len(result.Candidates) == 0 {
		return nil, fmt.Errorf("%w: no candidates in response", llm.ErrAPIError)
	}

	candidate := result.Candidates[0]
	response := &llm.ChatResponse{
		Model: a.model,
	}

	// Convert finish reason
	response.FinishReason = a.convertFinishReason(candidate.FinishReason)

	// Extract content and tool calls from parts
	var contentParts []string
	var toolCalls []llm.ToolCall

	if candidate.Content != nil {
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				contentParts = append(contentParts, part.Text)
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				toolCalls = append(toolCalls, llm.ToolCall{
					ID:   fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, len(toolCalls)),
					Type: "function",
					Function: llm.FunctionCall{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				})
			}
		}
	}

	response.Message = llm.ChatMessage{
		Role:      llm.RoleAssistant,
		Content:   strings.Join(contentParts, ""),
		ToolCalls: toolCalls,
	}

	// Convert usage
	if result.UsageMetadata != nil {
		response.Usage = llm.TokenUsage{
			PromptTokens:     int(result.UsageMetadata.PromptTokenCount),
			CompletionTokens: int(result.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      int(result.UsageMetadata.TotalTokenCount),
		}
	}

	return response, nil
}

// convertStreamChunk converts a Gemini streaming response to our StreamChunk format.
func (a *GeminiAdapter) convertStreamChunk(result *genai.GenerateContentResponse) llm.StreamChunk {
	chunk := llm.StreamChunk{}

	if len(result.Candidates) == 0 {
		return chunk
	}

	candidate := result.Candidates[0]

	// Extract text content
	if candidate.Content != nil {
		for i, part := range candidate.Content.Parts {
			if part.Text != "" {
				chunk.Delta += part.Text
			}
			if part.FunctionCall != nil {
				argsJSON, _ := json.Marshal(part.FunctionCall.Args)
				chunk.ToolCall = &llm.ToolCallDelta{
					Index: i,
					ID:    fmt.Sprintf("call_%s_%d", part.FunctionCall.Name, i),
					Type:  "function",
					Function: &llm.FunctionCallDelta{
						Name:      part.FunctionCall.Name,
						Arguments: string(argsJSON),
					},
				}
			}
		}
	}

	// Convert finish reason
	if candidate.FinishReason != "" {
		chunk.FinishReason = a.convertFinishReason(candidate.FinishReason)
		chunk.Done = true
	}

	// Convert usage if available
	if result.UsageMetadata != nil {
		chunk.Usage = &llm.TokenUsage{
			PromptTokens:     int(result.UsageMetadata.PromptTokenCount),
			CompletionTokens: int(result.UsageMetadata.CandidatesTokenCount),
			TotalTokens:      int(result.UsageMetadata.TotalTokenCount),
		}
	}

	return chunk
}

// convertFinishReason converts Gemini's finish reason to our format.
func (a *GeminiAdapter) convertFinishReason(reason genai.FinishReason) string {
	switch reason {
	case genai.FinishReasonStop:
		return llm.FinishReasonStop
	case genai.FinishReasonMaxTokens:
		return llm.FinishReasonLength
	case genai.FinishReasonSafety, genai.FinishReasonRecitation, genai.FinishReasonBlocklist:
		return llm.FinishReasonContentFilter
	default:
		if reason != "" {
			return string(reason)
		}
		return ""
	}
}

// wrapError wraps Gemini errors in our error types.
func (a *GeminiAdapter) wrapError(err error) error {
	if err == nil {
		return nil
	}

	errStr := err.Error()

	// Check for common error patterns
	switch {
	case strings.Contains(errStr, "API key"):
		return fmt.Errorf("%w: %s", llm.ErrInvalidAPIKey, errStr)
	case strings.Contains(errStr, "not found") || strings.Contains(errStr, "404"):
		return fmt.Errorf("%w: %s", llm.ErrModelNotFound, errStr)
	case strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "429"):
		return llm.ErrRateLimited
	case strings.Contains(errStr, "context") && strings.Contains(errStr, "token"):
		return llm.ErrContextTooLong
	default:
		return fmt.Errorf("%w: %s", llm.ErrAPIError, errStr)
	}
}

// ModelName returns the name of the model being used.
func (a *GeminiAdapter) ModelName() string {
	return a.model
}

// Verify GeminiAdapter implements Provider interface.
var _ llm.Provider = (*GeminiAdapter)(nil)
