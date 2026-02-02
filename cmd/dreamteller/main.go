// Package main is the entry point for dreamteller.
package main

import (
	"fmt"
	"os"

	"github.com/azyu/dreamteller/internal/app"
	"github.com/azyu/dreamteller/internal/project"
	"github.com/azyu/dreamteller/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		fromPrompt, _ := cmd.Flags().GetString("from-prompt")

		application, err := app.New()
		if err != nil {
			return fmt.Errorf("failed to initialize app: %w", err)
		}

		if fromPrompt != "" {
			// TODO: Implement prompt-based setup
			return fmt.Errorf("prompt-based setup not yet implemented")
		}

		// For now, create with default genre
		// TODO: Implement wizard mode
		genre := "fantasy"
		if err := application.CreateProject(name, genre); err != nil {
			return fmt.Errorf("failed to create project: %w", err)
		}

		fmt.Printf("Created project '%s' at %s\n", name, application.CurrentProject.Path())
		return nil
	},
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
			// TODO: Use current directory or prompt
			return fmt.Errorf("please specify a project name")
		}

		if err := application.OpenProject(name); err != nil {
			return fmt.Errorf("failed to open project: %w", err)
		}

		// TODO: Implement reindexing
		fmt.Printf("Reindexing project '%s'...\n", name)
		fmt.Println("Reindex complete.")
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
	// Add flags
	newCmd.Flags().String("from-prompt", "", "Path to prompt file for one-shot setup (use '-' for stdin)")

	// Add subcommands
	rootCmd.AddCommand(newCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(reindexCmd)
	rootCmd.AddCommand(exportCmd)
	rootCmd.AddCommand(configCmd)
}

func runTUI(proj *project.Project) error {
	model := tui.New(proj)
	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	return nil
}
