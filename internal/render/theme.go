package render

import "github.com/charmbracelet/lipgloss"

var (
	ColorPrimary = lipgloss.Color("#58A6FF")
	ColorSuccess = lipgloss.Color("#3FB950")
	ColorWarning = lipgloss.Color("#E3B341")
	ColorDanger  = lipgloss.Color("#F85149")
	ColorMuted   = lipgloss.Color("#8B949E")
	ColorBg      = lipgloss.Color("#161B22")
	ColorText    = lipgloss.Color("#C9D1D9")
)

var (
	Border = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorMuted)

	Card = Border.Padding(0, 1)

	TabActive = lipgloss.NewStyle().
			Foreground(ColorPrimary).
			Bold(true).
			Padding(0, 2)

	TabInactive = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(0, 2)

	TabBar = lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(ColorMuted)

	TopBar = lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		Padding(0, 1)

	StatusBar = lipgloss.NewStyle().
			Foreground(ColorMuted).
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(ColorMuted)

	StatusOK    = lipgloss.NewStyle().Foreground(ColorSuccess)
	StatusWarn  = lipgloss.NewStyle().Foreground(ColorWarning)
	StatusError = lipgloss.NewStyle().Foreground(ColorDanger)
	StatusMute  = lipgloss.NewStyle().Foreground(ColorMuted)
)
