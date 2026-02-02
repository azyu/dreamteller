package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultProjectConfig(t *testing.T) {
	tests := []struct {
		name          string
		projectName   string
		genre         string
		wantVersion   int
		wantProvider  string
		wantModel     string
		wantMaxChunks int
		wantChunkSize int
		wantStyle     string
		wantPOV       string
		wantTense     string
	}{
		{
			name:          "creates config with fantasy genre",
			projectName:   "My Fantasy Novel",
			genre:         "fantasy",
			wantVersion:   1,
			wantProvider:  "openai",
			wantModel:     "gpt-4-turbo",
			wantMaxChunks: 5,
			wantChunkSize: 800,
			wantStyle:     "descriptive, immersive",
			wantPOV:       "third-person-limited",
			wantTense:     "past",
		},
		{
			name:          "creates config with scifi genre",
			projectName:   "Space Adventure",
			genre:         "scifi",
			wantVersion:   1,
			wantProvider:  "openai",
			wantModel:     "gpt-4-turbo",
			wantMaxChunks: 5,
			wantChunkSize: 800,
			wantStyle:     "descriptive, immersive",
			wantPOV:       "third-person-limited",
			wantTense:     "past",
		},
		{
			name:          "creates config with empty values",
			projectName:   "",
			genre:         "",
			wantVersion:   1,
			wantProvider:  "openai",
			wantModel:     "gpt-4-turbo",
			wantMaxChunks: 5,
			wantChunkSize: 800,
			wantStyle:     "descriptive, immersive",
			wantPOV:       "third-person-limited",
			wantTense:     "past",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultProjectConfig(tt.projectName, tt.genre)

			// Verify name and genre are passed through
			assert.Equal(t, tt.projectName, cfg.Name)
			assert.Equal(t, tt.genre, cfg.Genre)

			// Verify version
			assert.Equal(t, tt.wantVersion, cfg.Version)

			// Verify LLM defaults
			assert.Equal(t, tt.wantProvider, cfg.LLM.Provider)
			assert.Equal(t, tt.wantModel, cfg.LLM.Model)

			// Verify Context defaults
			assert.Equal(t, tt.wantMaxChunks, cfg.Context.MaxChunks)
			assert.Equal(t, tt.wantChunkSize, cfg.Context.ChunkSize)
			assert.Equal(t, 0.15, cfg.Context.ChunkOverlap)

			// Verify Writing defaults
			assert.Equal(t, tt.wantStyle, cfg.Writing.Style)
			assert.Equal(t, tt.wantPOV, cfg.Writing.POV)
			assert.Equal(t, tt.wantTense, cfg.Writing.Tense)

			// Verify CreatedAt is set (non-zero)
			assert.False(t, cfg.CreatedAt.IsZero())
		})
	}
}

func TestDefaultProjectConfig_BudgetRatios(t *testing.T) {
	cfg := DefaultProjectConfig("Test Project", "mystery")

	// Verify individual budget ratios
	assert.Equal(t, 0.20, cfg.Budget.SystemPrompt)
	assert.Equal(t, 0.40, cfg.Budget.Context)
	assert.Equal(t, 0.30, cfg.Budget.History)
	assert.Equal(t, 0.10, cfg.Budget.Response)

	// Verify budget ratios sum to 1.0
	totalRatio := cfg.Budget.SystemPrompt +
		cfg.Budget.Context +
		cfg.Budget.History +
		cfg.Budget.Response
	assert.InDelta(t, 1.0, totalRatio, 0.0001, "budget ratios should sum to 1.0")
}

func TestDefaultGlobalConfig(t *testing.T) {
	tests := []struct {
		name            string
		wantVersion     int
		wantProjectsDir string
		wantProvider    string
		wantLogLevel    string
	}{
		{
			name:            "creates global config with defaults",
			wantVersion:     1,
			wantProjectsDir: "~/dreamteller-projects",
			wantProvider:    "openai",
			wantLogLevel:    "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultGlobalConfig()

			// Verify version
			assert.Equal(t, tt.wantVersion, cfg.Version)

			// Verify projects directory
			assert.Equal(t, tt.wantProjectsDir, cfg.ProjectsDir)

			// Verify defaults
			assert.Equal(t, tt.wantProvider, cfg.Defaults.Provider)

			// Verify logging
			assert.Equal(t, tt.wantLogLevel, cfg.Logging.Level)
		})
	}
}

func TestDefaultGlobalConfig_ProvidersMapInitialized(t *testing.T) {
	cfg := DefaultGlobalConfig()

	// Verify providers map is initialized (not nil)
	assert.NotNil(t, cfg.Providers, "providers map should be initialized")

	// Verify providers map is empty initially
	assert.Empty(t, cfg.Providers, "providers map should be empty initially")

	// Verify we can add to the providers map without panic
	cfg.Providers["openai"] = &ProviderConfig{
		APIKey:       "test-key",
		DefaultModel: "gpt-4",
	}
	assert.Len(t, cfg.Providers, 1)
	assert.Equal(t, "test-key", cfg.Providers["openai"].APIKey)
	assert.Equal(t, "gpt-4", cfg.Providers["openai"].DefaultModel)
}
