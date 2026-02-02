// Package app provides application lifecycle management.
package app

import (
	"fmt"

	"github.com/azyu/dreamteller/internal/project"
	"github.com/azyu/dreamteller/pkg/types"
)

// App represents the main application instance.
type App struct {
	Config         *ConfigManager
	ProjectManager *project.Manager
	CurrentProject *project.Project
}

// New creates a new application instance.
func New() (*App, error) {
	configManager, err := NewConfigManager()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config manager: %w", err)
	}

	// Load global config to get projects directory
	globalConfig, err := configManager.LoadGlobalConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load global config: %w", err)
	}

	projectManager, err := project.NewManager(globalConfig.ProjectsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize project manager: %w", err)
	}

	return &App{
		Config:         configManager,
		ProjectManager: projectManager,
	}, nil
}

// OpenProject opens an existing project by name.
func (a *App) OpenProject(name string) error {
	proj, err := a.ProjectManager.Open(name)
	if err != nil {
		return fmt.Errorf("failed to open project: %w", err)
	}
	a.CurrentProject = proj
	return nil
}

// CreateProject creates a new project.
func (a *App) CreateProject(name, genre string) error {
	config := types.DefaultProjectConfig(name, genre)
	proj, err := a.ProjectManager.Create(name, config)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}
	a.CurrentProject = proj
	return nil
}

// ListProjects returns all available projects.
func (a *App) ListProjects() ([]*types.Project, error) {
	return a.ProjectManager.List()
}

// Close cleans up application resources.
func (a *App) Close() error {
	if a.CurrentProject != nil {
		return a.CurrentProject.Close()
	}
	return nil
}
