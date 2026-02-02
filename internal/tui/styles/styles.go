// Package styles provides Lip Gloss styling for the TUI.
package styles

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	Primary     = lipgloss.Color("#7C3AED") // Purple
	Secondary   = lipgloss.Color("#10B981") // Green
	Accent      = lipgloss.Color("#F59E0B") // Amber
	Error       = lipgloss.Color("#EF4444") // Red
	Muted       = lipgloss.Color("#6B7280") // Gray
	Background  = lipgloss.Color("#1F2937") // Dark gray
	Surface     = lipgloss.Color("#374151") // Lighter dark gray
	TextPrimary = lipgloss.Color("#F9FAFB") // Almost white
	TextMuted   = lipgloss.Color("#9CA3AF") // Light gray

	// Base styles
	App = lipgloss.NewStyle().
		Background(Background)

	// Header
	Header = lipgloss.NewStyle().
		Bold(true).
		Foreground(Primary).
		Padding(0, 1).
		MarginBottom(1)

	// Title
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(TextPrimary)

	// Subtitle
	Subtitle = lipgloss.NewStyle().
			Foreground(TextMuted).
			Italic(true)

	// Chat message styles
	UserMessage = lipgloss.NewStyle().
			Foreground(Secondary).
			PaddingLeft(2)

	AssistantMessage = lipgloss.NewStyle().
				Foreground(TextPrimary).
				PaddingLeft(2)

	SystemMessage = lipgloss.NewStyle().
			Foreground(TextMuted).
			Italic(true).
			PaddingLeft(2)

	// Input area
	InputPrompt = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	InputText = lipgloss.NewStyle().
			Foreground(TextPrimary)

	// Status bar
	StatusBar = lipgloss.NewStyle().
			Background(Surface).
			Foreground(TextMuted).
			Padding(0, 1)

	StatusKey = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	StatusValue = lipgloss.NewStyle().
			Foreground(TextPrimary)

	// Error and info messages
	ErrorText = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)

	InfoText = lipgloss.NewStyle().
			Foreground(Accent)

	SuccessText = lipgloss.NewStyle().
			Foreground(Secondary)

	// Help
	HelpKey = lipgloss.NewStyle().
		Foreground(Primary).
		Bold(true)

	HelpDesc = lipgloss.NewStyle().
			Foreground(TextMuted)

	// Borders
	BorderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Surface)

	FocusedBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Primary)

	// List items
	ListItem = lipgloss.NewStyle().
			PaddingLeft(2)

	SelectedItem = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true).
			PaddingLeft(2)

	// Spinner
	Spinner = lipgloss.NewStyle().
		Foreground(Primary)

	// Context indicator
	ContextIndicator = lipgloss.NewStyle().
				Foreground(Accent).
				Bold(true)

	// Token counter
	TokenCounter = lipgloss.NewStyle().
			Foreground(TextMuted).
			Italic(true)

	// Chapter marker
	ChapterMarker = lipgloss.NewStyle().
			Foreground(Secondary).
			Bold(true).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(Secondary).
			MarginTop(1).
			MarginBottom(1)

	// Command
	Command = lipgloss.NewStyle().
		Foreground(Primary).
		Bold(true)

	// Quote block
	Quote = lipgloss.NewStyle().
		Foreground(TextMuted).
		Italic(true).
		Border(lipgloss.ThickBorder(), false, false, false, true).
		BorderForeground(Surface).
		PaddingLeft(1)

	// Muted text style (for using TextMuted color as a style)
	MutedText = lipgloss.NewStyle().
			Foreground(TextMuted)
)

// Width returns the available width for content.
func Width(termWidth int) int {
	return termWidth - 4 // Account for padding
}

// ApplyWidth applies width constraint to a style.
func ApplyWidth(s lipgloss.Style, width int) lipgloss.Style {
	return s.Width(width)
}
