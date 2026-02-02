// Package app provides application lifecycle management.
package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/azyu/dreamteller/pkg/types"
	"gopkg.in/yaml.v3"
)

var (
	ErrConfigNotFound = errors.New("configuration file not found")
	ErrInvalidConfig  = errors.New("invalid configuration")
)

// ConfigManager handles global and project configuration.
type ConfigManager struct {
	globalConfigPath string
	globalConfig     *types.GlobalConfig
}

// NewConfigManager creates a new configuration manager.
func NewConfigManager() (*ConfigManager, error) {
	configDir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	return &ConfigManager{
		globalConfigPath: filepath.Join(configDir, "config.yaml"),
	}, nil
}

// getConfigDir returns the configuration directory path.
func getConfigDir() (string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "dreamteller"), nil
}

// LoadGlobalConfig loads the global configuration.
func (cm *ConfigManager) LoadGlobalConfig() (*types.GlobalConfig, error) {
	if cm.globalConfig != nil {
		return cm.globalConfig, nil
	}

	data, err := os.ReadFile(cm.globalConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			cm.globalConfig = types.DefaultGlobalConfig()
			return cm.globalConfig, nil
		}
		return nil, fmt.Errorf("failed to read global config: %w", err)
	}

	var config types.GlobalConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse global config: %w", err)
	}

	// Expand environment variables in API keys
	for name, provider := range config.Providers {
		if strings.HasPrefix(provider.APIKey, "${") && strings.HasSuffix(provider.APIKey, "}") {
			envVar := provider.APIKey[2 : len(provider.APIKey)-1]
			provider.APIKey = os.Getenv(envVar)
			config.Providers[name] = provider
		}
	}

	// Expand ~ in projects directory
	config.ProjectsDir = expandPath(config.ProjectsDir)

	cm.globalConfig = &config
	return cm.globalConfig, nil
}

// SaveGlobalConfig saves the global configuration.
func (cm *ConfigManager) SaveGlobalConfig(config *types.GlobalConfig) error {
	// Ensure directory exists
	dir := filepath.Dir(cm.globalConfigPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := atomicWrite(cm.globalConfigPath, data); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	cm.globalConfig = config
	return nil
}

// expandPath expands ~ to home directory.
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// atomicWrite writes data to a file atomically using temp file + rename.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()

	defer func() {
		// Clean up temp file on error
		if tmpPath != "" {
			os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		return err
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return err
	}

	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}

	tmpPath = "" // Prevent cleanup since rename succeeded
	return nil
}

// GetProjectsDir returns the projects directory path.
func (cm *ConfigManager) GetProjectsDir() (string, error) {
	config, err := cm.LoadGlobalConfig()
	if err != nil {
		return "", err
	}
	return config.ProjectsDir, nil
}

// GetProviderConfig returns the configuration for a specific provider.
func (cm *ConfigManager) GetProviderConfig(providerName string) (*types.ProviderConfig, error) {
	config, err := cm.LoadGlobalConfig()
	if err != nil {
		return nil, err
	}

	provider, ok := config.Providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %q not configured", providerName)
	}

	return provider, nil
}
