package tui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/azyu/dreamteller/internal/llm"
	"github.com/azyu/dreamteller/internal/project"
	"github.com/azyu/dreamteller/internal/search"
	"github.com/azyu/dreamteller/pkg/types"
	"github.com/stretchr/testify/require"
)

type stubProvider struct {
	caps llm.Capabilities
}

func (p stubProvider) Chat(ctx context.Context, req llm.ChatRequest) (*llm.ChatResponse, error) {
	return nil, llm.ErrAPIError
}

func (p stubProvider) Stream(ctx context.Context, req llm.ChatRequest) (<-chan llm.StreamChunk, error) {
	return nil, llm.ErrStreamingNotSupported
}

func (p stubProvider) Capabilities() llm.Capabilities { return p.caps }
func (p stubProvider) Close() error                   { return nil }

func TestAssembleChatRequest_OrderingAndSingleSystem(t *testing.T) {
	proj := createTempProjectWithContext(t)

	provider := stubProvider{caps: llm.Capabilities{
		MaxContextTokens:  800,
		MaxOutputTokens:   128,
		TokenizerType:     "gemini",
		SupportsStreaming: true,
	}}

	msgs := []Message{
		{Role: "user", Content: "안녕"},
		{Role: "assistant", Content: "안녕하세요"},
		{Role: "user", Content: "이 캐릭터 설정을 기반으로 1문단 장면 써줘"},
	}

	assembled, err := assembleChatRequest(proj, provider, "gemini-2.0-flash", ContextHybrid, nil, msgs)
	require.NoError(t, err)

	// Exactly one system message.
	sysCount := 0
	for _, m := range assembled.Request.Messages {
		if m.Role == llm.RoleSystem {
			sysCount++
		}
	}
	require.Equal(t, 1, sysCount)

	// Last message is the current user message.
	last := assembled.Request.Messages[len(assembled.Request.Messages)-1]
	require.Equal(t, llm.RoleUser, last.Role)
	require.Contains(t, last.Content, "장면")

	// Canonical facts are placed near the start of system prompt.
	require.Contains(t, assembled.SystemPrompt, "## 정설(캐릭터)")
	require.Contains(t, assembled.SystemPrompt, "- 하나: ")
}

func TestAssembleChatRequest_HistoryCompressionInjectsSummary(t *testing.T) {
	provider := stubProvider{caps: llm.Capabilities{
		MaxContextTokens:  200,
		MaxOutputTokens:   64,
		TokenizerType:     "cl100k_base",
		SupportsStreaming: true,
	}}

	// Many prior messages to exceed history budget.
	msgs := []Message{
		{Role: "user", Content: strings.Repeat("user words ", 80)},
		{Role: "assistant", Content: strings.Repeat("assistant words ", 80)},
		{Role: "user", Content: strings.Repeat("more words ", 80)},
		{Role: "assistant", Content: strings.Repeat("even more words ", 80)},
		{Role: "user", Content: strings.Repeat("older context ", 80)},
		{Role: "assistant", Content: strings.Repeat("older reply ", 80)},
		{Role: "user", Content: strings.Repeat("older notes ", 80)},
		{Role: "assistant", Content: strings.Repeat("older response ", 80)},
		{Role: "user", Content: "질문: 다음 장면에서 갈등을 어떻게 키울까?"},
	}

	assembled, err := assembleChatRequest(nil, provider, "gpt-4", ContextEssential, nil, msgs)
	require.NoError(t, err)

	// Summary message should be injected (assistant role) before last user.
	foundSummary := false
	for i, m := range assembled.Request.Messages {
		if strings.Contains(m.Content, "이전 대화 요약") {
			foundSummary = true
			// Ensure it's not the last message.
			require.Less(t, i, len(assembled.Request.Messages)-1)
			require.Equal(t, llm.RoleAssistant, m.Role)
			break
		}
	}
	require.True(t, foundSummary)

	last := assembled.Request.Messages[len(assembled.Request.Messages)-1]
	require.Equal(t, llm.RoleUser, last.Role)
}

func TestBuildBudgetedRetrievalMessage_RespectsMaxChunks(t *testing.T) {
	proj := createTempProjectWithContext(t)
	// Force MaxChunks=1 so selection is deterministic.
	proj.Config.Context.MaxChunks = 1

	engine := search.NewFTSEngine(proj.DB)
	// Index a few chunks that match the same query.
	require.NoError(t, engine.Index("dragon dragon dragon CHUNK_ONE", "chapter", "chapters/ch1.md", 200, types.DefaultProjectConfig("x", "y").CreatedAt, ""))
	require.NoError(t, engine.Index("dragon CHUNK_TWO", "chapter", "chapters/ch2.md", 200, types.DefaultProjectConfig("x", "y").CreatedAt, ""))
	require.NoError(t, engine.Index("dragon CHUNK_THREE", "chapter", "chapters/ch3.md", 200, types.DefaultProjectConfig("x", "y").CreatedAt, ""))

	provider := stubProvider{caps: llm.Capabilities{
		MaxContextTokens:  2000,
		MaxOutputTokens:   128,
		TokenizerType:     "cl100k_base",
		SupportsStreaming: true,
	}}

	env, err := newAssemblyEnv(proj, provider, "gpt-4")
	require.NoError(t, err)

	msg := buildBudgetedRetrievalMessage(engine, env.cm, env.tokenizer, 1000, "dragon")
	require.NotNil(t, msg)

	// MaxChunks=1 => only one chunk marker should appear.
	count := 0
	for _, marker := range []string{"CHUNK_ONE", "CHUNK_TWO", "CHUNK_THREE"} {
		if strings.Contains(msg.Content, marker) {
			count++
		}
	}
	require.Equal(t, 1, count)
}

func createTempProjectWithContext(t *testing.T) *project.Project {
	t.Helper()

	root := t.TempDir()
	mgr, err := project.NewManager(root)
	require.NoError(t, err)

	cfg := types.DefaultProjectConfig("Test Novel", "fantasy")
	proj, err := mgr.Create("test-novel", cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = proj.Close() })

	// Add context files.
	require.NoError(t, os.WriteFile(filepath.Join(proj.Path(), "context", "characters", "hana.md"), []byte(
		"# 하나\n\n- 주인공\n- 냉정하지만 따뜻함\n\n추가 설명.",
	), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(proj.Path(), "context", "settings", "seoul.md"), []byte(
		"# 서울\n\n- 현대\n- 빗속의 네온\n",
	), 0644))

	return proj
}
