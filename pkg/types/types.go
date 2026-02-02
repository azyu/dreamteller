// Package types provides shared data models for dreamteller.
package types

import (
	"time"
)

// Project represents a novel writing project.
type Project struct {
	Name      string    `yaml:"name" json:"name"`
	Path      string    `yaml:"-" json:"path"`
	Genre     string    `yaml:"genre" json:"genre"`
	CreatedAt time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at" json:"updated_at"`
}

// ProjectConfig is the per-project configuration stored in .dreamteller/config.yaml.
type ProjectConfig struct {
	Version   int           `yaml:"version"`
	Name      string        `yaml:"name"`
	Genre     string        `yaml:"genre"`
	CreatedAt time.Time     `yaml:"created_at"`
	LLM       LLMConfig     `yaml:"llm"`
	Context   ContextConfig `yaml:"context"`
	Budget    BudgetConfig  `yaml:"token_budget"`
	Writing   WritingConfig `yaml:"writing"`
}

// LLMConfig specifies the LLM provider settings.
type LLMConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// ContextConfig controls semantic search and context injection.
type ContextConfig struct {
	MaxChunks    int     `yaml:"max_chunks"`
	ChunkSize    int     `yaml:"chunk_size"`
	ChunkOverlap float64 `yaml:"chunk_overlap"`
}

// BudgetConfig defines token budget allocation ratios.
type BudgetConfig struct {
	SystemPrompt float64 `yaml:"system_prompt"`
	Context      float64 `yaml:"context"`
	History      float64 `yaml:"history"`
	Response     float64 `yaml:"response"`
}

// WritingConfig holds writing style preferences.
type WritingConfig struct {
	Style string `yaml:"style"`
	POV   string `yaml:"pov"`
	Tense string `yaml:"tense"`
}

// GlobalConfig is the user-wide configuration at ~/.config/dreamteller/config.yaml.
type GlobalConfig struct {
	Version     int                        `yaml:"version"`
	ProjectsDir string                     `yaml:"projects_dir"`
	Providers   map[string]*ProviderConfig `yaml:"providers"`
	Defaults    DefaultsConfig             `yaml:"defaults"`
	Logging     LoggingConfig              `yaml:"logging"`
}

// ProviderConfig holds API configuration for an LLM provider.
type ProviderConfig struct {
	APIKey       string `yaml:"api_key"`
	DefaultModel string `yaml:"default_model"`
	BaseURL      string `yaml:"base_url,omitempty"`
}

// DefaultsConfig specifies default settings.
type DefaultsConfig struct {
	Provider string `yaml:"provider"`
}

// LoggingConfig specifies logging settings.
type LoggingConfig struct {
	Level string `yaml:"level"`
}

// Character represents a character in the novel.
type Character struct {
	Name        string            `yaml:"name" json:"name"`
	Description string            `yaml:"description" json:"description"`
	Traits      map[string]string `yaml:"traits" json:"traits"`
	FilePath    string            `yaml:"-" json:"file_path"`
}

// Setting represents a world/location setting.
type Setting struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	FilePath    string `yaml:"-" json:"file_path"`
}

// PlotPoint represents a plot element.
type PlotPoint struct {
	Title       string `yaml:"title" json:"title"`
	Description string `yaml:"description" json:"description"`
	Order       int    `yaml:"order" json:"order"`
	FilePath    string `yaml:"-" json:"file_path"`
}

// Chapter represents a written chapter.
type Chapter struct {
	Number    int       `yaml:"number" json:"number"`
	Title     string    `yaml:"title" json:"title"`
	Content   string    `yaml:"-" json:"content,omitempty"`
	FilePath  string    `yaml:"-" json:"file_path"`
	CreatedAt time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at" json:"updated_at"`
}

// Chunk represents a text chunk for indexing and retrieval.
type Chunk struct {
	ID         int64             `json:"id"`
	Content    string            `json:"content"`
	SourceType string            `json:"source_type"` // character, setting, plot, chapter
	SourcePath string            `json:"source_path"`
	Metadata   map[string]string `json:"metadata"`
	TokenCount int               `json:"token_count"`
}

// SearchResult represents a search result with relevance score.
type SearchResult struct {
	Chunk Chunk   `json:"chunk"`
	Score float64 `json:"score"`
}

// ConversationMessage represents a single message in the conversation.
type ConversationMessage struct {
	Role      string    `json:"role"` // user, assistant, system
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// ConversationState persists the conversation history.
type ConversationState struct {
	Messages  []ConversationMessage `json:"messages"`
	UpdatedAt time.Time             `json:"updated_at"`
}

// ParsePromptResult is the result of AI parsing a free-form setup prompt.
type ParsePromptResult struct {
	Genre      string          `json:"genre"`
	Setting    SettingInfo     `json:"setting"`
	Characters []CharacterInfo `json:"characters"`
	PlotHints  []string        `json:"plot_hints"`
	StyleGuide StyleInfo       `json:"style_guide"`
}

// SettingInfo contains extracted setting information.
type SettingInfo struct {
	TimePeriod  string `json:"time_period"`
	Location    string `json:"location"`
	Description string `json:"description"`
}

// CharacterInfo contains extracted character information.
type CharacterInfo struct {
	Name        string            `json:"name"`
	Role        string            `json:"role"` // protagonist, antagonist, supporting
	Description string            `json:"description"`
	Traits      map[string]string `json:"traits"`
}

// StyleInfo contains extracted writing style preferences.
type StyleInfo struct {
	Tone       string   `json:"tone"`
	Pacing     string   `json:"pacing"`
	Dialogue   string   `json:"dialogue"`
	Vocabulary []string `json:"vocabulary_notes"`
}

// DefaultProjectConfig returns a new ProjectConfig with sensible defaults.
func DefaultProjectConfig(name, genre string) *ProjectConfig {
	return &ProjectConfig{
		Version:   1,
		Name:      name,
		Genre:     genre,
		CreatedAt: time.Now(),
		LLM: LLMConfig{
			Provider: "openai",
			Model:    "gpt-4-turbo",
		},
		Context: ContextConfig{
			MaxChunks:    5,
			ChunkSize:    800,
			ChunkOverlap: 0.15,
		},
		Budget: BudgetConfig{
			SystemPrompt: 0.20,
			Context:      0.40,
			History:      0.30,
			Response:     0.10,
		},
		Writing: WritingConfig{
			Style: "descriptive, immersive",
			POV:   "third-person-limited",
			Tense: "past",
		},
	}
}

// DefaultGlobalConfig returns a new GlobalConfig with sensible defaults.
func DefaultGlobalConfig() *GlobalConfig {
	return &GlobalConfig{
		Version:     1,
		ProjectsDir: "~/dreamteller-projects",
		Providers:   make(map[string]*ProviderConfig),
		Defaults: DefaultsConfig{
			Provider: "openai",
		},
		Logging: LoggingConfig{
			Level: "info",
		},
	}
}
