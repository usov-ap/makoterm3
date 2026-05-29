package ui

import "github.com/charmbracelet/lipgloss"

// Kanagawa Theme Colors
var (
	FujiWhite   = lipgloss.Color("#DCD7BA")
	SumiInk0    = lipgloss.Color("#16161D")
	SumiInk1    = lipgloss.Color("#1F1F28")
	SumiInk2    = lipgloss.Color("#2A2A37")
	SumiInk3    = lipgloss.Color("#363646")
	CrystalBlue = lipgloss.Color("#7E9CD8")
	SpringGreen = lipgloss.Color("#98BB6C")
	AutumnRed   = lipgloss.Color("#C34043")
	SakuraPink  = lipgloss.Color("#D27E99")
	SurimiOrange= lipgloss.Color("#FFA066")
	Onyx        = lipgloss.Color("#000000") // Very dark fallback
)

// Global Styles
var (
	BaseStyle = lipgloss.NewStyle().
		Foreground(FujiWhite).
		Background(SumiInk1)

	TitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(CrystalBlue).
		Padding(0, 1)

	SelectedStyle = lipgloss.NewStyle().
		Foreground(SumiInk1).
		Background(CrystalBlue).
		Bold(true)

	NormalItemStyle = lipgloss.NewStyle().
		Foreground(FujiWhite).
		Padding(0, 1)

	BorderColor = SumiInk3

	ColumnStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderColor)
		
	ActiveColumnStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(CrystalBlue)

	HostDetailLabelStyle = lipgloss.NewStyle().
		Foreground(SpringGreen).
		Bold(true).
		Width(12)

	HostDetailValueStyle = lipgloss.NewStyle().
		Foreground(FujiWhite)

	ErrorStyle = lipgloss.NewStyle().
		Foreground(AutumnRed).
		Bold(true)

	HelpStyle = lipgloss.NewStyle().
		Foreground(SumiInk3)
)
