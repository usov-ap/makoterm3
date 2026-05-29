package ui

import (
	"fmt"
	"strings"

	"makoterm/database"

	"github.com/charmbracelet/lipgloss"
)

// MillerColumns handles the rendering of the 3 columns
type MillerColumns struct {
	Width       int
	Height      int
	Groups      []database.Group
	Hosts       []database.Host
	ActiveCol   int // 0: Groups, 1: Hosts
	GroupCursor int
	HostCursor  int
}

func (m *MillerColumns) UpdateData(groups []database.Group, hosts []database.Host) {
	m.Groups = groups
	m.Hosts = hosts
	if m.GroupCursor >= len(m.Groups) {
		m.GroupCursor = len(m.Groups) - 1
	}
	if m.GroupCursor < 0 {
		m.GroupCursor = 0
	}
	if m.HostCursor >= len(m.Hosts) {
		m.HostCursor = len(m.Hosts) - 1
	}
	if m.HostCursor < 0 {
		m.HostCursor = 0
	}
}

func (m *MillerColumns) MoveUp() {
	if m.ActiveCol == 0 && m.GroupCursor > 0 {
		m.GroupCursor--
	} else if m.ActiveCol == 1 && m.HostCursor > 0 {
		m.HostCursor--
	}
}

func (m *MillerColumns) MoveDown() {
	if m.ActiveCol == 0 && m.GroupCursor < len(m.Groups)-1 {
		m.GroupCursor++
	} else if m.ActiveCol == 1 && m.HostCursor < len(m.Hosts)-1 {
		m.HostCursor++
	}
}

func (m *MillerColumns) MoveLeft() {
	if m.ActiveCol > 0 {
		m.ActiveCol--
	}
}

func (m *MillerColumns) MoveRight() {
	if m.ActiveCol == 0 && len(m.Groups) > 0 {
		m.ActiveCol++
	}
}

func (m *MillerColumns) SelectedGroup() *database.Group {
	if len(m.Groups) > 0 && m.GroupCursor >= 0 && m.GroupCursor < len(m.Groups) {
		return &m.Groups[m.GroupCursor]
	}
	return nil
}

func (m *MillerColumns) SelectedHost() *database.Host {
	if len(m.Hosts) > 0 && m.HostCursor >= 0 && m.HostCursor < len(m.Hosts) {
		return &m.Hosts[m.HostCursor]
	}
	return nil
}

func (m *MillerColumns) View() string {
	if m.Width == 0 || m.Height == 0 {
		return ""
	}

	colWidth := (m.Width - 4) / 3

	// Column 1: Groups
	var groupItems []string
	for i, g := range m.Groups {
		cursor := "  "
		style := NormalItemStyle
		if i == m.GroupCursor {
			if m.ActiveCol == 0 {
				cursor = "> "
				style = SelectedStyle
			} else {
				style = SelectedStyle.Copy().Background(SumiInk3) // slightly dimmed if inactive
			}
		}
		label := fmt.Sprintf("%s%s", cursor, g.Name)
		groupItems = append(groupItems, style.Width(colWidth-4).Render(label))
	}
	
	groupStyle := ColumnStyle.Copy().Width(colWidth).Height(m.Height)
	if m.ActiveCol == 0 {
		groupStyle = ActiveColumnStyle.Copy().Width(colWidth).Height(m.Height)
	}
	groupView := groupStyle.Render(strings.Join(groupItems, "\n"))

	// Column 2: Hosts
	var hostItems []string
	for i, h := range m.Hosts {
		cursor := "  "
		style := NormalItemStyle
		if i == m.HostCursor {
			if m.ActiveCol == 1 {
				cursor = "> "
				style = SelectedStyle
			} else {
				style = SelectedStyle.Copy().Background(SumiInk3)
			}
		}
		label := fmt.Sprintf("%s%s", cursor, h.Name)
		hostItems = append(hostItems, style.Width(colWidth-4).Render(label))
	}

	hostStyle := ColumnStyle.Copy().Width(colWidth).Height(m.Height)
	if m.ActiveCol == 1 {
		hostStyle = ActiveColumnStyle.Copy().Width(colWidth).Height(m.Height)
	}
	hostView := hostStyle.Render(strings.Join(hostItems, "\n"))

	// Column 3: Details
	var details string
	if m.ActiveCol == 1 {
		selectedHost := m.SelectedHost()
		if selectedHost != nil {
			var sb strings.Builder
			sb.WriteString(TitleStyle.Render("Host Details") + "\n\n")
			
			renderLine := func(lbl, val string) string {
				return HostDetailLabelStyle.Render(lbl) + HostDetailValueStyle.Render(val) + "\n"
			}
			
			sb.WriteString(renderLine("Name:", selectedHost.Name))
			sb.WriteString(renderLine("Address:", selectedHost.Address))
			sb.WriteString(renderLine("Port:", fmt.Sprintf("%d", selectedHost.Port)))
			sb.WriteString(renderLine("User:", selectedHost.User))
			
			details = sb.String()
		}
	} else {
		selectedGroup := m.SelectedGroup()
		if selectedGroup != nil {
			details = TitleStyle.Render(fmt.Sprintf("Group: %s", selectedGroup.Name))
		}
	}

	detailStyle := ColumnStyle.Copy().Width(colWidth).Height(m.Height)
	detailView := detailStyle.Render(details)

	return lipgloss.JoinHorizontal(lipgloss.Top, groupView, hostView, detailView)
}
