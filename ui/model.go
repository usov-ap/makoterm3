package ui

import (
	"fmt"

	"makoterm/database"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type errMsg error

type State int

const (
	StateNormal State = iota
	StateForm
)

type Model struct {
	miller MillerColumns
	err    error
	width  int
	height int
	SelectedToConnect *database.Host
	ShouldQuit        bool

	state State
	form  Form
}

func InitialModel() Model {
	m := Model{
		miller: MillerColumns{
			ActiveCol: 0,
		},
	}
	
	// Load initial data
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

	switch msg := msg.(type) {
	
	case tea.KeyMsg:
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
				// Connect to host!
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
					database.DeleteGroup(grp)
					m.reload()
				}
			} else if m.miller.ActiveCol == 1 {
				host := m.miller.SelectedHost()
				if host != nil {
					database.DeleteHost(host)
					m.reload()
				}
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		
		// Subtract some header/footer space
		m.miller.Width = msg.Width
		m.miller.Height = msg.Height - 4
		
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
		
		// Ensure HostCursor is valid after reload
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

func (m Model) View() string {
	if m.err != nil {
		return ErrorStyle.Render(fmt.Sprintf("Error: %v", m.err))
	}

	header := TitleStyle.Render("MakoTerm3 - Kanagawa SSH Client")
	footerText := "q/ctrl+c: quit • ↑/↓/k/j: navigate • ←/→/h/l: cols • enter: connect • a: add • e: edit • d: delete"
	if m.state == StateForm {
		footerText = "esc: cancel form • enter: save"
	}
	
	footer := HelpStyle.Render(footerText)

	var mainView string
	if m.state == StateForm {
		// Calculate padding to center the form roughly
		// A simple way is to use lipgloss.Place
		formStr := m.form.View()
		mainView = lipgloss.Place(m.width, m.height-4, lipgloss.Center, lipgloss.Center, formStr)
	} else {
		mainView = m.miller.View()
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"\n",
		mainView,
		"\n",
		footer,
	)
}
