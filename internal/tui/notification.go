package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/ansi"
	"github.com/muesli/reflow/truncate"
)

type ToastLevel int

const (
	ToastInfo ToastLevel = iota
	ToastSuccess
	ToastWarning
	ToastError
)

type Toast struct {
	Message string
	Level   ToastLevel
	Visible bool
}

type clearToastMsg struct{}

var (
	toastBaseStyle = lipgloss.NewStyle().
			Padding(0, 1).
			BorderStyle(lipgloss.RoundedBorder())

	toastInfoStyle = toastBaseStyle.
			BorderForeground(lipgloss.Color("#00D7FF")).
			Foreground(lipgloss.Color("#00D7FF"))

	toastSuccessStyle = toastBaseStyle.
				BorderForeground(lipgloss.Color("#10B981")).
				Foreground(lipgloss.Color("#10B981"))

	toastWarningStyle = toastBaseStyle.
				BorderForeground(lipgloss.Color("#F59E0B")).
				Foreground(lipgloss.Color("#F59E0B"))

	toastErrorStyle = toastBaseStyle.
			BorderForeground(lipgloss.Color("#EF4444")).
			Foreground(lipgloss.Color("#EF4444"))
)

func (t Toast) getIcon() string {
	switch t.Level {
	case ToastSuccess:
		return "✓"
	case ToastError:
		return "✗"
	case ToastWarning:
		return "⚠"
	default:
		return "ℹ"
	}
}

func (t Toast) getStyle() lipgloss.Style {
	switch t.Level {
	case ToastSuccess:
		return toastSuccessStyle
	case ToastError:
		return toastErrorStyle
	case ToastWarning:
		return toastWarningStyle
	default:
		return toastInfoStyle
	}
}

func showToast(msg string, level ToastLevel, duration time.Duration) (Toast, tea.Cmd) {
	return Toast{
			Message: msg,
			Level:   level,
			Visible: true,
		},
		tea.Tick(duration, func(time.Time) tea.Msg {
			return clearToastMsg{}
		})
}

func (t *Toast) Update(msg tea.Msg) {
	if _, ok := msg.(clearToastMsg); ok {
		t.Visible = false
		t.Message = ""
	}
}

func (t Toast) View(maxWidth int) string {
	if !t.Visible || t.Message == "" {
		return ""
	}

	icon := t.getIcon()
	style := t.getStyle()

	msg := t.Message
	if maxWidth > 0 && len(msg) > maxWidth-10 {
		msg = msg[:maxWidth-13] + "..."
	}

	content := icon + " " + msg
	return style.Render(content)
}

func getLines(s string) (lines []string, widest int) {
	lines = strings.Split(s, "\n")
	for _, l := range lines {
		w := ansi.PrintableRuneWidth(l)
		if widest < w {
			widest = w
		}
	}
	return lines, widest
}

func placeOverlay(x, y int, fg, bg string) string {
	fgLines, fgWidth := getLines(fg)
	bgLines, bgWidth := getLines(bg)
	bgHeight := len(bgLines)
	fgHeight := len(fgLines)

	if fgWidth >= bgWidth && fgHeight >= bgHeight {
		return fg
	}

	if x < 0 {
		x = 0
	}
	if x > bgWidth-fgWidth {
		x = bgWidth - fgWidth
	}
	if y < 0 {
		y = 0
	}
	if y > bgHeight-fgHeight {
		y = bgHeight - fgHeight
	}

	var b strings.Builder
	for i, bgLine := range bgLines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if i < y || i >= y+fgHeight {
			b.WriteString(bgLine)
			continue
		}

		pos := 0
		if x > 0 {
			left := truncateString(bgLine, x)
			pos = ansi.PrintableRuneWidth(left)
			b.WriteString(left)
			if pos < x {
				b.WriteString(strings.Repeat(" ", x-pos))
				pos = x
			}
		}

		fgLine := fgLines[i-y]
		b.WriteString(fgLine)
		pos += ansi.PrintableRuneWidth(fgLine)

		bgLineWidth := ansi.PrintableRuneWidth(bgLine)
		if pos < bgLineWidth {
			right := truncateLeftString(bgLine, pos)
			b.WriteString(right)
		}
	}

	return b.String()
}

func truncateString(s string, maxWidth int) string {
	return truncate.String(s, uint(maxWidth))
}

func truncateLeftString(s string, skip int) string {
	width := 0
	for i, r := range s {
		if width >= skip {
			return s[i:]
		}
		width += ansi.PrintableRuneWidth(string(r))
	}
	return ""
}

func renderToastTopRight(toast, background string, padding int) string {
	if toast == "" {
		return background
	}

	toastWidth := lipgloss.Width(toast)
	bgWidth := lipgloss.Width(background)

	x := bgWidth - toastWidth - padding
	y := padding

	return placeOverlay(x, y, toast, background)
}
