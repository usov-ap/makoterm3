package ui

import "github.com/charmbracelet/lipgloss"

// ── Kanagawa Color Palette ──────────────────────────────────────────

var (
	// Backgrounds (dark → light)
	SumiInk0 = lipgloss.Color("#16161D")
	SumiInk1 = lipgloss.Color("#1F1F28")
	SumiInk2 = lipgloss.Color("#2A2A37")
	SumiInk3 = lipgloss.Color("#363646")
	SumiInk4 = lipgloss.Color("#54546D") // muted but readable
	WaveBlue = lipgloss.Color("#2D4F67") // subtle selection bg

	// Text
	FujiWhite = lipgloss.Color("#DCD7BA")
	OldWhite  = lipgloss.Color("#C8C093") // secondary text

	// Accents
	CrystalBlue  = lipgloss.Color("#7E9CD8")
	SpringGreen  = lipgloss.Color("#98BB6C")
	AutumnRed    = lipgloss.Color("#C34043")
	SurimiOrange = lipgloss.Color("#FFA066")
	SakuraPink   = lipgloss.Color("#D27E99")
	BoatYellow   = lipgloss.Color("#C0A36E") // warm labels
)

// ── Header & Footer bars ────────────────────────────────────────────

var (
	HeaderBarStyle = lipgloss.NewStyle().
			Background(SumiInk2).
			Padding(0, 1)

	AppNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(CrystalBlue)

	HeaderHintStyle = lipgloss.NewStyle().
			Foreground(SumiInk4)

	FooterBarStyle = lipgloss.NewStyle().
			Background(SumiInk2).
			Padding(0, 1)

	FooterKeyStyle = lipgloss.NewStyle().
			Foreground(CrystalBlue).
			Bold(true)

	FooterActionStyle = lipgloss.NewStyle().
				Foreground(SumiInk4)
)

// ── Columns ─────────────────────────────────────────────────────────

var (
	ColumnHeaderStyle = lipgloss.NewStyle().
				Foreground(SumiInk4).
				Bold(true).
				Padding(0, 1)

	ColumnStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(SumiInk3)

	ActiveColumnStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(CrystalBlue)
)

// ── List Items ──────────────────────────────────────────────────────

var (
	ItemStyle = lipgloss.NewStyle().
			Foreground(FujiWhite).
			Padding(0, 1)

	ItemSelectedStyle = lipgloss.NewStyle().
				Foreground(SumiInk0).
				Background(CrystalBlue).
				Bold(true).
				Padding(0, 1)

	ItemDimSelectedStyle = lipgloss.NewStyle().
				Foreground(FujiWhite).
				Background(WaveBlue).
				Padding(0, 1)

	EmptyStyle = lipgloss.NewStyle().
			Foreground(SumiInk4).
			Italic(true).
			Padding(1, 2)
)

// ── Details Panel ───────────────────────────────────────────────────

var (
	DetailNameStyle = lipgloss.NewStyle().
			Foreground(FujiWhite).
			Bold(true)

	DetailConnStyle = lipgloss.NewStyle().
			Foreground(OldWhite)

	DetailLabelStyle = lipgloss.NewStyle().
				Foreground(BoatYellow).
				Width(12)

	DetailValueStyle = lipgloss.NewStyle().
				Foreground(FujiWhite)

	DetailDividerStyle = lipgloss.NewStyle().
				Foreground(SumiInk3)

	DetailHintStyle = lipgloss.NewStyle().
			Foreground(SumiInk4).
			Italic(true)
)

// ── Dialogs & Overlays ──────────────────────────────────────────────

var (
	DialogStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(CrystalBlue).
			Foreground(FujiWhite).
			Padding(1, 3)

	DangerDialogStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(AutumnRed).
				Foreground(FujiWhite).
				Padding(1, 3)

	DialogTitleStyle = lipgloss.NewStyle().
				Foreground(FujiWhite).
				Bold(true)

	DangerTitleStyle = lipgloss.NewStyle().
				Foreground(AutumnRed).
				Bold(true)
)

// ── Forms ───────────────────────────────────────────────────────────

var (
	FormTitleStyle = lipgloss.NewStyle().
			Foreground(CrystalBlue).
			Bold(true)

	FormLabelStyle = lipgloss.NewStyle().
			Foreground(BoatYellow)
)

// ── Help Screen ─────────────────────────────────────────────────────

var (
	HelpSectionStyle = lipgloss.NewStyle().
				Foreground(CrystalBlue).
				Bold(true)

	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(CrystalBlue).
			Bold(true).
			Width(12).
			Align(lipgloss.Right)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(OldWhite).
			PaddingLeft(2)
)

// ── Common ──────────────────────────────────────────────────────────

var (
	ErrorStyle = lipgloss.NewStyle().
			Foreground(AutumnRed).
			Bold(true)

	MutedStyle = lipgloss.NewStyle().
			Foreground(SumiInk4)

	// TitleStyle kept for backward compatibility
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(CrystalBlue).
			Padding(0, 1)
)
