package contenoxcli

import "github.com/charmbracelet/lipgloss"

var (
	vibeColorUser      = lipgloss.Color("#7DF9FF")
	vibeColorAssistant = lipgloss.Color("#A8FF78")
	vibeColorShell     = lipgloss.Color("#FFD700")
	vibeColorTool      = lipgloss.Color("#C8A2C8")
	vibeColorError     = lipgloss.Color("#FF6B6B")
	vibeColorMuted     = lipgloss.Color("#888888")
	vibeColorBorder    = lipgloss.Color("#333333")
	vibeColorActive    = lipgloss.Color("#A8FF78")
	vibeColorInactive  = lipgloss.Color("#555555")
	vibeColorPending   = lipgloss.Color("#FFD700")
	vibeColorDone      = lipgloss.Color("#A8FF78")
	vibeColorFailed    = lipgloss.Color("#FF6B6B")
	vibeColorSkipped   = lipgloss.Color("#888888")
	vibeColorHITL      = lipgloss.Color("#FF9F1C")
)

var (
	vibeStyleUser  = lipgloss.NewStyle().Foreground(vibeColorUser).Bold(true)
	vibeStyleAI    = lipgloss.NewStyle().Foreground(vibeColorAssistant)
	vibeStyleShell = lipgloss.NewStyle().Foreground(vibeColorShell).Italic(true)
	vibeStyleTool  = lipgloss.NewStyle().Foreground(vibeColorTool).Italic(true)
	vibeStyleError = lipgloss.NewStyle().Foreground(vibeColorError).Bold(true)
	vibeStyleMuted = lipgloss.NewStyle().Foreground(vibeColorMuted)

	vibeStyleBorderBox = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(vibeColorBorder).
				Padding(0, 1)

	vibeStyleHeader = lipgloss.NewStyle().
			Background(lipgloss.Color("#1a1a2e")).
			Foreground(lipgloss.Color("#FFFFFF")).
			Padding(0, 1)

	vibeStyleStatus = lipgloss.NewStyle().
			Background(lipgloss.Color("#16213e")).
			Foreground(vibeColorMuted).
			Padding(0, 1)

	vibeStyleSidebarTitle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Underline(true)
	vibeStyleSidebarSection = lipgloss.NewStyle().Foreground(vibeColorMuted).Bold(true)
	vibeStyleInputPrompt    = lipgloss.NewStyle().Foreground(vibeColorUser).Bold(true)

	vibeStyleHITL = lipgloss.NewStyle().
			Foreground(vibeColorHITL).
			Bold(true).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(vibeColorHITL).
			Padding(1, 2)

	// vibeStyleStatusWorking replaces the idle status bar when the engine is busy.
	vibeStyleStatusWorking = lipgloss.NewStyle().
				Background(lipgloss.Color("#B8860B")).
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Padding(0, 1)
)

func vibePlanStepStyle(status string) lipgloss.Style {
	switch status {
	case "completed":
		return lipgloss.NewStyle().Foreground(vibeColorDone)
	case "failed":
		return lipgloss.NewStyle().Foreground(vibeColorFailed)
	case "skipped":
		return lipgloss.NewStyle().Foreground(vibeColorSkipped)
	default:
		return lipgloss.NewStyle().Foreground(vibeColorPending)
	}
}

func vibeDot(active bool) string {
	if active {
		return lipgloss.NewStyle().Foreground(vibeColorActive).Render("●")
	}
	return lipgloss.NewStyle().Foreground(vibeColorInactive).Render("○")
}
