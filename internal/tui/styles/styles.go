package styles

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	// Palette Colors
	ColorPrimary   = lipgloss.Color("#7D56F4") // Vibrant Purple
	ColorSecondary = lipgloss.Color("#04B575") // Cyan Emerald
	ColorAccent    = lipgloss.Color("#FF76A0") // Coral Pink
	ColorBgDark    = lipgloss.Color("#1A1B26") // Soft Dark
	ColorCardBg    = lipgloss.Color("#24283B") // Card Background
	ColorText      = lipgloss.Color("#C0CAF5") // Crisp Off-white
	ColorMuted     = lipgloss.Color("#565F89") // Muted Gray
	ColorHighlight = lipgloss.Color("#E0AF68") // Warm Amber
	ColorError     = lipgloss.Color("#F7768E") // Soft Red

	// Base Styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorPrimary).
			Padding(0, 1)

	CardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorMuted).
			Padding(0, 1).
			MarginBottom(1)

	ActiveCardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorSecondary).
			Padding(0, 1).
			MarginBottom(1)

	BadgeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorMuted).
			Padding(0, 1).
			Bold(true)

	BadgeSuccessStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(ColorSecondary).
				Padding(0, 1).
				Bold(true)

	BadgeAccentStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(ColorAccent).
				Padding(0, 1).
				Bold(true)

	HelpStyle = lipgloss.NewStyle().
			Foreground(ColorMuted).
			MarginTop(1)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Bold(true)

	MetricLabelStyle = lipgloss.NewStyle().
				Foreground(ColorMuted).
				Bold(true)

	MetricValueStyle = lipgloss.NewStyle().
				Foreground(ColorHighlight).
				Bold(true)
)
