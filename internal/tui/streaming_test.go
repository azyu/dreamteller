package tui

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// StreamConfig Tests
// ============================================================================

func TestDefaultStreamConfig(t *testing.T) {
	config := DefaultStreamConfig()

	assert.Equal(t, 120*time.Second, config.Timeout)
	assert.Equal(t, 3, config.RetryAttempts)
	assert.Equal(t, 1*time.Second, config.RetryDelay)
}

func TestStreamConfig(t *testing.T) {
	config := StreamConfig{
		Timeout:       60 * time.Second,
		RetryAttempts: 5,
		RetryDelay:    2 * time.Second,
	}

	assert.Equal(t, 60*time.Second, config.Timeout)
	assert.Equal(t, 5, config.RetryAttempts)
	assert.Equal(t, 2*time.Second, config.RetryDelay)
}

// ============================================================================
// StreamController Tests
// ============================================================================

func TestNewStreamController(t *testing.T) {
	config := StreamConfig{
		Timeout:       5 * time.Second,
		RetryAttempts: 3,
		RetryDelay:    1 * time.Second,
	}

	sc := NewStreamController(config)

	assert.NotNil(t, sc)
	assert.NotNil(t, sc.ctx)
	assert.NotNil(t, sc.cancel)
	assert.Equal(t, config, sc.config)
}

func TestStreamController_Context(t *testing.T) {
	config := StreamConfig{Timeout: 5 * time.Second}
	sc := NewStreamController(config)

	ctx := sc.Context()

	assert.NotNil(t, ctx)

	// Context should not be done immediately
	select {
	case <-ctx.Done():
		t.Fatal("context should not be done")
	default:
		// Expected
	}
}

func TestStreamController_Cancel(t *testing.T) {
	config := StreamConfig{Timeout: 5 * time.Second}
	sc := NewStreamController(config)

	// Context should not be done before cancel
	select {
	case <-sc.Done():
		t.Fatal("context should not be done before cancel")
	default:
		// Expected
	}

	sc.Cancel()

	// Context should be done after cancel
	select {
	case <-sc.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context should be done after cancel")
	}
}

func TestStreamController_Done(t *testing.T) {
	config := StreamConfig{Timeout: 5 * time.Second}
	sc := NewStreamController(config)

	done := sc.Done()

	assert.NotNil(t, done)
	// Should be the same channel as context's done
	assert.Equal(t, sc.ctx.Done(), done)
}

func TestStreamController_Err(t *testing.T) {
	t.Run("returns nil before cancel", func(t *testing.T) {
		config := StreamConfig{Timeout: 5 * time.Second}
		sc := NewStreamController(config)

		assert.Nil(t, sc.Err())
	})

	t.Run("returns context.Canceled after cancel", func(t *testing.T) {
		config := StreamConfig{Timeout: 5 * time.Second}
		sc := NewStreamController(config)

		sc.Cancel()

		assert.Equal(t, context.Canceled, sc.Err())
	})
}

func TestStreamController_Timeout(t *testing.T) {
	// Use a very short timeout for testing
	config := StreamConfig{Timeout: 50 * time.Millisecond}
	sc := NewStreamController(config)

	// Wait for timeout
	select {
	case <-sc.Done():
		// Expected
		assert.Equal(t, context.DeadlineExceeded, sc.Err())
	case <-time.After(200 * time.Millisecond):
		t.Fatal("context should have timed out")
	}
}

// ============================================================================
// StreamHandler Tests
// ============================================================================

func TestNewStreamHandler(t *testing.T) {
	config := DefaultStreamConfig()
	sh := NewStreamHandler(config)

	assert.NotNil(t, sh)
	assert.NotNil(t, sh.controller)
	assert.NotNil(t, sh.chunks)
	assert.NotNil(t, sh.done)
}

func TestStreamHandler_SendChunk(t *testing.T) {
	config := DefaultStreamConfig()
	sh := NewStreamHandler(config)

	t.Run("sends chunk successfully", func(t *testing.T) {
		go sh.SendChunk("Hello")

		select {
		case chunk := <-sh.chunks:
			assert.Equal(t, "Hello", chunk)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("chunk should have been sent")
		}
	})
}

func TestStreamHandler_SendChunk_WhenCanceled(t *testing.T) {
	config := StreamConfig{Timeout: 50 * time.Millisecond}
	sh := NewStreamHandler(config)

	// Cancel the controller
	sh.controller.Cancel()

	// This should not block
	done := make(chan bool)
	go func() {
		sh.SendChunk("Should not block")
		done <- true
	}()

	select {
	case <-done:
		// Expected - should return immediately
	case <-time.After(100 * time.Millisecond):
		t.Fatal("SendChunk should not block when canceled")
	}
}

func TestStreamHandler_Complete(t *testing.T) {
	t.Run("completes with nil error", func(t *testing.T) {
		config := DefaultStreamConfig()
		sh := NewStreamHandler(config)

		go sh.Complete(nil)

		select {
		case err := <-sh.done:
			assert.Nil(t, err)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("done should have received completion")
		}
	})

	t.Run("completes with error", func(t *testing.T) {
		config := DefaultStreamConfig()
		sh := NewStreamHandler(config)
		testErr := errors.New("test error")

		go sh.Complete(testErr)

		select {
		case err := <-sh.done:
			assert.Equal(t, testErr, err)
		case <-time.After(100 * time.Millisecond):
			t.Fatal("done should have received error")
		}
	})

	t.Run("closes chunks channel", func(t *testing.T) {
		config := DefaultStreamConfig()
		sh := NewStreamHandler(config)

		sh.Complete(nil)

		// Chunks channel should be closed
		_, ok := <-sh.chunks
		assert.False(t, ok, "chunks channel should be closed")
	})
}

func TestStreamHandler_Cancel(t *testing.T) {
	config := DefaultStreamConfig()
	sh := NewStreamHandler(config)

	sh.Cancel()

	// Controller should be canceled
	select {
	case <-sh.controller.Done():
		// Expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("controller should be canceled")
	}
}

func TestStreamHandler_StreamToTea(t *testing.T) {
	t.Run("returns StreamChunkMsg for chunks", func(t *testing.T) {
		config := DefaultStreamConfig()
		sh := NewStreamHandler(config)

		go func() {
			sh.SendChunk("Hello")
			time.Sleep(10 * time.Millisecond)
			sh.Complete(nil)
		}()

		cmd := sh.StreamToTea()
		msg := cmd()

		chunkMsg, ok := msg.(StreamChunkMsg)
		require.True(t, ok)
		assert.Equal(t, "Hello", chunkMsg.Content)
	})

	t.Run("returns StreamDoneMsg on completion", func(t *testing.T) {
		config := DefaultStreamConfig()
		sh := NewStreamHandler(config)

		go func() {
			sh.Complete(nil)
		}()

		cmd := sh.StreamToTea()
		msg := cmd()

		_, ok := msg.(StreamDoneMsg)
		assert.True(t, ok)
	})

	t.Run("returns StreamErrorMsg on error", func(t *testing.T) {
		config := DefaultStreamConfig()
		sh := NewStreamHandler(config)
		testErr := errors.New("test error")

		go func() {
			sh.Complete(testErr)
		}()

		cmd := sh.StreamToTea()
		msg := cmd()

		errMsg, ok := msg.(StreamErrorMsg)
		require.True(t, ok)
		assert.Equal(t, testErr, errMsg.Err)
	})

	t.Run("returns StreamErrorMsg on context cancel", func(t *testing.T) {
		config := StreamConfig{Timeout: 50 * time.Millisecond}
		sh := NewStreamHandler(config)

		sh.Cancel()

		cmd := sh.StreamToTea()
		msg := cmd()

		errMsg, ok := msg.(StreamErrorMsg)
		require.True(t, ok)
		assert.Equal(t, context.Canceled, errMsg.Err)
	})

	t.Run("returns StreamErrorMsg on timeout", func(t *testing.T) {
		config := StreamConfig{Timeout: 50 * time.Millisecond}
		sh := NewStreamHandler(config)

		// Don't send anything, let it timeout
		cmd := sh.StreamToTea()
		msg := cmd()

		errMsg, ok := msg.(StreamErrorMsg)
		require.True(t, ok)
		assert.Equal(t, context.DeadlineExceeded, errMsg.Err)
	})
}

// ============================================================================
// RetryableStream Tests
// ============================================================================

func TestNewRetryableStream(t *testing.T) {
	config := StreamConfig{RetryAttempts: 3, RetryDelay: 1 * time.Second}
	rs := NewRetryableStream(config)

	assert.NotNil(t, rs)
	assert.Equal(t, config, rs.config)
	assert.Equal(t, 0, rs.attempt)
}

func TestRetryableStream_ShouldRetry(t *testing.T) {
	t.Run("returns false for nil error", func(t *testing.T) {
		config := StreamConfig{RetryAttempts: 3}
		rs := NewRetryableStream(config)

		assert.False(t, rs.ShouldRetry(nil))
		assert.Equal(t, 0, rs.attempt)
	})

	t.Run("returns true for regular error until max attempts", func(t *testing.T) {
		config := StreamConfig{RetryAttempts: 3}
		rs := NewRetryableStream(config)
		testErr := errors.New("network error")

		// First retry
		assert.True(t, rs.ShouldRetry(testErr))
		assert.Equal(t, 1, rs.attempt)

		// Second retry
		assert.True(t, rs.ShouldRetry(testErr))
		assert.Equal(t, 2, rs.attempt)

		// Third retry
		assert.True(t, rs.ShouldRetry(testErr))
		assert.Equal(t, 3, rs.attempt)

		// Fourth should not retry
		assert.False(t, rs.ShouldRetry(testErr))
		assert.Equal(t, 4, rs.attempt)
	})

	t.Run("returns false for context.Canceled", func(t *testing.T) {
		config := StreamConfig{RetryAttempts: 3}
		rs := NewRetryableStream(config)

		assert.False(t, rs.ShouldRetry(context.Canceled))
		assert.Equal(t, 0, rs.attempt) // Attempt not incremented
	})

	t.Run("returns false for context.DeadlineExceeded", func(t *testing.T) {
		config := StreamConfig{RetryAttempts: 3}
		rs := NewRetryableStream(config)

		assert.False(t, rs.ShouldRetry(context.DeadlineExceeded))
		assert.Equal(t, 0, rs.attempt)
	})
}

func TestRetryableStream_WaitForRetry(t *testing.T) {
	config := StreamConfig{RetryDelay: 10 * time.Millisecond}
	rs := NewRetryableStream(config)

	start := time.Now()
	rs.WaitForRetry()
	elapsed := time.Since(start)

	// Should have waited approximately the retry delay
	assert.True(t, elapsed >= 10*time.Millisecond, "should wait at least retry delay")
	assert.True(t, elapsed < 50*time.Millisecond, "should not wait too long")
}

func TestRetryableStream_Attempt(t *testing.T) {
	config := StreamConfig{RetryAttempts: 3}
	rs := NewRetryableStream(config)

	assert.Equal(t, 0, rs.Attempt())

	rs.ShouldRetry(errors.New("error"))
	assert.Equal(t, 1, rs.Attempt())

	rs.ShouldRetry(errors.New("error"))
	assert.Equal(t, 2, rs.Attempt())
}

func TestRetryableStream_Reset(t *testing.T) {
	config := StreamConfig{RetryAttempts: 3}
	rs := NewRetryableStream(config)

	// Make some attempts
	rs.ShouldRetry(errors.New("error"))
	rs.ShouldRetry(errors.New("error"))
	assert.Equal(t, 2, rs.Attempt())

	rs.Reset()

	assert.Equal(t, 0, rs.Attempt())
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestStreamingFlow(t *testing.T) {
	t.Run("full streaming flow with chunks", func(t *testing.T) {
		config := StreamConfig{
			Timeout:       5 * time.Second,
			RetryAttempts: 3,
			RetryDelay:    100 * time.Millisecond,
		}
		sh := NewStreamHandler(config)

		// Simulate streaming in goroutine
		go func() {
			chunks := []string{"Hello", " ", "World", "!"}
			for _, chunk := range chunks {
				sh.SendChunk(chunk)
				time.Sleep(10 * time.Millisecond)
			}
			sh.Complete(nil)
		}()

		// Collect all chunks
		var received []string
		for {
			cmd := sh.StreamToTea()
			msg := cmd()

			switch m := msg.(type) {
			case StreamChunkMsg:
				received = append(received, m.Content)
			case StreamDoneMsg:
				// Done
				assert.Equal(t, []string{"Hello", " ", "World", "!"}, received)
				return
			case StreamErrorMsg:
				t.Fatalf("unexpected error: %v", m.Err)
			}
		}
	})

	t.Run("streaming flow with error", func(t *testing.T) {
		config := DefaultStreamConfig()
		sh := NewStreamHandler(config)
		testErr := errors.New("connection lost")

		// Simulate error during streaming
		go func() {
			sh.SendChunk("Partial")
			time.Sleep(10 * time.Millisecond)
			sh.Complete(testErr)
		}()

		// First should be chunk
		cmd := sh.StreamToTea()
		msg := cmd()
		chunkMsg, ok := msg.(StreamChunkMsg)
		require.True(t, ok)
		assert.Equal(t, "Partial", chunkMsg.Content)

		// Second should be error
		cmd = sh.StreamToTea()
		msg = cmd()
		errMsg, ok := msg.(StreamErrorMsg)
		require.True(t, ok)
		assert.Equal(t, testErr, errMsg.Err)
	})

	t.Run("retry flow", func(t *testing.T) {
		config := StreamConfig{
			Timeout:       1 * time.Second,
			RetryAttempts: 3,
			RetryDelay:    10 * time.Millisecond,
		}
		rs := NewRetryableStream(config)

		// Simulate retrying on error
		attemptCount := 0
		for {
			attemptCount++
			err := errors.New("temporary error")

			if attemptCount < 3 {
				assert.True(t, rs.ShouldRetry(err))
				rs.WaitForRetry()
			} else {
				// Simulate success on third attempt
				break
			}
		}

		assert.Equal(t, 3, attemptCount)
		assert.Equal(t, 2, rs.Attempt()) // Two retries were recorded
	})
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestStreamController_MultipleCancel(t *testing.T) {
	config := StreamConfig{Timeout: 5 * time.Second}
	sc := NewStreamController(config)

	// Cancel multiple times should not panic
	sc.Cancel()
	sc.Cancel()
	sc.Cancel()

	assert.Equal(t, context.Canceled, sc.Err())
}

func TestStreamHandler_CompleteTwice(t *testing.T) {
	// The current implementation panics on double close.
	// This test documents that behavior - users should not call Complete twice.
	// If protection is added later, this test can be updated.
	t.Run("documents that double complete panics", func(t *testing.T) {
		config := DefaultStreamConfig()
		sh := NewStreamHandler(config)

		// First complete
		sh.Complete(nil)

		// Second complete will panic (close of closed channel)
		// We use defer/recover to verify this expected behavior
		defer func() {
			if r := recover(); r == nil {
				t.Log("Implementation now supports multiple Complete calls - update this test")
			} else {
				// Expected - double close panics
				t.Log("Double Complete panics as expected (close of closed channel)")
			}
		}()

		sh.Complete(errors.New("will panic"))
	})
}

func TestRetryableStream_ZeroRetries(t *testing.T) {
	config := StreamConfig{RetryAttempts: 0}
	rs := NewRetryableStream(config)

	// Should not retry at all
	assert.False(t, rs.ShouldRetry(errors.New("error")))
}

func TestRetryableStream_NegativeRetries(t *testing.T) {
	config := StreamConfig{RetryAttempts: -1}
	rs := NewRetryableStream(config)

	// Should not retry with negative attempts
	assert.False(t, rs.ShouldRetry(errors.New("error")))
}
