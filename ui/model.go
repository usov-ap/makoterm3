package ui

import (
	"fmt"
	"strings"

	"makoterm/database"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type errMsg error

type State int

const (
	StateNormal State = iota
	StateForm
	StateConfirmDelete
	StateHelp
)

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

	groups, err := database.GetRootGroups()
	if err != nil {
		m.err = err
	}

	var hosts []database.Host
	if len(groups) > 0 {
		hosts, _ = database.GetHostsForGroup(groups[0].ID)
	}

	m.miller.UpdateData(groups, hosts)

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
			if m.miller.ActiveCol == 0 {
				m.form = NewForm(FormTypeGroupAdd, nil, nil)
			} else if m.miller.ActiveCol == 1 {
				grp := m.miller.SelectedGroup()
				if grp != nil {
					m.form = NewForm(FormTypeHostAdd, grp, nil)
				}
			}
			m.state = StateForm

		case "e":
			if m.miller.ActiveCol == 0 {
				grp := m.miller.SelectedGroup()
				if grp != nil {
					m.form = NewForm(FormTypeGroupEdit, grp, nil)
					m.state = StateForm
				}
			} else if m.miller.ActiveCol == 1 {
				host := m.miller.SelectedHost()
				if host != nil {
					m.form = NewForm(FormTypeHostEdit, nil, host)
					m.state = StateForm
				}
			}

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
		m.miller.Height = msg.Height - 2 // header bar + footer bar

		m.miller.ClampOffsets()

	case errMsg:
		m.err = msg
	}

	return m, nil
}

func (m *Model) updateHosts() {
	group := m.miller.SelectedGroup()
	if group != nil {
		hosts, _ := database.GetHostsForGroup(group.ID)
		m.miller.Hosts = hosts

		if m.miller.HostCursor >= len(hosts) {
			m.miller.HostCursor = len(hosts) - 1
		}
		if m.miller.HostCursor < 0 {
			m.miller.HostCursor = 0
		}
	}
}

func (m *Model) reload() {
	groups, _ := database.GetRootGroups()
	m.miller.Groups = groups
	m.updateHosts()
}

// ── View ────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.width == 0 {
		return "" // terminal not yet sized
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	contentHeight := m.height - 2

	var mainView string

	if m.err != nil {
		errDialog := m.renderError()
		mainView = lipgloss.Place(m.width, contentHeight, lipgloss.Center, lipgloss.Center, errDialog)
	} else {
		switch m.state {
		case StateForm:
			formStr := m.form.View()
			mainView = lipgloss.Place(m.width, contentHeight, lipgloss.Center, lipgloss.Center, formStr)
		case StateConfirmDelete:
			confirmStr := m.renderConfirmation()
			mainView = lipgloss.Place(m.width, contentHeight, lipgloss.Center, lipgloss.Center, confirmStr)
		case StateHelp:
			helpStr := renderHelp()
			mainView = lipgloss.Place(m.width, contentHeight, lipgloss.Center, lipgloss.Center, helpStr)
		default:
			mainView = m.miller.View()
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, mainView, footer)
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

	content := strings.Join(items, "   ")
	return FooterBarStyle.Width(m.width).Render(content)
}

func footerItem(key, action string) string {
	return FooterKeyStyle.Render(key) + " " + FooterActionStyle.Render(action)
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
