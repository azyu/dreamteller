// Package project provides project management functionality.
package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/azyu/dreamteller/internal/storage"
	"github.com/azyu/dreamteller/pkg/types"
	"gopkg.in/yaml.v3"
)

var (
	ErrConfigNotFound = errors.New("configuration file not found")
)

// LoadProjectConfig loads a project's configuration.
func LoadProjectConfig(projectPath string) (*types.ProjectConfig, error) {
	configPath := filepath.Join(projectPath, ".dreamteller", "config.yaml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrConfigNotFound
		}
		return nil, fmt.Errorf("failed to read project config: %w", err)
	}

	var config types.ProjectConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse project config: %w", err)
	}

	return &config, nil
}

// SaveProjectConfig saves a project's configuration.
func SaveProjectConfig(projectPath string, config *types.ProjectConfig) error {
	configDir := filepath.Join(projectPath, ".dreamteller")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create .dreamteller directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal project config: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	if err := storage.AtomicWriteFile(configPath, data); err != nil {
		return fmt.Errorf("failed to write project config: %w", err)
	}

	return nil
}
