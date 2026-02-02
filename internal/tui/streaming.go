// Package tui provides the terminal user interface using Bubble Tea.
package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// StreamConfig configures streaming behavior.
type StreamConfig struct {
	// Timeout for the entire streaming operation
	Timeout time.Duration
	// RetryAttempts for failed requests
	RetryAttempts int
	// RetryDelay between retry attempts
	RetryDelay time.Duration
}

// DefaultStreamConfig returns sensible defaults.
func DefaultStreamConfig() StreamConfig {
	return StreamConfig{
		Timeout:       120 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    1 * time.Second,
	}
}

// StreamController manages streaming operations with cancellation support.
type StreamController struct {
	ctx    context.Context
	cancel context.CancelFunc
	config StreamConfig
}

// NewStreamController creates a new stream controller.
func NewStreamController(config StreamConfig) *StreamController {
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	return &StreamController{
		ctx:    ctx,
		cancel: cancel,
		config: config,
	}
}

// Context returns the stream context.
func (sc *StreamController) Context() context.Context {
	return sc.ctx
}

// Cancel cancels the streaming operation.
func (sc *StreamController) Cancel() {
	sc.cancel()
}

// Done returns the context's done channel.
func (sc *StreamController) Done() <-chan struct{} {
	return sc.ctx.Done()
}

// Err returns any context error.
func (sc *StreamController) Err() error {
	return sc.ctx.Err()
}

// StreamHandler handles streaming responses.
type StreamHandler struct {
	controller *StreamController
	chunks     chan string
	done       chan error
}

// NewStreamHandler creates a new stream handler.
func NewStreamHandler(config StreamConfig) *StreamHandler {
	return &StreamHandler{
		controller: NewStreamController(config),
		chunks:     make(chan string, 100),
		done:       make(chan error, 1),
	}
}

// SendChunk sends a chunk to the handler.
func (sh *StreamHandler) SendChunk(content string) {
	select {
	case sh.chunks <- content:
	case <-sh.controller.Done():
	}
}

// Complete marks the stream as complete.
func (sh *StreamHandler) Complete(err error) {
	select {
	case sh.done <- err:
	default:
	}
	close(sh.chunks)
}

// Cancel cancels the stream.
func (sh *StreamHandler) Cancel() {
	sh.controller.Cancel()
}

// StreamToTea converts stream handler events to Bubble Tea messages.
func (sh *StreamHandler) StreamToTea() tea.Cmd {
	return func() tea.Msg {
		for {
			select {
			case chunk, ok := <-sh.chunks:
				if !ok {
					// Channel closed, check for error
					select {
					case err := <-sh.done:
						if err != nil {
							return StreamErrorMsg{Err: err}
						}
						return StreamDoneMsg{}
					default:
						return StreamDoneMsg{}
					}
				}
				return StreamChunkMsg{Content: chunk}

			case err := <-sh.done:
				if err != nil {
					return StreamErrorMsg{Err: err}
				}
				return StreamDoneMsg{}

			case <-sh.controller.Done():
				return StreamErrorMsg{Err: sh.controller.Err()}
			}
		}
	}
}

// RetryableStream wraps a stream operation with retry logic.
type RetryableStream struct {
	config  StreamConfig
	attempt int
}

// NewRetryableStream creates a new retryable stream.
func NewRetryableStream(config StreamConfig) *RetryableStream {
	return &RetryableStream{
		config:  config,
		attempt: 0,
	}
}

// ShouldRetry returns true if retry should be attempted.
func (rs *RetryableStream) ShouldRetry(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry on context cancellation
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	rs.attempt++
	return rs.attempt <= rs.config.RetryAttempts
}

// WaitForRetry waits for the retry delay.
func (rs *RetryableStream) WaitForRetry() {
	time.Sleep(rs.config.RetryDelay)
}

// Attempt returns the current attempt number.
func (rs *RetryableStream) Attempt() int {
	return rs.attempt
}

// Reset resets the retry counter.
func (rs *RetryableStream) Reset() {
	rs.attempt = 0
}
