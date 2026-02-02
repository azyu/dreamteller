// Package main is the entry point for dreamteller.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

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

// runInteractiveSetup shows the setup mode selection UI.
func runInteractiveSetup(application *app.App, name string) error {
	var mode SetupMode

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[SetupMode]().
				Title("How would you like to set up your project?").
				Options(
					huh.NewOption("Wizard - Guided step-by-step setup", SetupModeWizard),
					huh.NewOption("Prompt - Describe your story and auto-create", SetupModePrompt),
					huh.NewOption("Template - Start from a preset (coming soon)", SetupModeTemplate),
				).
				Value(&mode),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("setup mode selection failed: %w", err)
	}

	switch mode {
	case SetupModeWizard:
		return runWizardSetup(application, name)
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
func runWizardSetup(application *app.App, name string) error {
	var genre string
	var writingStyle string
	var pov string
	var tense string

	genres := []huh.Option[string]{
		huh.NewOption("Fantasy", "fantasy"),
		huh.NewOption("Science Fiction", "scifi"),
		huh.NewOption("Mystery", "mystery"),
		huh.NewOption("Romance", "romance"),
		huh.NewOption("Thriller", "thriller"),
		huh.NewOption("Horror", "horror"),
		huh.NewOption("Historical Fiction", "historical"),
		huh.NewOption("Literary Fiction", "literary"),
		huh.NewOption("Other", "other"),
	}

	povOptions := []huh.Option[string]{
		huh.NewOption("First Person", "first-person"),
		huh.NewOption("Third Person Limited", "third-person-limited"),
		huh.NewOption("Third Person Omniscient", "third-person-omniscient"),
		huh.NewOption("Second Person", "second-person"),
	}

	tenseOptions := []huh.Option[string]{
		huh.NewOption("Past Tense", "past"),
		huh.NewOption("Present Tense", "present"),
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select your genre").
				Options(genres...).
				Value(&genre),
		),
		huh.NewGroup(
			huh.NewInput().
				Title("Describe your writing style").
				Placeholder("e.g., descriptive, immersive, fast-paced").
				Value(&writingStyle),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Point of View").
				Options(povOptions...).
				Value(&pov),
		),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Tense").
				Options(tenseOptions...).
				Value(&tense),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("wizard setup failed: %w", err)
	}

	// Create project with wizard config
	config := types.DefaultProjectConfig(name, genre)
	config.Writing.Style = writingStyle
	config.Writing.POV = pov
	config.Writing.Tense = tense

	proj, err := application.ProjectManager.Create(name, config)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}
	application.CurrentProject = proj

	fmt.Printf("\nCreated project '%s' at %s\n", name, proj.Path())
	fmt.Printf("Genre: %s\n", genre)
	fmt.Printf("Style: %s\n", writingStyle)
	fmt.Printf("POV: %s, Tense: %s\n", pov, tense)
	fmt.Println("\nRun 'dreamteller open " + name + "' to start writing!")

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

// createProjectFromPrompt creates a project using AI to parse the prompt.
func createProjectFromPrompt(application *app.App, name, promptContent string) error {
	fmt.Println("Analyzing your story description...")

	// Load global config to get provider settings
	globalConfig, err := application.Config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get the default provider
	providerName := globalConfig.Defaults.Provider
	if providerName == "" {
		providerName = "openai"
	}

	providerConfig, err := application.Config.GetProviderConfig(providerName)
	if err != nil {
		return fmt.Errorf("failed to get provider config: %w", err)
	}

	if providerConfig.APIKey == "" {
		return fmt.Errorf("no API key configured for provider %s. Configure it in ~/.config/dreamteller/config.yaml", providerName)
	}

	// Initialize LLM provider
	ctx := context.Background()
	provider, err := initLLMProvider(ctx, providerName, providerConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize LLM provider: %w", err)
	}
	defer provider.Close()

	// Parse the prompt using AI
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

func init() {
	// Add flags to newCmd
	newCmd.Flags().String("from-prompt", "", "Path to prompt file for one-shot setup (use '-' for stdin)")
	newCmd.Flags().String("genre", "", "Genre for quick project creation without wizard")

	// Add subcommands
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(reindexCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(configCmd)
}

func runTUI(proj *project.Project) error {
	searchEngine := search.NewFTSEngine(proj.DB)

	application, err := app.New()
	if err != nil {
		return fmt.Errorf("failed to initialize app: %w", err)
	}

	globalConfig, err := application.Config.LoadGlobalConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	providerName := globalConfig.Defaults.Provider
	if providerName == "" {
		providerName = "openai"
	}

	providerConfig, err := application.Config.GetProviderConfig(providerName)
	if err != nil {
		return fmt.Errorf("failed to get provider config: %w", err)
	}

	ctx := context.Background()
	provider, err := initLLMProvider(ctx, providerName, providerConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize LLM provider: %w", err)
	}
	defer provider.Close()

	model := tui.New(proj, provider, searchEngine)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
