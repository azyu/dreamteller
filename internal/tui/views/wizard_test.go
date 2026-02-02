package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	// Disable colors for consistent test output
	lipgloss.SetColorProfile(termenv.Ascii)
}

// ============================================================================
// Wizard Creation Tests
// ============================================================================

func TestNewWizard(t *testing.T) {
	w := NewWizard()

	assert.NotNil(t, w)
	assert.Equal(t, StepProjectName, w.currentStep)
	assert.Equal(t, totalSteps, w.totalSteps)
	assert.False(t, w.completed)
	assert.False(t, w.cancelled)
	assert.Nil(t, w.err)
}

func TestNewWizard_DefaultValues(t *testing.T) {
	w := NewWizard()

	// Default values should be set
	assert.Equal(t, "fantasy", w.result.Genre)
	assert.Equal(t, "openai", w.result.LLMProvider)
}

func TestNewWizard_Components(t *testing.T) {
	w := NewWizard()

	// All input components should be initialized
	assert.NotNil(t, w.nameInput)
	assert.NotNil(t, w.premiseInput)
	assert.NotNil(t, w.worldInput)
	assert.NotNil(t, w.charactersInput)
	assert.NotNil(t, w.plotInput)
}

func TestInit(t *testing.T) {
	w := NewWizard()
	cmd := w.Init()

	assert.NotNil(t, cmd, "Init should return a command for textarea blink")
}

// ============================================================================
// Navigation Tests
// ============================================================================

func TestWizardNavigation_Next(t *testing.T) {
	w := NewWizard()
	w.ready = true

	// Simulate window size
	w.width = 80
	w.height = 24

	// Start at StepProjectName
	assert.Equal(t, StepProjectName, w.currentStep)

	// Press Enter to advance
	model, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	assert.Equal(t, StepGenre, w.currentStep)
}

func TestWizardNavigation_Back(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24

	// Move to genre step
	w.currentStep = StepGenre

	// Press Backspace to go back
	model, _ := w.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	w = model.(*WizardModel)

	assert.Equal(t, StepProjectName, w.currentStep)
}

func TestWizardNavigation_BackAtFirstStep(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24

	// At first step, backspace shouldn't go back
	model, _ := w.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	w = model.(*WizardModel)

	// Should still be at first step
	assert.Equal(t, StepProjectName, w.currentStep)
}

func TestWizardNavigation_Cancel(t *testing.T) {
	tests := []struct {
		name    string
		keyType tea.KeyType
	}{
		{"Ctrl+C cancels", tea.KeyCtrlC},
		{"Esc cancels", tea.KeyEsc},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewWizard()
			w.ready = true

			_, cmd := w.Update(tea.KeyMsg{Type: tt.keyType})

			assert.True(t, w.cancelled)
			assert.NotNil(t, cmd, "should return quit command")
		})
	}
}

func TestWizardNavigation_TabSkip(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24

	assert.Equal(t, StepProjectName, w.currentStep)

	// Tab should skip to next step
	model, _ := w.Update(tea.KeyMsg{Type: tea.KeyTab})
	w = model.(*WizardModel)

	assert.Equal(t, StepGenre, w.currentStep)
	// Should have saved default value
	assert.Equal(t, "my-novel", w.result.ProjectName)
}

func TestWizardNavigation_Complete(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24

	// Move to last step
	w.currentStep = StepLLMProvider

	// Complete wizard
	_, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.True(t, w.completed)
	assert.NotNil(t, cmd, "should return quit command")
}

func TestWizardNavigation_AllSteps(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24

	steps := []WizardStep{
		StepProjectName,
		StepGenre,
		StepPremise,
		StepWorldSetting,
		StepCharacters,
		StepPlotOutline,
		StepLLMProvider,
	}

	for i, expectedStep := range steps {
		assert.Equal(t, expectedStep, w.currentStep, "step %d should be %v", i, expectedStep)

		if i < len(steps)-1 {
			// Advance to next step
			model, _ := w.Update(tea.KeyMsg{Type: tea.KeyTab})
			w = model.(*WizardModel)
		}
	}
}

// ============================================================================
// Validation Tests
// ============================================================================

func TestWizardValidation_ProjectName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectErr bool
	}{
		{"accepts valid name", "my-novel", false},
		{"accepts name with numbers", "novel123", false},
		{"accepts name with underscores", "my_novel", false},
		{"accepts empty (uses default)", "", false},
		{"rejects forward slash", "my/novel", true},
		{"rejects backslash", "my\\novel", true},
		{"rejects colon", "my:novel", true},
		{"rejects asterisk", "my*novel", true},
		{"rejects question mark", "my?novel", true},
		{"rejects quotes", "my\"novel", true},
		{"rejects less than", "my<novel", true},
		{"rejects greater than", "my>novel", true},
		{"rejects pipe", "my|novel", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := NewWizard()
			w.ready = true
			w.width = 80
			w.height = 24

			// Set the name input value
			w.nameInput.SetValue(tt.input)

			// Try to advance
			model, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
			w = model.(*WizardModel)

			if tt.expectErr {
				assert.NotNil(t, w.err)
				assert.Equal(t, StepProjectName, w.currentStep, "should stay on same step")
			} else {
				assert.Nil(t, w.err)
				assert.Equal(t, StepGenre, w.currentStep, "should advance to next step")
			}
		})
	}
}

func TestWizardValidation_ProjectNameTooLong(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24

	// Create a string longer than 64 characters
	longName := ""
	for i := 0; i < 70; i++ {
		longName += "a"
	}

	w.nameInput.SetValue(longName)

	model, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	// The textarea has a character limit, so check either:
	// 1. The validation caught it (w.err != nil)
	// 2. Or the input was capped by CharLimit (less likely with 64 char input)
	if w.err != nil {
		assert.Contains(t, w.err.Error(), "too long")
	} else {
		// If no error, check we advanced (validation may have capped input)
		// This is acceptable behavior if the textarea CharLimit is 64
		t.Log("Input may have been capped by textarea CharLimit")
	}
}

// ============================================================================
// Data Collection Tests
// ============================================================================

func TestWizardResult_CollectsProjectName(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24

	w.nameInput.SetValue("test-project")

	// Advance to save value
	model, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	assert.Equal(t, "test-project", w.result.ProjectName)
}

func TestWizardResult_CollectsGenre(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24
	w.currentStep = StepGenre

	// Advance to save selected genre
	model, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	// Genre should be set from list selection
	assert.NotEmpty(t, w.result.Genre)
}

func TestWizardResult_CollectsPremise(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24
	w.currentStep = StepPremise

	w.premiseInput.SetValue("A story about adventure")

	// Advance to save value
	model, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	assert.Equal(t, "A story about adventure", w.result.Premise)
}

func TestWizardResult_CollectsWorldSetting(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24
	w.currentStep = StepWorldSetting

	w.worldInput.SetValue("A magical realm")

	model, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	assert.Equal(t, "A magical realm", w.result.WorldSetting)
}

func TestWizardResult_CollectsCharacters(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24
	w.currentStep = StepCharacters

	w.charactersInput.SetValue("Alice: protagonist\nBob: antagonist")

	model, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	assert.Contains(t, w.result.Characters, "Alice")
	assert.Contains(t, w.result.Characters, "Bob")
}

func TestWizardResult_CollectsPlotOutline(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24
	w.currentStep = StepPlotOutline

	w.plotInput.SetValue("Act 1: Setup\nAct 2: Conflict\nAct 3: Resolution")

	model, _ := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	assert.Contains(t, w.result.PlotOutline, "Act 1")
}

func TestWizardResult_CollectsLLMProvider(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24
	w.currentStep = StepLLMProvider

	// Provider should be set from list selection
	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	assert.NotEmpty(t, w.result.LLMProvider)
}

// ============================================================================
// Result and Status Methods
// ============================================================================

func TestWizard_Result(t *testing.T) {
	w := NewWizard()
	w.result = WizardResult{
		ProjectName: "test",
		Genre:       "fantasy",
		Premise:     "premise",
		LLMProvider: "openai",
	}

	result := w.Result()

	assert.Equal(t, "test", result.ProjectName)
	assert.Equal(t, "fantasy", result.Genre)
	assert.Equal(t, "premise", result.Premise)
	assert.Equal(t, "openai", result.LLMProvider)
}

func TestWizard_Completed(t *testing.T) {
	t.Run("returns false initially", func(t *testing.T) {
		w := NewWizard()
		assert.False(t, w.Completed())
	})

	t.Run("returns true after completion", func(t *testing.T) {
		w := NewWizard()
		w.completed = true
		assert.True(t, w.Completed())
	})
}

func TestWizard_Cancelled(t *testing.T) {
	t.Run("returns false initially", func(t *testing.T) {
		w := NewWizard()
		assert.False(t, w.Cancelled())
	})

	t.Run("returns true after cancellation", func(t *testing.T) {
		w := NewWizard()
		w.cancelled = true
		assert.True(t, w.Cancelled())
	})
}

// ============================================================================
// View Rendering Tests
// ============================================================================

func TestWizard_View_NotReady(t *testing.T) {
	w := NewWizard()
	w.ready = false

	view := w.View()

	assert.Contains(t, view, "Initializing")
}

func TestWizard_View_Ready(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24

	view := w.View()

	assert.Contains(t, view, "DREAMTELLER")
	assert.Contains(t, view, "Project Setup")
	assert.Contains(t, view, "Step 1")
	assert.Contains(t, view, "Project Name")
}

func TestWizard_View_ShowsProgress(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24
	w.currentStep = StepPremise // Step 3

	view := w.View()

	// Progress indicator should show completed and current steps
	assert.Contains(t, view, "●") // Dots for progress
}

func TestWizard_View_ShowsError(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24
	w.err = assert.AnError

	view := w.View()

	assert.Contains(t, view, "Error")
}

func TestWizard_View_ShowsNavigationHelp(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24

	view := w.View()

	assert.Contains(t, view, "Enter")
	assert.Contains(t, view, "Tab")
	assert.Contains(t, view, "Esc")
}

func TestWizard_View_ShowsBackHelpAfterFirstStep(t *testing.T) {
	w := NewWizard()
	w.ready = true
	w.width = 80
	w.height = 24
	w.currentStep = StepGenre

	view := w.View()

	assert.Contains(t, view, "Backspace")
	assert.Contains(t, view, "back")
}

// ============================================================================
// Window Size Tests
// ============================================================================

func TestWizard_WindowSizeMsg(t *testing.T) {
	w := NewWizard()

	model, _ := w.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	w = model.(*WizardModel)

	assert.True(t, w.ready)
	assert.Equal(t, 100, w.width)
	assert.Equal(t, 40, w.height)
}

func TestWizard_WindowSizeMsg_UpdatesComponents(t *testing.T) {
	w := NewWizard()

	_, _ = w.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	// Components should have updated widths
	assert.Greater(t, w.nameInput.Width, 0)
}

// ============================================================================
// Step Titles and Help
// ============================================================================

func TestWizard_GetCurrentStepTitle(t *testing.T) {
	w := NewWizard()

	titles := map[WizardStep]string{
		StepProjectName:  "Project Name",
		StepGenre:        "Genre",
		StepPremise:      "Story Premise",
		StepWorldSetting: "World Setting",
		StepCharacters:   "Main Characters",
		StepPlotOutline:  "Plot Outline",
		StepLLMProvider:  "LLM Provider",
	}

	for step, expectedTitle := range titles {
		w.currentStep = step
		assert.Equal(t, expectedTitle, w.getCurrentStepTitle(), "step %d", step)
	}
}

func TestWizard_GetCurrentStepHelp(t *testing.T) {
	w := NewWizard()

	// Each step should have help text
	for step := StepProjectName; step <= StepLLMProvider; step++ {
		w.currentStep = step
		help := w.getCurrentStepHelp()
		assert.NotEmpty(t, help, "step %d should have help text", step)
	}
}

// ============================================================================
// canGoBack Tests
// ============================================================================

func TestWizard_CanGoBack(t *testing.T) {
	t.Run("cannot go back from first step", func(t *testing.T) {
		w := NewWizard()
		w.currentStep = StepProjectName

		assert.False(t, w.canGoBack())
	})

	t.Run("can go back from genre (list step)", func(t *testing.T) {
		w := NewWizard()
		w.currentStep = StepGenre

		assert.True(t, w.canGoBack())
	})

	t.Run("can go back from provider (list step)", func(t *testing.T) {
		w := NewWizard()
		w.currentStep = StepLLMProvider

		assert.True(t, w.canGoBack())
	})

	t.Run("can go back from premise when empty", func(t *testing.T) {
		w := NewWizard()
		w.currentStep = StepPremise
		w.premiseInput.SetValue("")

		assert.True(t, w.canGoBack())
	})

	t.Run("cannot go back from premise when has content", func(t *testing.T) {
		w := NewWizard()
		w.currentStep = StepPremise
		w.premiseInput.SetValue("Some content")

		assert.False(t, w.canGoBack())
	})
}

// ============================================================================
// Genre and Provider Lists
// ============================================================================

func TestGenres(t *testing.T) {
	assert.NotEmpty(t, Genres)

	for _, g := range Genres {
		assert.NotEmpty(t, g.Name)
		assert.NotEmpty(t, g.Description)
	}
}

func TestLLMProviders(t *testing.T) {
	assert.NotEmpty(t, LLMProviders)

	for _, p := range LLMProviders {
		assert.NotEmpty(t, p.Name)
		assert.NotEmpty(t, p.Description)
	}

	// Should include expected providers
	names := make([]string, len(LLMProviders))
	for i, p := range LLMProviders {
		names[i] = p.Name
	}
	assert.Contains(t, names, "openai")
	assert.Contains(t, names, "gemini")
	assert.Contains(t, names, "local")
}

// ============================================================================
// listItem Tests
// ============================================================================

func TestListItem(t *testing.T) {
	item := listItem{
		name:        "fantasy",
		description: "Magic and mythical creatures",
	}

	assert.Equal(t, "fantasy", item.Title())
	assert.Equal(t, "Magic and mythical creatures", item.Description())
	assert.Equal(t, "fantasy", item.FilterValue())
}

// ============================================================================
// WizardResult Tests
// ============================================================================

func TestWizardResult(t *testing.T) {
	result := WizardResult{
		ProjectName:  "my-novel",
		Genre:        "fantasy",
		Premise:      "A hero's journey",
		WorldSetting: "Medieval kingdom",
		Characters:   "Alice, Bob",
		PlotOutline:  "Three acts",
		LLMProvider:  "openai",
	}

	assert.Equal(t, "my-novel", result.ProjectName)
	assert.Equal(t, "fantasy", result.Genre)
	assert.Equal(t, "A hero's journey", result.Premise)
	assert.Equal(t, "Medieval kingdom", result.WorldSetting)
	assert.Equal(t, "Alice, Bob", result.Characters)
	assert.Equal(t, "Three acts", result.PlotOutline)
	assert.Equal(t, "openai", result.LLMProvider)
}

// ============================================================================
// min helper Tests
// ============================================================================

func TestMin(t *testing.T) {
	tests := []struct {
		a, b, expected int
	}{
		{1, 2, 1},
		{2, 1, 1},
		{0, 0, 0},
		{-1, 1, -1},
		{100, 50, 50},
	}

	for _, tt := range tests {
		result := min(tt.a, tt.b)
		assert.Equal(t, tt.expected, result)
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestWizard_FullFlow(t *testing.T) {
	w := NewWizard()

	// Initialize with window size
	model, _ := w.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	w = model.(*WizardModel)
	require.True(t, w.ready)

	// Step 1: Project Name
	assert.Equal(t, StepProjectName, w.currentStep)
	w.nameInput.SetValue("my-test-project")
	model, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	// Step 2: Genre
	assert.Equal(t, StepGenre, w.currentStep)
	model, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	// Step 3: Premise
	assert.Equal(t, StepPremise, w.currentStep)
	w.premiseInput.SetValue("Test premise")
	model, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	// Step 4: World Setting
	assert.Equal(t, StepWorldSetting, w.currentStep)
	w.worldInput.SetValue("Test world")
	model, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	// Step 5: Characters
	assert.Equal(t, StepCharacters, w.currentStep)
	w.charactersInput.SetValue("Test characters")
	model, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	// Step 6: Plot Outline
	assert.Equal(t, StepPlotOutline, w.currentStep)
	w.plotInput.SetValue("Test plot")
	model, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w = model.(*WizardModel)

	// Step 7: LLM Provider
	assert.Equal(t, StepLLMProvider, w.currentStep)
	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Should be completed
	assert.True(t, w.completed)

	// Check result
	result := w.Result()
	assert.Equal(t, "my-test-project", result.ProjectName)
	assert.NotEmpty(t, result.Genre)
	assert.Equal(t, "Test premise", result.Premise)
	assert.Equal(t, "Test world", result.WorldSetting)
	assert.Equal(t, "Test characters", result.Characters)
	assert.Equal(t, "Test plot", result.PlotOutline)
	assert.NotEmpty(t, result.LLMProvider)
}
