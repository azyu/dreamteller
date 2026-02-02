// Package project provides project management functionality.
package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/azyu/dreamteller/internal/storage"
	"github.com/azyu/dreamteller/pkg/types"
)

var (
	ErrProjectNotFound = errors.New("project not found")
	ErrProjectExists   = errors.New("project already exists")
	ErrInvalidName     = errors.New("invalid project name")
)

// Manager handles project lifecycle operations.
type Manager struct {
	projectsDir string
}

// NewManager creates a new project manager.
func NewManager(projectsDir string) (*Manager, error) {
	// Expand ~ if present
	if strings.HasPrefix(projectsDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		projectsDir = filepath.Join(home, projectsDir[2:])
	}

	// Ensure projects directory exists
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create projects directory: %w", err)
	}

	return &Manager{
		projectsDir: projectsDir,
	}, nil
}

// Project represents an open novel project.
type Project struct {
	Info   *types.Project
	Config *types.ProjectConfig
	FS     *storage.FileSystem
	DB     *storage.SQLiteDB
	path   string
}

// Create creates a new project.
func (m *Manager) Create(name string, config *types.ProjectConfig) (*Project, error) {
	if !isValidName(name) {
		return nil, ErrInvalidName
	}

	projectPath := filepath.Join(m.projectsDir, name)

	// Check if project already exists
	if _, err := os.Stat(projectPath); err == nil {
		return nil, ErrProjectExists
	}

	// Create project directory structure
	dirs := []string{
		".dreamteller",
		"context/characters",
		"context/settings",
		"context/plot",
		"chapters",
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(projectPath, dir), 0755); err != nil {
			// Clean up on failure
			os.RemoveAll(projectPath)
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Save project config
	if err := SaveProjectConfig(projectPath, config); err != nil {
		os.RemoveAll(projectPath)
		return nil, fmt.Errorf("failed to save project config: %w", err)
	}

	// Create README.md
	readme := fmt.Sprintf("# %s\n\nA %s novel created with Dreamteller.\n\nCreated: %s\n",
		config.Name, config.Genre, config.CreatedAt.Format("2006-01-02"))

	if err := storage.AtomicWriteFile(filepath.Join(projectPath, "README.md"), []byte(readme)); err != nil {
		os.RemoveAll(projectPath)
		return nil, fmt.Errorf("failed to create README: %w", err)
	}

	// Open the newly created project
	return m.Open(name)
}

// Open opens an existing project.
func (m *Manager) Open(name string) (*Project, error) {
	projectPath := filepath.Join(m.projectsDir, name)

	// Check if project exists
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return nil, ErrProjectNotFound
	}

	// Load config
	config, err := LoadProjectConfig(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load project config: %w", err)
	}

	// Initialize storage
	fs := storage.NewFileSystem(projectPath)

	db, err := storage.NewSQLiteDB(projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &Project{
		Info: &types.Project{
			Name:      config.Name,
			Path:      projectPath,
			Genre:     config.Genre,
			CreatedAt: config.CreatedAt,
			UpdatedAt: time.Now(),
		},
		Config: config,
		FS:     fs,
		DB:     db,
		path:   projectPath,
	}, nil
}

// List returns all available projects.
func (m *Manager) List() ([]*types.Project, error) {
	entries, err := os.ReadDir(m.projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*types.Project{}, nil
		}
		return nil, err
	}

	var projects []*types.Project
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectPath := filepath.Join(m.projectsDir, entry.Name())
		configPath := filepath.Join(projectPath, ".dreamteller", "config.yaml")

		// Check if this is a valid project
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			continue
		}

		config, err := LoadProjectConfig(projectPath)
		if err != nil {
			continue // Skip invalid projects
		}

		info, _ := entry.Info()
		projects = append(projects, &types.Project{
			Name:      config.Name,
			Path:      projectPath,
			Genre:     config.Genre,
			CreatedAt: config.CreatedAt,
			UpdatedAt: info.ModTime(),
		})
	}

	return projects, nil
}

// Delete removes a project.
func (m *Manager) Delete(name string) error {
	projectPath := filepath.Join(m.projectsDir, name)

	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return ErrProjectNotFound
	}

	return os.RemoveAll(projectPath)
}

// isValidName checks if a project name is valid.
func isValidName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}

	// Check for invalid characters
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", "..", " "}
	for _, char := range invalid {
		if strings.Contains(name, char) {
			return false
		}
	}

	// Check for reserved names
	reserved := []string{".", "..", "con", "prn", "aux", "nul"}
	nameLower := strings.ToLower(name)
	for _, r := range reserved {
		if nameLower == r {
			return false
		}
	}

	return true
}

// Path returns the project's filesystem path.
func (p *Project) Path() string {
	return p.path
}

// Close releases project resources.
func (p *Project) Close() error {
	if p.DB != nil {
		return p.DB.Close()
	}
	return nil
}

// LoadCharacters loads all character files.
func (p *Project) LoadCharacters() ([]*types.Character, error) {
	files, err := p.FS.ListMarkdownFiles("context/characters")
	if err != nil {
		return nil, err
	}

	var characters []*types.Character
	for _, file := range files {
		content, err := p.FS.ReadMarkdown(file.Path)
		if err != nil {
			continue
		}

		title := p.FS.ParseMarkdownTitle(content)
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(file.Path), ".md")
		}

		characters = append(characters, &types.Character{
			Name:        title,
			Description: content,
			FilePath:    file.Path,
		})
	}

	return characters, nil
}

// LoadSettings loads all setting files.
func (p *Project) LoadSettings() ([]*types.Setting, error) {
	files, err := p.FS.ListMarkdownFiles("context/settings")
	if err != nil {
		return nil, err
	}

	var settings []*types.Setting
	for _, file := range files {
		content, err := p.FS.ReadMarkdown(file.Path)
		if err != nil {
			continue
		}

		title := p.FS.ParseMarkdownTitle(content)
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(file.Path), ".md")
		}

		settings = append(settings, &types.Setting{
			Name:        title,
			Description: content,
			FilePath:    file.Path,
		})
	}

	return settings, nil
}

// LoadPlots loads all plot files.
func (p *Project) LoadPlots() ([]*types.PlotPoint, error) {
	files, err := p.FS.ListMarkdownFiles("context/plot")
	if err != nil {
		return nil, err
	}

	var plots []*types.PlotPoint
	for i, file := range files {
		content, err := p.FS.ReadMarkdown(file.Path)
		if err != nil {
			continue
		}

		title := p.FS.ParseMarkdownTitle(content)
		if title == "" {
			title = strings.TrimSuffix(filepath.Base(file.Path), ".md")
		}

		plots = append(plots, &types.PlotPoint{
			Title:       title,
			Description: content,
			Order:       i + 1,
			FilePath:    file.Path,
		})
	}

	return plots, nil
}

// LoadChapters loads all chapter files.
func (p *Project) LoadChapters() ([]*types.Chapter, error) {
	files, err := p.FS.ListMarkdownFiles("chapters")
	if err != nil {
		return nil, err
	}

	var chapters []*types.Chapter
	for i, file := range files {
		content, err := p.FS.ReadMarkdown(file.Path)
		if err != nil {
			continue
		}

		title := p.FS.ParseMarkdownTitle(content)
		if title == "" {
			title = fmt.Sprintf("Chapter %d", i+1)
		}

		chapters = append(chapters, &types.Chapter{
			Number:    i + 1,
			Title:     title,
			Content:   content,
			FilePath:  file.Path,
			CreatedAt: file.ModTime,
			UpdatedAt: file.ModTime,
		})
	}

	return chapters, nil
}

// SaveChapter saves a chapter to disk.
func (p *Project) SaveChapter(chapter *types.Chapter) error {
	filename := fmt.Sprintf("chapter-%03d.md", chapter.Number)
	return p.FS.WriteMarkdown(filepath.Join("chapters", filename), chapter.Content)
}

// CreateContextFile creates a new context file.
func (p *Project) CreateContextFile(category, filename, content string) error {
	path := filepath.Join("context", category, filename)
	if !strings.HasSuffix(path, ".md") {
		path += ".md"
	}
	return p.FS.WriteMarkdown(path, content)
}

// WriteContextContent writes or updates context content based on operation.
func (p *Project) WriteContextContent(category, filename, content, operation string) error {
	path := filepath.Join("context", category, filename)
	if !strings.HasSuffix(path, ".md") {
		path += ".md"
	}

	switch operation {
	case "create", "update":
		return p.FS.WriteMarkdown(path, content)
	case "append":
		existing, err := p.FS.ReadMarkdown(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to read existing file: %w", err)
		}
		newContent := existing + "\n\n" + content
		return p.FS.WriteMarkdown(path, newContent)
	default:
		return fmt.Errorf("unknown operation: %s", operation)
	}
}

// WriteCharacterContent writes character content.
func (p *Project) WriteCharacterContent(filename, content, operation string) error {
	return p.WriteContextContent("characters", filename, content, operation)
}

// WriteSettingContent writes setting content.
func (p *Project) WriteSettingContent(filename, content, operation string) error {
	return p.WriteContextContent("settings", filename, content, operation)
}

// WritePlotContent writes plot content.
func (p *Project) WritePlotContent(filename, content, operation string) error {
	return p.WriteContextContent("plot", filename, content, operation)
}
