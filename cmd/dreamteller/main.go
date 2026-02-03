// Package main is the entry point for dreamteller.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/azyu/dreamteller/internal/app"
	"github.com/azyu/dreamteller/internal/llm"
	"github.com/azyu/dreamteller/internal/llm/adapters"
	"github.com/azyu/dreamteller/internal/project"
	"github.com/azyu/dreamteller/internal/search"
	"github.com/azyu/dreamteller/internal/token"
	"github.com/azyu/dreamteller/internal/tui"
	"github.com/azyu/dreamteller/pkg/types"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

var version = "0.1.0"

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "dreamteller",
	Short: "A TUI application for writing novels with AI assistance",
	Long: `Dreamteller is a terminal-based application that helps you write novels
with AI assistance. It provides context-aware suggestions based on your
characters, settings, and plot points.`,
	Version: version,
}

var newCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Create a new novel project",
	Args:  cobra.ExactArgs(1),
	RunE:  runNewCmd,
}

func runNewCmd(cmd *cobra.Command, args []string) error {
	name := args[0]
	fromPrompt, _ := cmd.Flags().GetString("from-prompt")
	genre, _ := cmd.Flags().GetString("genre")

	application, err := app.New()
	if err != nil {
		return fmt.Errorf("failed to initialize app: %w", err)
	}
	defer application.Close()

	if application.ProjectManager.Exists(name) {
		return fmt.Errorf("project '%s' already exists", name)
	}

	// Handle --from-prompt flag
	if fromPrompt != "" {
		promptContent, err := readPromptFile(fromPrompt)
		if err != nil {
			return fmt.Errorf("failed to read prompt file: %w", err)
		}
		return createProjectFromPrompt(application, name, promptContent)
	}

	// Handle --genre flag for quick creation
	if genre != "" {
		if err := application.CreateProject(name, genre); err != nil {
			return fmt.Errorf("failed to create project: %w", err)
		}
		fmt.Printf("Created project '%s' with genre '%s' at %s\n", name, genre, application.CurrentProject.Path())
		return nil
	}

	// Interactive mode: show setup mode selection
	return runInteractiveSetup(application, name)
}

// SetupMode represents the selected setup mode.
type SetupMode string

const (
	SetupModeWizard   SetupMode = "wizard"
	SetupModePrompt   SetupMode = "prompt"
	SetupModeTemplate SetupMode = "template"
)

type Language string

const (
	LangEnglish  Language = "en"
	LangKorean   Language = "ko"
	LangJapanese Language = "ja"
)

type i18nStrings struct {
	SetupTitle       string
	SetupWizard      string
	SetupPrompt      string
	SetupTemplate    string
	SelectGenre      string
	WritingStyle     string
	StylePlaceholder string
	PointOfView      string
	Tense            string
	Genres           map[string]string
	POVs             map[string]string
	Tenses           map[string]string
	CreatedProject   string
	RunToStart       string
}

var translations = map[Language]i18nStrings{
	LangEnglish: {
		SetupTitle:       "How would you like to set up your project?",
		SetupWizard:      "Wizard - Guided step-by-step setup",
		SetupPrompt:      "Prompt - Describe your story and auto-create",
		SetupTemplate:    "Template - Start from a preset (coming soon)",
		SelectGenre:      "Select your genre",
		WritingStyle:     "Describe your writing style",
		StylePlaceholder: "e.g., descriptive, immersive, fast-paced",
		PointOfView:      "Point of View",
		Tense:            "Tense",
		Genres: map[string]string{
			"fantasy":    "Fantasy",
			"scifi":      "Science Fiction",
			"mystery":    "Mystery",
			"romance":    "Romance",
			"thriller":   "Thriller",
			"horror":     "Horror",
			"historical": "Historical Fiction",
			"literary":   "Literary Fiction",
			"other":      "Other",
		},
		POVs: map[string]string{
			"first-person":            "First Person",
			"third-person-limited":    "Third Person Limited",
			"third-person-omniscient": "Third Person Omniscient",
			"second-person":           "Second Person",
		},
		Tenses: map[string]string{
			"past":    "Past Tense",
			"present": "Present Tense",
		},
		CreatedProject: "Created project '%s' at %s",
		RunToStart:     "Run 'dreamteller open %s' to start writing!",
	},
	LangKorean: {
		SetupTitle:       "프로젝트를 어떻게 설정하시겠습니까?",
		SetupWizard:      "마법사 - 단계별 안내 설정",
		SetupPrompt:      "프롬프트 - 스토리 설명으로 자동 생성",
		SetupTemplate:    "템플릿 - 프리셋으로 시작 (준비 중)",
		SelectGenre:      "장르를 선택하세요",
		WritingStyle:     "작문 스타일을 설명하세요",
		StylePlaceholder: "예: 묘사적, 몰입감 있는, 빠른 전개",
		PointOfView:      "시점",
		Tense:            "시제",
		Genres: map[string]string{
			"fantasy":    "판타지",
			"scifi":      "SF (과학 소설)",
			"mystery":    "미스터리",
			"romance":    "로맨스",
			"thriller":   "스릴러",
			"horror":     "호러",
			"historical": "역사 소설",
			"literary":   "순수 문학",
			"other":      "기타",
		},
		POVs: map[string]string{
			"first-person":            "1인칭",
			"third-person-limited":    "3인칭 제한",
			"third-person-omniscient": "3인칭 전지",
			"second-person":           "2인칭",
		},
		Tenses: map[string]string{
			"past":    "과거 시제",
			"present": "현재 시제",
		},
		CreatedProject: "'%s' 프로젝트가 %s에 생성되었습니다",
		RunToStart:     "'dreamteller open %s' 명령으로 시작하세요!",
	},
	LangJapanese: {
		SetupTitle:       "プロジェクトの設定方法を選んでください",
		SetupWizard:      "ウィザード - ステップバイステップのガイド設定",
		SetupPrompt:      "プロンプト - ストーリーを説明して自動作成",
		SetupTemplate:    "テンプレート - プリセットから開始（準備中）",
		SelectGenre:      "ジャンルを選択してください",
		WritingStyle:     "文体を説明してください",
		StylePlaceholder: "例：描写的、没入感のある、テンポが速い",
		PointOfView:      "視点",
		Tense:            "時制",
		Genres: map[string]string{
			"fantasy":    "ファンタジー",
			"scifi":      "SF（サイエンスフィクション）",
			"mystery":    "ミステリー",
			"romance":    "ロマンス",
			"thriller":   "スリラー",
			"horror":     "ホラー",
			"historical": "歴史小説",
			"literary":   "純文学",
			"other":      "その他",
		},
		POVs: map[string]string{
			"first-person":            "一人称",
			"third-person-limited":    "三人称限定",
			"third-person-omniscient": "三人称全知",
			"second-person":           "二人称",
		},
		Tenses: map[string]string{
			"past":    "過去形",
			"present": "現在形",
		},
		CreatedProject: "プロジェクト '%s' を %s に作成しました",
		RunToStart:     "'dreamteller open %s' で開始してください！",
	},
}

// runInteractiveSetup shows the setup mode selection UI.
func runInteractiveSetup(application *app.App, name string) error {
	var lang Language

	langForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[Language]().
				Title("Select your language / 언어 선택 / 言語を選択").
				Options(
					huh.NewOption("English", LangEnglish),
					huh.NewOption("한국어", LangKorean),
					huh.NewOption("日本語", LangJapanese),
				).
				Value(&lang),
		),
	)

	if err := langForm.Run(); err != nil {
		return fmt.Errorf("language selection failed: %w", err)
	}

	t := translations[lang]
	var mode SetupMode

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[SetupMode]().
				Title(t.SetupTitle).
				Options(
					huh.NewOption(t.SetupWizard, SetupModeWizard),
					huh.NewOption(t.SetupPrompt, SetupModePrompt),
					huh.NewOption(t.SetupTemplate, SetupModeTemplate),
				).
				Value(&mode),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("setup mode selection failed: %w", err)
	}

	switch mode {
	case SetupModeWizard:
		return runWizardSetup(application, name, lang)
	case SetupModePrompt:
		return runPromptSetup(application, name)
	case SetupModeTemplate:
		fmt.Println("Template mode is coming soon!")
		fmt.Println("Please use Wizard or Prompt mode for now.")
		return nil
	default:
		return fmt.Errorf("unknown setup mode: %s", mode)
	}
}

// runWizardSetup runs the guided step-by-step wizard.
func runWizardSetup(application *app.App, name string, lang Language) error {
	t := translations[lang]
	var genre string
	var writingStyle string
	var pov string
	var tense string

	genreKeys := []string{"fantasy", "scifi", "mystery", "romance", "thriller", "horror", "historical", "literary", "other"}
	genres := make([]huh.Option[string], len(genreKeys))
	for i, key := range genreKeys {
		genres[i] = huh.NewOption(t.Genres[key], key)
	}

	povKeys := []string{"first-person", "third-person-limited", "third-person-omniscient", "second-person"}
	povOptions := make([]huh.Option[string], len(povKeys))
	for i, key := range povKeys {
		povOptions[i] = huh.NewOption(t.POVs[key], key)
	}

	tenseKeys := []string{"past", "present"}
	tenseOptions := make([]huh.Option[string], len(tenseKeys))
	for i, key := range tenseKeys {
		tenseOptions[i] = huh.NewOption(t.Tenses[key], key)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(t.SelectGenre).
				Options(genres...).
				Value(&genre),
		),
		huh.NewGroup(
			huh.NewInput().
				Title(t.WritingStyle).
				Placeholder(t.StylePlaceholder).
				Value(&writingStyle),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(t.PointOfView).
				Options(povOptions...).
				Value(&pov),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(t.Tense).
				Options(tenseOptions...).
				Value(&tense),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("wizard setup failed: %w", err)
	}

	config := types.DefaultProjectConfig(name, genre)
	config.Writing.Style = writingStyle
	config.Writing.POV = pov
	config.Writing.Tense = tense

	proj, err := application.ProjectManager.Create(name, config)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}
	application.CurrentProject = proj

	fmt.Printf("\n"+t.CreatedProject+"\n", name, proj.Path())
	fmt.Printf("Genre: %s\n", t.Genres[genre])
	fmt.Printf("Style: %s\n", writingStyle)
	fmt.Printf("POV: %s, Tense: %s\n", t.POVs[pov], t.Tenses[tense])
	fmt.Printf("\n"+t.RunToStart+"\n", name)

	return nil
}

// runPromptSetup runs the prompt-based setup.
func runPromptSetup(application *app.App, name string) error {
	var prompt string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Title("Describe your story").
				Description("Include details about genre, setting, characters, and plot ideas.").
				Placeholder("Write a detailed description of your novel idea...").
				CharLimit(4000).
				Value(&prompt),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("prompt setup failed: %w", err)
	}

	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt cannot be empty")
	}

	return createProjectFromPrompt(application, name, prompt)
}

// readPromptFile reads prompt content from a file or stdin.
func readPromptFile(path string) (string, error) {
	if path == "-" {
		return readFromStdin()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", path, err)
	}

	return strings.TrimSpace(string(data)), nil
}

// readFromStdin reads all content from stdin.
func readFromStdin() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	var builder strings.Builder

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				builder.WriteString(line)
				break
			}
			return "", fmt.Errorf("error reading stdin: %w", err)
		}
		builder.WriteString(line)
	}

	return strings.TrimSpace(builder.String()), nil
}

var errNoProvider = fmt.Errorf("no LLM provider configured")

func checkLLMProvider(application *app.App) (*types.ProviderConfig, string, error) {
	globalConfig, err := application.Config.LoadGlobalConfig()
	if err != nil {
		return nil, "", fmt.Errorf("failed to load config: %w", err)
	}

	providerName := globalConfig.Defaults.Provider
	if providerName == "" {
		providerName = "openai"
	}

	providerConfig, err := application.Config.GetProviderConfig(providerName)
	if err != nil {
		fmt.Println("\n⚠ No LLM provider configured.")
		fmt.Println("Run 'dreamteller auth' to set up a provider.")
		return nil, "", errNoProvider
	}

	if providerName != "local" && providerConfig.APIKey == "" {
		fmt.Printf("\n⚠ No API key configured for %s.\n", providerName)
		fmt.Println("Run 'dreamteller auth' to set up a provider.")
		return nil, "", errNoProvider
	}

	return providerConfig, providerName, nil
}

func createProjectFromPrompt(application *app.App, name, promptContent string) error {
	fmt.Println("Analyzing your story description...")

	providerConfig, providerName, err := checkLLMProvider(application)
	if err != nil {
		return err
	}

	ctx := context.Background()
	provider, err := initLLMProvider(ctx, providerName, providerConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize LLM provider: %w", err)
	}
	defer provider.Close()

	parseResult, err := parsePromptWithAI(ctx, provider, promptContent)
	if err != nil {
		return fmt.Errorf("failed to parse prompt: %w", err)
	}

	fmt.Println("Creating project structure...")

	// Create project config from parsed result
	config := types.DefaultProjectConfig(name, parseResult.Genre)
	if parseResult.StyleGuide.Tone != "" {
		config.Writing.Style = parseResult.StyleGuide.Tone
	}

	// Create the project
	proj, err := application.ProjectManager.Create(name, config)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}
	application.CurrentProject = proj

	// Generate initial context files
	if err := generateInitialContext(proj, parseResult); err != nil {
		fmt.Printf("Warning: failed to generate some context files: %v\n", err)
	}

	fmt.Printf("\nCreated project '%s' at %s\n", name, proj.Path())
	fmt.Printf("Genre: %s\n", parseResult.Genre)

	if len(parseResult.Characters) > 0 {
		fmt.Printf("Characters: %d created\n", len(parseResult.Characters))
	}
	if parseResult.Setting.Location != "" {
		fmt.Println("Setting: created")
	}
	if len(parseResult.PlotHints) > 0 {
		fmt.Printf("Plot hints: %d created\n", len(parseResult.PlotHints))
	}

	fmt.Println("\nRun 'dreamteller open " + name + "' to start writing!")

	return nil
}

// initLLMProvider initializes the appropriate LLM provider.
func initLLMProvider(ctx context.Context, providerName string, config *types.ProviderConfig) (llm.Provider, error) {
	switch providerName {
	case "openai":
		model := config.DefaultModel
		if model == "" {
			model = "gpt-4o"
		}
		var opts []adapters.OpenAIOption
		if config.BaseURL != "" {
			opts = append(opts, adapters.WithOpenAIBaseURL(config.BaseURL))
		}
		return adapters.NewOpenAIAdapter(config.APIKey, model, opts...)

	case "gemini":
		model := config.DefaultModel
		if model == "" {
			model = "gemini-2.5-flash"
		}
		return adapters.NewGeminiAdapter(ctx, config.APIKey, model)

	case "local":
		baseURL := config.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:11434"
		}
		model := config.DefaultModel
		if model == "" {
			model = "llama3"
		}
		return adapters.NewLocalAdapter(baseURL, model), nil

	default:
		return nil, fmt.Errorf("unsupported provider: %s", providerName)
	}
}

// parsePromptWithAI uses the LLM to parse the story prompt and extract structured data.
func parsePromptWithAI(ctx context.Context, provider llm.Provider, promptContent string) (*types.ParsePromptResult, error) {
	systemPrompt := `You are a creative writing assistant. Analyze the user's story description and extract structured information.

Return a JSON object with the following structure:
{
  "genre": "the primary genre (fantasy, scifi, mystery, romance, thriller, horror, historical, literary)",
  "setting": {
    "time_period": "when the story takes place",
    "location": "where the story takes place",
    "description": "detailed setting description"
  },
  "characters": [
    {
      "name": "character name",
      "role": "protagonist/antagonist/supporting",
      "description": "character description",
      "traits": {"key": "value"}
    }
  ],
  "plot_hints": ["key plot point or theme"],
  "style_guide": {
    "tone": "the overall tone (dark, humorous, romantic, etc.)",
    "pacing": "slow/medium/fast",
    "dialogue": "description of dialogue style",
    "vocabulary_notes": ["notes about word choices or style"]
  }
}

Be creative in filling in details based on the user's description. If something isn't mentioned, make reasonable inferences based on the genre and context.`

	messages := []llm.ChatMessage{
		llm.NewSystemMessage(systemPrompt),
		llm.NewUserMessage(promptContent),
	}

	resp, err := provider.Chat(ctx, llm.ChatRequest{
		Messages:    messages,
		MaxTokens:   2000,
		Temperature: 0.7,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	// Parse the JSON response
	content := strings.TrimSpace(resp.Message.Content)

	// Try to extract JSON from the response (in case it's wrapped in markdown)
	content = extractJSON(content)

	var result types.ParsePromptResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response as JSON: %w\nResponse: %s", err, content)
	}

	// Default genre if not detected
	if result.Genre == "" {
		result.Genre = "literary"
	}

	return &result, nil
}

// extractJSON attempts to extract JSON from a response that might be wrapped in markdown.
func extractJSON(content string) string {
	// Try to find JSON block in markdown
	if idx := strings.Index(content, "```json"); idx != -1 {
		content = content[idx+7:]
		if endIdx := strings.Index(content, "```"); endIdx != -1 {
			content = content[:endIdx]
		}
	} else if idx := strings.Index(content, "```"); idx != -1 {
		content = content[idx+3:]
		if endIdx := strings.Index(content, "```"); endIdx != -1 {
			content = content[:endIdx]
		}
	}

	// Try to find JSON object boundaries
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start != -1 && end != -1 && end > start {
		content = content[start : end+1]
	}

	return strings.TrimSpace(content)
}

// generateInitialContext creates initial context files from parsed data.
func generateInitialContext(proj *project.Project, result *types.ParsePromptResult) error {
	var errs []string

	// Create setting file
	if result.Setting.Location != "" || result.Setting.Description != "" {
		settingContent := fmt.Sprintf("# %s\n\n", result.Setting.Location)
		if result.Setting.TimePeriod != "" {
			settingContent += fmt.Sprintf("**Time Period:** %s\n\n", result.Setting.TimePeriod)
		}
		settingContent += result.Setting.Description

		if err := proj.CreateContextFile("settings", "main-setting", settingContent); err != nil {
			errs = append(errs, fmt.Sprintf("setting: %v", err))
		}
	}

	// Create character files
	for _, char := range result.Characters {
		content := fmt.Sprintf("# %s\n\n", char.Name)
		content += fmt.Sprintf("**Role:** %s\n\n", char.Role)
		content += fmt.Sprintf("## Description\n\n%s\n", char.Description)

		if len(char.Traits) > 0 {
			content += "\n## Traits\n\n"
			for k, v := range char.Traits {
				content += fmt.Sprintf("- **%s:** %s\n", k, v)
			}
		}

		filename := sanitizeFilename(char.Name)
		if err := proj.CreateContextFile("characters", filename, content); err != nil {
			errs = append(errs, fmt.Sprintf("character %s: %v", char.Name, err))
		}
	}

	// Create plot file
	if len(result.PlotHints) > 0 {
		content := "# Plot Overview\n\n"
		for i, hint := range result.PlotHints {
			content += fmt.Sprintf("%d. %s\n", i+1, hint)
		}

		if err := proj.CreateContextFile("plot", "overview", content); err != nil {
			errs = append(errs, fmt.Sprintf("plot: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

// sanitizeFilename converts a name to a safe filename.
func sanitizeFilename(name string) string {
	// Convert to lowercase and replace spaces with hyphens
	result := strings.ToLower(name)
	result = strings.ReplaceAll(result, " ", "-")

	// Remove any characters that aren't alphanumeric, hyphens, or underscores
	var sanitized strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			sanitized.WriteRune(r)
		}
	}

	return sanitized.String()
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all novel projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return fmt.Errorf("failed to initialize app: %w", err)
		}

		projects, err := application.ListProjects()
		if err != nil {
			return fmt.Errorf("failed to list projects: %w", err)
		}

		if len(projects) == 0 {
			fmt.Println("No projects found. Create one with: dreamteller new <name>")
			return nil
		}

		fmt.Println("Projects:")
		for _, p := range projects {
			fmt.Printf("  - %s (%s) - %s\n", p.Name, p.Genre, p.Path)
		}
		return nil
	},
}

var openCmd = &cobra.Command{
	Use:   "open <name>",
	Short: "Open a novel project in TUI mode",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		application, err := app.New()
		if err != nil {
			return fmt.Errorf("failed to initialize app: %w", err)
		}
		defer application.Close()

		if err := application.OpenProject(name); err != nil {
			return fmt.Errorf("failed to open project: %w", err)
		}

		return runTUI(application.CurrentProject)
	},
}

var reindexCmd = &cobra.Command{
	Use:   "reindex [name]",
	Short: "Rebuild the search index for a project",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		application, err := app.New()
		if err != nil {
			return fmt.Errorf("failed to initialize app: %w", err)
		}
		defer application.Close()

		var name string
		if len(args) > 0 {
			name = args[0]
		} else {
			// Try to detect project from current directory
			return fmt.Errorf("please specify a project name")
		}

		if err := application.OpenProject(name); err != nil {
			return fmt.Errorf("failed to open project: %w", err)
		}

		proj := application.CurrentProject
		fmt.Printf("Reindexing project '%s'...\n", name)

		// Initialize the search engine and indexer
		ftsEngine := search.NewFTSEngine(proj.DB)

		// Initialize token counter for chunking
		counter, err := token.NewCounter("cl100k_base")
		if err != nil {
			return fmt.Errorf("failed to initialize token counter: %w", err)
		}

		indexer := search.NewIndexer(
			ftsEngine,
			counter,
			proj.Config.Context.ChunkSize,
			proj.Config.Context.ChunkOverlap,
		)

		// Perform full reindex
		if err := indexer.FullReindexWithDB(proj.FS, proj.DB); err != nil {
			return fmt.Errorf("reindex failed: %w", err)
		}

		// Get stats
		count, err := ftsEngine.GetChunkCount()
		if err != nil {
			fmt.Println("Reindex complete.")
			return nil
		}

		fmt.Printf("Reindex complete. Indexed %d chunks.\n", count)
		return nil
	},
}

var exportCmd = &cobra.Command{
	Use:   "export <name> <format>",
	Short: "Export a novel to a specific format",
	Long:  "Export a novel to epub, pdf, or txt format.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		format := args[1]

		switch format {
		case "epub", "pdf", "txt":
			// TODO: Implement export
			fmt.Printf("Exporting '%s' to %s format...\n", name, format)
			return fmt.Errorf("export not yet implemented")
		default:
			return fmt.Errorf("unsupported format: %s (use epub, pdf, or txt)", format)
		}
	},
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Edit global configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		// TODO: Open config in editor or show interactive config
		fmt.Println("Configuration editor not yet implemented.")
		fmt.Println("Edit ~/.config/dreamteller/config.yaml manually.")
		return nil
	},
}

var deleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a novel project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		force, _ := cmd.Flags().GetBool("force")

		application, err := app.New()
		if err != nil {
			return fmt.Errorf("failed to initialize app: %w", err)
		}

		if !application.ProjectManager.Exists(name) {
			return fmt.Errorf("project '%s' not found", name)
		}

		if !force {
			var confirm string
			fmt.Printf("This will permanently delete project '%s' and all its files.\n", name)
			fmt.Printf("Type the project name to confirm: ")
			fmt.Scanln(&confirm)

			if confirm != name {
				fmt.Println("Deletion cancelled.")
				return nil
			}
		}

		if err := application.ProjectManager.Delete(name); err != nil {
			return fmt.Errorf("failed to delete project: %w", err)
		}

		fmt.Printf("Project '%s' deleted.\n", name)
		return nil
	},
}

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Configure LLM provider authentication",
	RunE:  runAuthCmd,
}

func runAuthCmd(cmd *cobra.Command, args []string) error {
	listFlag, _ := cmd.Flags().GetBool("list")
	removeFlag, _ := cmd.Flags().GetString("remove")
	providerFlag, _ := cmd.Flags().GetString("provider")

	application, err := app.New()
	if err != nil {
		return fmt.Errorf("failed to initialize app: %w", err)
	}

	if listFlag {
		return listProviders(application)
	}

	if removeFlag != "" {
		return removeProvider(application, removeFlag)
	}

	if providerFlag != "" {
		return configureProvider(application, providerFlag)
	}

	return interactiveAuth(application)
}

func listProviders(application *app.App) error {
	config, err := application.Config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Println("Configured providers:")
	fmt.Println()

	providers := []struct {
		name  string
		label string
	}{
		{"openai", "OpenAI"},
		{"gemini", "Google Gemini"},
		{"local", "Local (Ollama/LM Studio)"},
	}

	hasAny := false
	for _, p := range providers {
		providerConfig, exists := config.Providers[p.name]
		if !exists || (providerConfig.APIKey == "" && providerConfig.BaseURL == "") {
			continue
		}

		hasAny = true
		isDefault := config.Defaults.Provider == p.name
		defaultMark := ""
		if isDefault {
			defaultMark = " (default)"
		}

		fmt.Printf("  %s%s\n", p.label, defaultMark)

		if providerConfig.APIKey != "" {
			masked := maskAPIKey(providerConfig.APIKey)
			fmt.Printf("    API Key: %s\n", masked)
		}
		if providerConfig.DefaultModel != "" {
			fmt.Printf("    Model: %s\n", providerConfig.DefaultModel)
		}
		if providerConfig.BaseURL != "" {
			fmt.Printf("    Base URL: %s\n", providerConfig.BaseURL)
		}
		fmt.Println()
	}

	if !hasAny {
		fmt.Println("  No providers configured.")
		fmt.Println()
		fmt.Println("Run 'dreamteller auth' to configure a provider.")
	}

	return nil
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

func removeProvider(application *app.App, providerName string) error {
	config, err := application.Config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if _, exists := config.Providers[providerName]; !exists {
		return fmt.Errorf("provider '%s' is not configured", providerName)
	}

	delete(config.Providers, providerName)

	if config.Defaults.Provider == providerName {
		config.Defaults.Provider = ""
		for name := range config.Providers {
			config.Defaults.Provider = name
			break
		}
	}

	if err := application.Config.SaveGlobalConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Provider '%s' removed.\n", providerName)
	return nil
}

func configureProvider(application *app.App, providerName string) error {
	switch providerName {
	case "openai", "gemini", "local":
		return setupProvider(application, providerName)
	default:
		return fmt.Errorf("unknown provider: %s (supported: openai, gemini, local)", providerName)
	}
}

func interactiveAuth(application *app.App) error {
	var providerName string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select provider to configure").
				Options(
					huh.NewOption("OpenAI", "openai"),
					huh.NewOption("Google Gemini", "gemini"),
					huh.NewOption("Local (Ollama/LM Studio)", "local"),
				).
				Value(&providerName),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("provider selection failed: %w", err)
	}

	return setupProvider(application, providerName)
}

func setupProvider(application *app.App, providerName string) error {
	config, err := application.Config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if config.Providers == nil {
		config.Providers = make(map[string]*types.ProviderConfig)
	}

	providerConfig := config.Providers[providerName]
	if providerConfig == nil {
		providerConfig = &types.ProviderConfig{}
	}

	switch providerName {
	case "openai":
		if err := setupOpenAI(providerConfig); err != nil {
			return err
		}
	case "gemini":
		if err := setupGemini(providerConfig); err != nil {
			return err
		}
	case "local":
		if err := setupLocal(providerConfig); err != nil {
			return err
		}
	}

	config.Providers[providerName] = providerConfig

	var setDefault bool
	defaultForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Set as default provider?").
				Value(&setDefault),
		),
	)

	if err := defaultForm.Run(); err != nil {
		return fmt.Errorf("default selection failed: %w", err)
	}

	if setDefault {
		config.Defaults.Provider = providerName
	}

	if err := application.Config.SaveGlobalConfig(config); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\n✓ %s configured successfully\n", providerName)
	return nil
}

func setupOpenAI(config *types.ProviderConfig) error {
	var apiKey, model string

	currentKey := ""
	if config.APIKey != "" {
		currentKey = " (current: " + maskAPIKey(config.APIKey) + ")"
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("OpenAI API Key"+currentKey).
				Placeholder("sk-...").
				Value(&apiKey),
			huh.NewSelect[string]().
				Title("Default model").
				Options(
					huh.NewOption("GPT-4o (recommended)", "gpt-4o"),
					huh.NewOption("GPT-4o Mini", "gpt-4o-mini"),
					huh.NewOption("GPT-4 Turbo", "gpt-4-turbo"),
					huh.NewOption("GPT-4", "gpt-4"),
					huh.NewOption("GPT-3.5 Turbo", "gpt-3.5-turbo"),
				).
				Value(&model),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("OpenAI setup failed: %w", err)
	}

	if apiKey != "" {
		config.APIKey = apiKey
	}
	if model != "" {
		config.DefaultModel = model
	}

	return nil
}

func setupGemini(config *types.ProviderConfig) error {
	var apiKey, model string

	currentKey := ""
	if config.APIKey != "" {
		currentKey = " (current: " + maskAPIKey(config.APIKey) + ")"
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Gemini API Key"+currentKey).
				Placeholder("Get from ai.google.dev").
				Value(&apiKey),
			huh.NewSelect[string]().
				Title("Default model").
				Options(
					huh.NewOption("Gemini 2.5 Flash (recommended)", "gemini-2.5-flash"),
					huh.NewOption("Gemini 2.5 Pro", "gemini-2.5-pro"),
					huh.NewOption("Gemini 2.0 Flash", "gemini-2.0-flash"),
				).
				Value(&model),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("Gemini setup failed: %w", err)
	}

	if apiKey != "" {
		config.APIKey = apiKey
	}
	if model != "" {
		config.DefaultModel = model
	}

	return nil
}

func setupLocal(config *types.ProviderConfig) error {
	var baseURL string
	var protocol string

	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434"
	}
	if config.Protocol == "" {
		config.Protocol = "openai"
	}

	protocols := []huh.Option[string]{
		huh.NewOption("OpenAI Compatible", "openai"),
		huh.NewOption("Anthropic Compatible", "anthropic"),
		huh.NewOption("Gemini Compatible", "gemini"),
		huh.NewOption("Ollama", "ollama"),
	}

	setupForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Protocol").
				Options(protocols...).
				Value(&protocol),
			huh.NewInput().
				Title("Base URL").
				Placeholder("http://localhost:11434").
				Value(&baseURL),
		),
	)

	if err := setupForm.Run(); err != nil {
		return fmt.Errorf("Local setup failed: %w", err)
	}

	if protocol != "" {
		config.Protocol = protocol
	}
	if baseURL != "" {
		config.BaseURL = baseURL
	}

	models, err := fetchLocalModels(config.BaseURL, config.Protocol)
	if err != nil {
		fmt.Printf("\n⚠ Could not fetch models from %s: %v\n", config.BaseURL, err)
		fmt.Println("Please enter model name manually.")

		var model string
		manualForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Model name").
					Placeholder("llama3, mistral, etc.").
					Validate(func(s string) error {
						if strings.TrimSpace(s) == "" {
							return fmt.Errorf("model name is required")
						}
						return nil
					}).
					Value(&model),
			),
		)
		if err := manualForm.Run(); err != nil {
			return fmt.Errorf("model input failed: %w", err)
		}
		config.DefaultModel = model
		return nil
	}

	if len(models) == 0 {
		fmt.Println("\n⚠ No models found. Please pull a model first:")
		fmt.Println("  ollama pull llama3.2")
		return fmt.Errorf("no models available")
	}

	var selectedModel string
	options := make([]huh.Option[string], len(models))
	for i, m := range models {
		options[i] = huh.NewOption(m.Display, m.Name)
	}

	modelForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select model").
				Options(options...).
				Value(&selectedModel),
		),
	)

	if err := modelForm.Run(); err != nil {
		return fmt.Errorf("model selection failed: %w", err)
	}

	config.DefaultModel = selectedModel
	return nil
}

type modelInfo struct {
	Name    string
	Display string
}

func fetchLocalModels(baseURL, protocol string) ([]modelInfo, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")
	client := &http.Client{Timeout: 5 * time.Second}

	endpointMap := map[string]struct {
		path  string
		parse func([]byte) ([]modelInfo, error)
	}{
		"ollama":    {"/api/tags", parseOllamaModels},
		"openai":    {"/v1/models", parseOpenAIModels},
		"anthropic": {"/v1/models", parseOpenAIModels},
		"gemini":    {"/v1beta/models", parseGeminiModels},
	}

	ep, ok := endpointMap[protocol]
	if !ok {
		return nil, fmt.Errorf("unknown protocol: %s", protocol)
	}

	resp, err := client.Get(baseURL + ep.path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned %d", ep.path, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return ep.parse(body)
}

func parseOllamaModels(body []byte) ([]modelInfo, error) {
	var result struct {
		Models []struct {
			Name    string `json:"name"`
			Details struct {
				ParameterSize     string `json:"parameter_size"`
				QuantizationLevel string `json:"quantization_level"`
			} `json:"details"`
		} `json:"models"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	models := make([]modelInfo, len(result.Models))
	for i, m := range result.Models {
		display := m.Name
		if m.Details.ParameterSize != "" {
			display = fmt.Sprintf("%s (%s", m.Name, m.Details.ParameterSize)
			if m.Details.QuantizationLevel != "" {
				display += ", " + m.Details.QuantizationLevel
			}
			display += ")"
		}
		models[i] = modelInfo{Name: m.Name, Display: display}
	}
	return models, nil
}

func parseOpenAIModels(body []byte) ([]modelInfo, error) {
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	models := make([]modelInfo, len(result.Data))
	for i, m := range result.Data {
		models[i] = modelInfo{Name: m.ID, Display: m.ID}
	}
	return models, nil
}

func parseGeminiModels(body []byte) ([]modelInfo, error) {
	var result struct {
		Models []struct {
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
		} `json:"models"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	models := make([]modelInfo, len(result.Models))
	for i, m := range result.Models {
		name := strings.TrimPrefix(m.Name, "models/")
		display := m.DisplayName
		if display == "" {
			display = name
		}
		models[i] = modelInfo{Name: name, Display: display}
	}
	return models, nil
}

func init() {
	newCmd.Flags().String("from-prompt", "", "Path to prompt file for one-shot setup (use '-' for stdin)")
	newCmd.Flags().String("genre", "", "Genre for quick project creation without wizard")

	deleteCmd.Flags().BoolP("force", "f", false, "Delete without confirmation")

	authCmd.Flags().BoolP("list", "l", false, "List configured providers")
	authCmd.Flags().StringP("remove", "r", "", "Remove a provider configuration")
	authCmd.Flags().StringP("provider", "p", "", "Configure a specific provider")

	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(reindexCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(authCmd)
}

func runTUI(proj *project.Project) error {
	searchEngine := search.NewFTSEngine(proj.DB)

	application, err := app.New()
	if err != nil {
		return fmt.Errorf("failed to initialize app: %w", err)
	}

	providerConfig, providerName, err := checkLLMProvider(application)
	if err != nil {
		return err
	}

	ctx := context.Background()
	provider, err := initLLMProvider(ctx, providerName, providerConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize LLM provider: %w", err)
	}
	defer provider.Close()

	modelName := providerConfig.DefaultModel
	if modelName == "" {
		modelName = providerName
	}

	baseURL := providerConfig.BaseURL
	if providerName == "local" && baseURL == "" {
		baseURL = "http://localhost:11434"
	}

	model := tui.New(proj, provider, searchEngine, modelName, providerName, baseURL)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
