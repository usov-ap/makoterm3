package ui

import (
	"fmt"
	"strings"

	"makoterm/database"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type State int

const (
	StateNormal State = iota
	StateForm
	StateConfirmDelete
	StateHelp
)

// minFooterWidth is the width below which the footer keeps only the two most
// important hints instead of overflowing.
const minFooterWidth = 40

// pendingDelete holds context for a delete confirmation dialog.
type pendingDelete struct {
	isGroup bool
	group   *database.Group
	host    *database.Host
}

type Model struct {
	miller            MillerColumns
	err               error
	width             int
	height            int
	SelectedToConnect *database.Host
	ShouldQuit        bool

	state   State
	form    Form
	pending *pendingDelete
}

func InitialModel() Model {
	m := Model{
		miller: MillerColumns{
			ActiveCol: 0,
		},
	}
	m.reload()
	return m
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	// ── Help overlay ──
	if m.state == StateHelp {
		if _, ok := msg.(tea.KeyMsg); ok {
			m.state = StateNormal
		}
		return m, nil
	}

	// ── Confirmation dialog ──
	if m.state == StateConfirmDelete {
		if msg, ok := msg.(tea.KeyMsg); ok {
			switch msg.String() {
			case "y", "Y":
				if m.pending != nil {
					var err error
					if m.pending.isGroup {
						err = database.DeleteGroup(m.pending.group)
					} else {
						err = database.DeleteHost(m.pending.host)
					}
					if err != nil {
						m.err = err
					}
					m.pending = nil
					m.reload()
				}
				m.state = StateNormal
			case "n", "N", "esc":
				m.pending = nil
				m.state = StateNormal
			}
		}
		return m, nil
	}

	// ── Form ──
	if m.state == StateForm {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.state = StateNormal
				return m, nil
			case "enter":
				err := m.form.Save()
				if err != nil {
					m.err = err
				} else {
					m.reload()
				}
				m.state = StateNormal
				return m, nil
			}
		}

		m.form, cmd = m.form.Update(msg)
		return m, cmd
	}

	// ── Normal state ──
	switch msg := msg.(type) {

	case tea.KeyMsg:
		// Clear error on any keypress
		if m.err != nil {
			m.err = nil
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "q":
			m.ShouldQuit = true
			return m, tea.Quit

		case "up", "k":
			m.miller.MoveUp()
			m.updateHosts()

		case "down", "j":
			m.miller.MoveDown()
			m.updateHosts()

		case "left", "h":
			m.miller.MoveLeft()

		case "right", "l", "enter":
			if m.miller.ActiveCol == 0 && len(m.miller.Groups) > 0 {
				m.miller.MoveRight()
			} else if m.miller.ActiveCol == 1 && len(m.miller.Hosts) > 0 {
				host := m.miller.SelectedHost()
				if host != nil {
					m.SelectedToConnect = host
					return m, tea.Quit
				}
			}

		case "a":
			// Only open a form when there is a valid target; otherwise the
			// state would flip to a form with no fields.
			if m.miller.ActiveCol == 0 {
				m.form = NewForm(FormTypeGroupAdd, nil, nil)
			} else if grp := m.miller.SelectedGroup(); grp != nil {
				m.form = NewForm(FormTypeHostAdd, grp, nil)
			} else {
				return m, nil
			}
			m.state = StateForm

		case "e":
			if m.miller.ActiveCol == 0 {
				grp := m.miller.SelectedGroup()
				if grp == nil {
					return m, nil
				}
				m.form = NewForm(FormTypeGroupEdit, grp, nil)
			} else if host := m.miller.SelectedHost(); host != nil {
				m.form = NewForm(FormTypeHostEdit, nil, host)
			} else {
				return m, nil
			}
			m.state = StateForm

		case "d":
			if m.miller.ActiveCol == 0 {
				grp := m.miller.SelectedGroup()
				if grp != nil {
					m.pending = &pendingDelete{isGroup: true, group: grp}
					m.state = StateConfirmDelete
				}
			} else if m.miller.ActiveCol == 1 {
				host := m.miller.SelectedHost()
				if host != nil {
					m.pending = &pendingDelete{isGroup: false, host: host}
					m.state = StateConfirmDelete
				}
			}

		case "?":
			m.state = StateHelp
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		m.miller.Width = msg.Width
		m.miller.Height = intMax(0, msg.Height-2) // header bar + footer bar

		m.miller.ClampOffsets()
	}

	return m, nil
}

func (m *Model) updateHosts() {
	group := m.miller.SelectedGroup()
	if group == nil {
		// Always clear the cache: a stale host list would let the user act on
		// a host that no longer belongs to the visible group.
		m.miller.Hosts = nil
		m.miller.HostCursor = 0
		m.miller.HostOffset = 0
		return
	}

	hosts, err := database.GetHostsForGroup(group.ID)
	if err != nil {
		m.err = err
		return
	}
	m.miller.Hosts = hosts

	if m.miller.HostCursor >= len(hosts) {
		m.miller.HostCursor = len(hosts) - 1
	}
	if m.miller.HostCursor < 0 {
		m.miller.HostCursor = 0
	}
	// The list shrank (or the group changed): re-derive the scroll window.
	m.miller.UpdateData(m.miller.Groups, hosts)
}

// reload refreshes the group list and the hosts of the selected group.
func (m *Model) reload() {
	groups, err := database.GetRootGroups()
	if err != nil {
		m.err = err
		return
	}
	m.miller.Groups = groups
	m.miller.clampCursors()
	m.updateHosts()
}

// ── View ────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "" // terminal not yet sized
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	contentHeight := intMax(0, m.height-2)

	var dialog string
	dialogLayer := true

	switch {
	case m.err != nil:
		dialog = m.renderError()
	case m.state == StateForm:
		dialog = m.form.View()
	case m.state == StateConfirmDelete:
		dialog = m.renderConfirmation()
	case m.state == StateHelp:
		dialog = renderHelp()
	default:
		dialogLayer = false
	}

	var mainView string
	if dialogLayer {
		mainView = lipgloss.Place(m.width, contentHeight, lipgloss.Center, lipgloss.Center,
			clipHeight(dialog, contentHeight))
	} else {
		mainView = clipHeight(m.miller.View(), contentHeight)
	}

	// The frame is exactly m.height rows: header + content + footer. Clipping
	// here is the last line of defence against a layout that grows past the
	// terminal and scrolls the header off screen.
	return clipHeight(lipgloss.JoinVertical(lipgloss.Left, header, mainView, footer), m.height)
}

// ── Header bar ──────────────────────────────────────────────────────

func (m Model) renderHeader() string {
	left := AppNameStyle.Render("🦈 MakoTerm")
	right := HeaderHintStyle.Render("? help")

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := m.width - leftW - rightW - 2 // 2 for padding
	if gap < 0 {
		gap = 1
	}

	content := left + strings.Repeat(" ", gap) + right
	return HeaderBarStyle.Width(m.width).Render(content)
}

// ── Footer bar ──────────────────────────────────────────────────────

func (m Model) renderFooter() string {
	var items []string

	switch m.state {
	case StateForm:
		items = []string{
			footerItem("Tab", "next"),
			footerItem("S-Tab", "prev"),
			footerItem("Enter", "save"),
			footerItem("Esc", "cancel"),
		}
	case StateConfirmDelete:
		items = []string{
			footerItem("y", "confirm delete"),
			footerItem("n", "cancel"),
		}
	case StateHelp:
		items = []string{
			footerItem("any key", "close help"),
		}
	default:
		if m.err != nil {
			items = []string{
				footerItem("any key", "dismiss error"),
			}
		} else {
			items = []string{
				footerItem("↑↓", "navigate"),
				footerItem("⏎", "connect"),
				footerItem("a", "add"),
				footerItem("e", "edit"),
				footerItem("d", "delete"),
				footerItem("?", "help"),
				footerItem("q", "quit"),
			}
		}
	}

	// The bar has one column of padding on each side.
	available := intMax(0, m.width-2)
	content := fitItems(items, available, minFooterWidth)
	return FooterBarStyle.Width(m.width).Render(clipHeight(content, 1))
}

func footerItem(key, action string) string {
	return FooterKeyStyle.Render(key) + " " + FooterActionStyle.Render(action)
}

// fitItems joins as many items as fit into width, dropping the least important
// ones from the end. Without this the footer wraps on narrow terminals and
// pushes the layout past the bottom of the screen.
func fitItems(items []string, width, minWidth int) string {
	if width <= 0 {
		return ""
	}
	// Below minWidth show only the first (most important) hint.
	if width < minWidth && len(items) > 1 {
		items = items[:1]
	}

	separator := "   "
	var b strings.Builder
	for _, item := range items {
		candidate := item
		if b.Len() > 0 {
			candidate = separator + item
		}
		if lipgloss.Width(b.String())+lipgloss.Width(candidate) > width {
			break
		}
		b.WriteString(candidate)
	}
	if b.Len() == 0 && len(items) > 0 {
		// Always show something, even if it has to be truncated.
		return truncate(items[0], width)
	}
	return b.String()
}

// clipHeight guarantees a rendered block is at most lines tall. lipgloss
// Height() is a minimum, not a maximum, so any overflow would otherwise push
// the layout past the bottom of the terminal.
func clipHeight(s string, lines int) string {
	if lines <= 0 {
		return ""
	}
	parts := strings.Split(s, "\n")
	if len(parts) <= lines {
		return s
	}
	return strings.Join(parts[:lines], "\n")
}

// ── Help screen ─────────────────────────────────────────────────────

func renderHelp() string {
	var sb strings.Builder

	sb.WriteString(DialogTitleStyle.Render("Keyboard Shortcuts"))

	section := func(name string) {
		sb.WriteString("\n\n")
		sb.WriteString(HelpSectionStyle.Render(name))
		sb.WriteString("\n")
	}

	item := func(key, desc string) {
		sb.WriteString(HelpKeyStyle.Render(key) + HelpDescStyle.Render(desc) + "\n")
	}

	section("Navigation")
	item("↑  k", "Move up")
	item("↓  j", "Move down")
	item("←  h", "Groups column")
	item("→  l", "Hosts column")
	item("Enter", "Connect to host")

	section("Management")
	item("a", "Add group / host")
	item("e", "Edit selected")
	item("d", "Delete selected")

	section("Application")
	item("?", "Toggle help")
	item("q", "Quit")
	item("Ctrl+C", "Force quit")

	return DialogStyle.Render(sb.String())
}

// ── Error dialog ────────────────────────────────────────────────────

func (m Model) renderError() string {
	var sb strings.Builder
	sb.WriteString(DangerTitleStyle.Render("Error"))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("  %v", m.err))
	sb.WriteString("\n\n")
	sb.WriteString(MutedStyle.Render("  Press any key to dismiss"))

	return DangerDialogStyle.Render(sb.String())
}

// ── Confirmation dialog ─────────────────────────────────────────────

func (m Model) renderConfirmation() string {
	var desc string
	if m.pending != nil {
		if m.pending.isGroup {
			desc = fmt.Sprintf("group \"%s\" and all its hosts", m.pending.group.Name)
		} else {
			desc = fmt.Sprintf("host \"%s\"", m.pending.host.Name)
		}
	}

	var sb strings.Builder
	sb.WriteString(DangerTitleStyle.Render("Delete"))
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("  Delete %s?\n\n", desc))
	sb.WriteString(MutedStyle.Render("  This action cannot be undone."))
	sb.WriteString("\n\n")
	sb.WriteString("  " + FooterKeyStyle.Render("y") + " " + FooterActionStyle.Render("confirm") + "    ")
	sb.WriteString(FooterKeyStyle.Render("n") + " " + FooterActionStyle.Render("cancel"))

	return DangerDialogStyle.Render(sb.String())
}
