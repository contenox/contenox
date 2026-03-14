package contenoxcli

import "github.com/charmbracelet/lipgloss"

// ─── Brand palette ────────────────────────────────────────────────────────────
// Primary accent: Contenox emerald green (#4ADE80 / #34D399)
// Everything derives from this single decision.

var (
	// Semantic role colors
	vibeColorUser      = lipgloss.Color("#34D399") // emerald-400 — user prompt / input indicator
	vibeColorAssistant = lipgloss.Color("#4ADE80") // emerald-300 — AI response text
	vibeColorShell     = lipgloss.Color("#86EFAC") // emerald-200 — shell output (lighter, readable)
	vibeColorTool      = lipgloss.Color("#6EE7B7") // emerald-300 mid — tool calls
	vibeColorError     = lipgloss.Color("#FF6B6B") // red — error (semantic, keep)
	vibeColorMuted     = lipgloss.Color("#6B7280") // cool gray-500 — muted text
	vibeColorBorder    = lipgloss.Color("#1F3D2A") // dark emerald — border
	vibeColorActive    = lipgloss.Color("#4ADE80") // emerald — active item
	vibeColorInactive  = lipgloss.Color("#374151") // gray-700 — inactive
	vibeColorPending   = lipgloss.Color("#FCD34D") // amber-300 — pending step (keep warm)
	vibeColorDone      = lipgloss.Color("#4ADE80") // emerald — completed step
	vibeColorFailed    = lipgloss.Color("#FF6B6B") // red — failed (semantic, keep)
	vibeColorSkipped   = lipgloss.Color("#6B7280") // gray — skipped
	vibeColorHITL      = lipgloss.Color("#FB923C") // orange-400 — human-in-the-loop (keep)
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

	// Header bar: near-black with a dark emerald tint — hacker terminal feel
	vibeStyleHeader = lipgloss.NewStyle().
			Background(lipgloss.Color("#0d1a12")).
			Foreground(lipgloss.Color("#D1FAE5")).
			Padding(0, 1)

	// Status bar: slightly lighter dark green baseline
	vibeStyleStatus = lipgloss.NewStyle().
			Background(lipgloss.Color("#0f2318")).
			Foreground(vibeColorMuted).
			Padding(0, 1)

	vibeStyleSidebarTitle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#D1FAE5")).Bold(true).Underline(true)
	vibeStyleSidebarSection = lipgloss.NewStyle().Foreground(vibeColorMuted).Bold(true)
	vibeStyleInputPrompt    = lipgloss.NewStyle().Foreground(vibeColorUser).Bold(true)

	vibeStyleHITL = lipgloss.NewStyle().
			Foreground(vibeColorHITL).
			Bold(true).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(vibeColorHITL).
			Padding(1, 2)

	// Status working bar: dark amber-on-emerald-dark — visible but not jarring
	vibeStyleStatusWorking = lipgloss.NewStyle().
				Background(lipgloss.Color("#92400E")).
				Foreground(lipgloss.Color("#FDE68A")).
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
