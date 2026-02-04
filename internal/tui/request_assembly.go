package tui

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/azyu/dreamteller/internal/llm"
	"github.com/azyu/dreamteller/internal/project"
	"github.com/azyu/dreamteller/internal/search"
	"github.com/azyu/dreamteller/internal/token"
	"github.com/azyu/dreamteller/pkg/types"
)

const (
	defaultSearchCandidateLimit = 50
	defaultHistoryLoadLimit     = 500

	defaultRecentMessagesToKeep = 6

	defaultUnknownTokenizerSafetyMargin = 0.15
	defaultKnownTokenizerSafetyMargin   = 0.07
)

var errUserMessageTooLarge = errors.New("user message too large to fit within history budget")

type assembledRequest struct {
	Request llm.ChatRequest

	// Debug fields used by tests.
	SystemPrompt string
	Budget       token.BudgetAllocation
}

type assemblyEnv struct {
	caps      llm.Capabilities
	tokenizer llm.TokenCounter

	budget token.BudgetAllocation
	cm     *llm.ContextManager
}

func newAssemblyEnv(proj *project.Project, provider llm.Provider, modelName string) (assemblyEnv, error) {
	if provider == nil {
		return assemblyEnv{}, fmt.Errorf("provider is nil")
	}

	caps := provider.Capabilities()
	maxContext := caps.MaxContextTokens
	if maxContext <= 0 {
		maxContext = token.DefaultContextLimit
	}

	encoding := caps.TokenizerType
	safetyMargin := defaultKnownTokenizerSafetyMargin
	if encoding == "" || encoding == "gemini" {
		encoding = "cl100k_base"
		safetyMargin = defaultUnknownTokenizerSafetyMargin
	}

	counter, err := token.NewCounter(encoding)
	if err != nil {
		counter = nil
		safetyMargin = defaultUnknownTokenizerSafetyMargin
	}

	maxForBudget := int(float64(maxContext) * (1.0 - safetyMargin))
	if maxForBudget <= 0 {
		maxForBudget = maxContext
	}

	ratios := token.DefaultBudgetRatios
	contextCfg := types.ContextConfig{MaxChunks: 10}
	if proj != nil && proj.Config != nil {
		if token.ValidateRatios(proj.Config.Budget) {
			ratios = proj.Config.Budget
		}
		if proj.Config.Context.MaxChunks > 0 {
			contextCfg.MaxChunks = proj.Config.Context.MaxChunks
		}
	}

	bm := token.NewBudgetManagerWithConfig(modelName, maxForBudget, ratios)
	budget := bm.GetBudget()

	var cmTokenizer llm.TokenCounter
	if counter != nil {
		cmTokenizer = counter
	} else {
		cmTokenizer = tokenEstimateCounter{}
	}

	cm := llm.NewContextManager(contextCfg, ratios, maxForBudget, cmTokenizer)

	return assemblyEnv{
		caps:      caps,
		tokenizer: cmTokenizer,
		budget:    budget,
		cm:        cm,
	}, nil
}

func assembleChatRequest(
	proj *project.Project,
	provider llm.Provider,
	modelName string,
	contextMode ContextMode,
	searchEngine *search.FTSEngine,
	messages []Message,
) (assembledRequest, error) {
	env, err := newAssemblyEnv(proj, provider, modelName)
	if err != nil {
		return assembledRequest{}, err
	}

	userMsg, priorHistory := splitCurrentUserMessage(messages)
	if userMsg == nil {
		return assembledRequest{}, fmt.Errorf("no user message to send")
	}

	// System prompt: role + canonical facts (Korean) + project info/style + mode context.
	systemPrompt := buildBudgetedSystemPrompt(proj, contextMode, env.tokenizer, env.budget.SystemPrompt)

	chatMessages := []llm.ChatMessage{llm.NewSystemMessage(systemPrompt)}

	// Hybrid: retrieval injection goes into middle as a NON-system message.
	if contextMode == ContextHybrid {
		if retrieval := buildBudgetedRetrievalMessage(searchEngine, env.cm, env.tokenizer, env.budget.Context, userMsg.Content); retrieval != nil {
			chatMessages = append(chatMessages, *retrieval)
		}
	}

	// History compression (Phase 2): summarize older history when it would exceed budget.
	// The summary message is injected before the preserved recent history.
	historyMsgs := convertTUIMessagesToLLM(priorHistory)
	if needsHistoryCompression(env.tokenizer, historyMsgs, userMsg.Content, env.budget.History) {
		summary, remaining := env.cm.SummarizeHistory(historyMsgs, defaultRecentMessagesToKeep)
		summary = strings.TrimSpace(summary)
		if summary != "" {
			summaryContent := "이전 대화 요약:\n" + summary
			chatMessages = append(chatMessages, llm.NewAssistantMessage(summaryContent))
		}
		historyMsgs = remaining
	}

	// Truncate history to fit within history budget (ensure current user message remains last).
	truncated, err := truncateHistoryPreservingLastUser(env.tokenizer, historyMsgs, *userMsg, env.budget.History)
	if err != nil {
		return assembledRequest{}, err
	}
	chatMessages = append(chatMessages, truncated...)
	chatMessages = append(chatMessages, llm.NewUserMessage(userMsg.Content))

	maxOut := env.budget.Response
	if env.caps.MaxOutputTokens > 0 && maxOut > env.caps.MaxOutputTokens {
		maxOut = env.caps.MaxOutputTokens
	}
	if maxOut <= 0 {
		maxOut = 1024
	}

	return assembledRequest{
		Request: llm.ChatRequest{
			Messages:    chatMessages,
			MaxTokens:   maxOut,
			Temperature: 0.7,
			Tools:       llm.PredefinedTools(),
		},
		SystemPrompt: systemPrompt,
		Budget:       env.budget,
	}, nil
}

func splitCurrentUserMessage(messages []Message) (user *Message, history []Message) {
	if len(messages) == 0 {
		return nil, nil
	}

	last := messages[len(messages)-1]
	if last.Role != llm.RoleUser {
		// Fallback: search for last user message.
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == llm.RoleUser {
				m := messages[i]
				h := append([]Message{}, messages[:i]...)
				return &m, h
			}
		}
		return nil, messages
	}

	m := last
	return &m, append([]Message{}, messages[:len(messages)-1]...)
}

func buildBudgetedSystemPrompt(proj *project.Project, mode ContextMode, tokenizer llm.TokenCounter, systemBudget int) string {
	// NOTE: We intentionally put canonical facts BEFORE the general role prompt.
	// The default role prompt is long, and for small budgets it can crowd out
	// the facts. Putting facts first ensures they survive truncation.
	var parts []string
	if facts := buildCanonicalFactsKorean(proj); facts != "" {
		parts = append(parts, facts)
	}
	parts = append(parts, llm.DefaultNovelWritingPrompt())

	if proj != nil && proj.Info != nil {
		parts = append(parts, fmt.Sprintf("You are helping write a %s novel titled \"%s\".", proj.Config.Genre, proj.Info.Name))
		parts = append(parts, fmt.Sprintf(`Writing Guidelines:
- Style: %s
- Point of View: %s
- Tense: %s`, proj.Config.Writing.Style, proj.Config.Writing.POV, proj.Config.Writing.Tense))
	}

	// Mode-specific static context remains in system prompt.
	// Retrieval context is injected as a non-system message (Hybrid only).
	var modeContext string
	switch mode {
	case ContextEssential, ContextHybrid:
		modeContext = buildEssentialContextAsync(proj)
	case ContextFull:
		modeContext = buildFullContextAsync(proj)
	}
	if modeContext != "" {
		parts = append(parts, modeContext)
	}

	prompt := strings.Join(parts, "\n\n")
	if systemBudget <= 0 {
		return prompt
	}

	// Keep the beginning (canonical facts) if we must trim.
	return truncateToTokens(tokenizer, prompt, systemBudget, false)
}

func buildBudgetedRetrievalMessage(
	searchEngine *search.FTSEngine,
	cm *llm.ContextManager,
	tokenizer llm.TokenCounter,
	contextBudget int,
	userInput string,
) *llm.ChatMessage {
	if searchEngine == nil || userInput == "" || contextBudget <= 0 {
		return nil
	}

	results, err := searchEngine.Search(userInput, defaultSearchCandidateLimit)
	if err != nil || len(results) == 0 {
		return nil
	}

	// Search returns results ordered by score (bm25), lower is better.
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].Score < results[j].Score
	})

	chunks := make([]llm.ContextChunk, 0, len(results))
	for _, r := range results {
		chunks = append(chunks, llm.ContextChunk{
			Content:    r.Content,
			SourceType: r.SourceType,
			SourcePath: r.SourcePath,
			Score:      r.Score,
			Tokens:     r.TokenCount,
		})
	}

	// Reserve a little budget for headers/formatting.
	usableBudget := contextBudget
	if usableBudget > 200 {
		usableBudget -= 200
	}

	selected := cm.SelectChunks(chunks, usableBudget)
	if len(selected) == 0 {
		return nil
	}

	ctx := cm.BuildContextPrompt(selected)
	ctx = strings.TrimSpace(ctx)
	if ctx == "" {
		return nil
	}

	content := "참고 컨텍스트(검색 결과):\n" + ctx
	content = truncateToTokens(tokenizer, content, contextBudget, false)
	m := llm.NewAssistantMessage(content)
	return &m
}

func needsHistoryCompression(tokenizer llm.TokenCounter, history []llm.ChatMessage, currentUser string, historyBudget int) bool {
	if historyBudget <= 0 {
		return false
	}

	// Approximate: history + current user are both part of "history" envelope.
	used := tokenizer.Count(currentUser)
	for _, m := range history {
		used += tokenizer.Count(m.Content)
	}
	return used > historyBudget
}

func truncateHistoryPreservingLastUser(
	tokenizer llm.TokenCounter,
	history []llm.ChatMessage,
	currentUser Message,
	historyBudget int,
) ([]llm.ChatMessage, error) {
	if historyBudget <= 0 {
		return nil, nil
	}

	userTokens := tokenizer.Count(currentUser.Content)
	if userTokens > historyBudget {
		return nil, errUserMessageTooLarge
	}

	remainingBudget := historyBudget - userTokens

	// Keep most recent messages that fit (chronological order).
	used := 0
	kept := make([]llm.ChatMessage, 0, len(history))
	for i := len(history) - 1; i >= 0; i-- {
		t := tokenizer.Count(history[i].Content)
		if used+t > remainingBudget {
			break
		}
		kept = append([]llm.ChatMessage{history[i]}, kept...)
		used += t
	}

	return kept, nil
}

func truncateTUIMessagesToBudget(tokenizer llm.TokenCounter, msgs []Message, budget int) []Message {
	if budget <= 0 || len(msgs) == 0 {
		return msgs
	}
	used := 0
	kept := make([]Message, 0, len(msgs))
	for i := len(msgs) - 1; i >= 0; i-- {
		t := tokenizer.Count(msgs[i].Content)
		if used+t > budget {
			break
		}
		kept = append([]Message{msgs[i]}, kept...)
		used += t
	}
	return kept
}

func convertTUIMessagesToLLM(msgs []Message) []llm.ChatMessage {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]llm.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleUser:
			out = append(out, llm.NewUserMessage(m.Content))
		case llm.RoleAssistant:
			out = append(out, llm.NewAssistantMessage(m.Content))
		case llm.RoleTool:
			// Tool messages are not currently stored in DB/TUI.
			out = append(out, llm.NewChatMessage(m.Role, m.Content))
		default:
			out = append(out, llm.NewChatMessage(m.Role, m.Content))
		}
	}
	return out
}

type tokenEstimateCounter struct{}

func (tokenEstimateCounter) Count(text string) int {
	return token.EstimateTokens(text)
}

func truncateToTokens(tokenizer llm.TokenCounter, text string, maxTokens int, fromEnd bool) string {
	if maxTokens <= 0 || text == "" {
		return text
	}

	// If we have a real token counter implementation, prefer its token-aware truncation.
	if c, ok := tokenizer.(*token.Counter); ok {
		return c.TruncateToFit(text, maxTokens, fromEnd)
	}

	// Fallback heuristic truncation by characters.
	if token.EstimateTokens(text) <= maxTokens {
		return text
	}
	maxChars := maxTokens * 4
	if maxChars <= 0 {
		return ""
	}
	if len(text) <= maxChars {
		return text
	}
	if fromEnd {
		return text[len(text)-maxChars:]
	}
	return text[:maxChars]
}

var (
	mdHeadingRE = regexp.MustCompile(`(?m)^#+\s+`)
	mdBulletRE  = regexp.MustCompile(`(?m)^\s*[-*]\s+`)
	mdBoldRE    = regexp.MustCompile(`\*\*(.*?)\*\*`)
	mdEmRE      = regexp.MustCompile(`\*(.*?)\*`)
)

func buildCanonicalFactsKorean(proj *project.Project) string {
	if proj == nil {
		return ""
	}

	characters, _ := proj.LoadCharacters()
	settings, _ := proj.LoadSettings()

	var lines []string

	if len(characters) > 0 {
		lines = append(lines, "## 정설(캐릭터)")
		for _, c := range characters {
			facts := extractTopFacts(c.Description, 2)
			role := detectRoleKorean(c.Description)
			parts := make([]string, 0, 3)
			parts = append(parts, facts...)
			if role != "" {
				parts = append(parts, role)
			} else {
				parts = append(parts, "역할: 미정")
			}
			line := fmt.Sprintf("- %s: %s", c.Name, strings.Join(parts, ", "))
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}

	if len(settings) > 0 {
		lines = append(lines, "## 정설(배경)")
		for _, s := range settings {
			facts := extractTopFacts(s.Description, 2)
			if len(facts) == 0 {
				facts = []string{"핵심특성: 미정"}
			}
			line := fmt.Sprintf("- %s: %s", s.Name, strings.Join(facts, ", "))
			lines = append(lines, line)
		}
		lines = append(lines, "")
	}

	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "\n")
}

func extractTopFacts(markdown string, max int) []string {
	if max <= 0 {
		return nil
	}
	text := strings.TrimSpace(markdown)
	if text == "" {
		return nil
	}

	// Strip headings to avoid returning file title as a "fact".
	text = mdHeadingRE.ReplaceAllString(text, "")
	text = mdBoldRE.ReplaceAllString(text, "$1")
	text = mdEmRE.ReplaceAllString(text, "$1")

	lines := strings.Split(text, "\n")

	// Prefer bullet points.
	var facts []string
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if mdBulletRE.MatchString(ln) {
			clean := mdBulletRE.ReplaceAllString(ln, "")
			clean = strings.TrimSpace(clean)
			clean = strings.TrimSuffix(clean, ".")
			if clean != "" {
				facts = append(facts, clean)
				if len(facts) >= max {
					return facts
				}
			}
		}
	}

	// Fallback: first non-empty paragraph line.
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		ln = strings.TrimSuffix(ln, ".")
		// Keep it short.
		if len([]rune(ln)) > 80 {
			r := []rune(ln)
			ln = string(r[:80])
		}
		return []string{ln}
	}

	return nil
}

func detectRoleKorean(text string) string {
	// Only return roles explicitly mentioned.
	lower := strings.ToLower(text)
	roles := []struct {
		needle string
		label  string
	}{
		{"주인공", "주인공"},
		{"프로타", "주인공"},
		{"악역", "악역"},
		{"안타", "악역"},
		{"조력", "조력자"},
		{"멘토", "멘토"},
		{"연인", "연인"},
		{"라이벌", "라이벌"},
	}
	for _, r := range roles {
		if strings.Contains(lower, r.needle) {
			return "역할: " + r.label
		}
	}
	return ""
}
