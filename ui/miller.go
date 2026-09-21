package ui

import (
	"fmt"
	"strings"

	"makoterm/database"

	"github.com/charmbracelet/lipgloss"
)

// MillerColumns handles the three-column navigation layout.
type MillerColumns struct {
	Width       int
	Height      int
	Groups      []database.Group
	Hosts       []database.Host
	ActiveCol   int // 0: Groups, 1: Hosts
	GroupCursor int
	HostCursor  int
	GroupOffset int
	HostOffset  int
}

func (m *MillerColumns) UpdateData(groups []database.Group, hosts []database.Host) {
	m.Groups = groups
	m.Hosts = hosts
	m.clampCursors()
}

// ClampOffsets ensures scroll offsets are valid for the current terminal size.
func (m *MillerColumns) ClampOffsets() {
	vh := m.visibleHeight()

	if m.GroupCursor >= m.GroupOffset+vh {
		m.GroupOffset = m.GroupCursor - vh + 1
	}
	if m.GroupOffset < 0 {
		m.GroupOffset = 0
	}

	if m.HostCursor >= m.HostOffset+vh {
		m.HostOffset = m.HostCursor - vh + 1
	}
	if m.HostOffset < 0 {
		m.HostOffset = 0
	}
}

func (m *MillerColumns) MoveUp() {
	if m.ActiveCol == 0 && m.GroupCursor > 0 {
		m.GroupCursor--
		if m.GroupCursor < m.GroupOffset {
			m.GroupOffset = m.GroupCursor
		}
	} else if m.ActiveCol == 1 && m.HostCursor > 0 {
		m.HostCursor--
		if m.HostCursor < m.HostOffset {
			m.HostOffset = m.HostCursor
		}
	}
}

func (m *MillerColumns) MoveDown() {
	vh := m.visibleHeight()

	if m.ActiveCol == 0 && m.GroupCursor < len(m.Groups)-1 {
		m.GroupCursor++
		if m.GroupCursor >= m.GroupOffset+vh {
			m.GroupOffset = m.GroupCursor - vh + 1
		}
	} else if m.ActiveCol == 1 && m.HostCursor < len(m.Hosts)-1 {
		m.HostCursor++
		if m.HostCursor >= m.HostOffset+vh {
			m.HostOffset = m.HostCursor - vh + 1
		}
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

func (m *MillerColumns) visibleHeight() int {
	h := m.Height - 3 // border top/bottom (2) + column header (1)
	if h < 1 {
		h = 1
	}
	return h
}

func (m *MillerColumns) clampCursors() {
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

// View renders the three-column layout. Pure — does not mutate state.
func (m *MillerColumns) View() string {
	if m.Width == 0 || m.Height == 0 {
		return ""
	}

	colWidth := (m.Width - 4) / 3
	vh := m.visibleHeight()

	groupView := m.renderGroupColumn(vh, colWidth)
	hostView := m.renderHostColumn(vh, colWidth)
	detailView := m.renderDetails(colWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top, groupView, hostView, detailView)
}

// ── Groups column ───────────────────────────────────────────────────

func (m *MillerColumns) renderGroupColumn(vh, colWidth int) string {
	isActive := m.ActiveCol == 0
	var lines []string

	lines = append(lines, ColumnHeaderStyle.Render("GROUPS"))

	if len(m.Groups) == 0 {
		lines = append(lines, EmptyStyle.Render("No groups yet.\nPress a to add one."))
	} else {
		if m.GroupOffset > 0 {
			lines = append(lines, ItemStyle.Render("  ↑"))
		}
		for i := m.GroupOffset; i < len(m.Groups) && i < m.GroupOffset+vh; i++ {
			cursor := "   "
			style := ItemStyle
			if i == m.GroupCursor {
				if isActive {
					cursor = " ▸ "
					style = ItemSelectedStyle
				} else {
					cursor = "   "
					style = ItemDimSelectedStyle
				}
			}
			label := cursor + m.Groups[i].Name
			lines = append(lines, style.Width(colWidth-4).Render(label))
		}
		if len(m.Groups) > m.GroupOffset+vh {
			lines = append(lines, ItemStyle.Render("  ↓"))
		}
	}

	colStyle := ColumnStyle.Width(colWidth).Height(m.Height)
	if isActive {
		colStyle = ActiveColumnStyle.Width(colWidth).Height(m.Height)
	}
	return colStyle.Render(strings.Join(lines, "\n"))
}

// ── Hosts column ────────────────────────────────────────────────────

func (m *MillerColumns) renderHostColumn(vh, colWidth int) string {
	isActive := m.ActiveCol == 1
	var lines []string

	lines = append(lines, ColumnHeaderStyle.Render("HOSTS"))

	if len(m.Hosts) == 0 {
		if m.ActiveCol >= 1 {
			lines = append(lines, EmptyStyle.Render("No hosts in group.\nPress a to add one."))
		} else {
			lines = append(lines, EmptyStyle.Render("Select a group."))
		}
	} else {
		if m.HostOffset > 0 {
			lines = append(lines, ItemStyle.Render("  ↑"))
		}
		for i := m.HostOffset; i < len(m.Hosts) && i < m.HostOffset+vh; i++ {
			cursor := "   "
			style := ItemStyle
			if i == m.HostCursor {
				if isActive {
					cursor = " ▸ "
					style = ItemSelectedStyle
				} else {
					cursor = "   "
					style = ItemDimSelectedStyle
				}
			}
			label := cursor + m.Hosts[i].Name
			lines = append(lines, style.Width(colWidth-4).Render(label))
		}
		if len(m.Hosts) > m.HostOffset+vh {
			lines = append(lines, ItemStyle.Render("  ↓"))
		}
	}

	colStyle := ColumnStyle.Width(colWidth).Height(m.Height)
	if isActive {
		colStyle = ActiveColumnStyle.Width(colWidth).Height(m.Height)
	}
	return colStyle.Render(strings.Join(lines, "\n"))
}

// ── Details column ──────────────────────────────────────────────────

func (m *MillerColumns) renderDetails(colWidth int) string {
	var sb strings.Builder

	sb.WriteString(ColumnHeaderStyle.Render("DETAILS"))
	sb.WriteString("\n")

	divW := colWidth - 8
	if divW < 4 {
		divW = 4
	}
	divider := DetailDividerStyle.Render(strings.Repeat("─", divW))

	if m.ActiveCol == 1 {
		host := m.SelectedHost()
		if host != nil {
			// Connection header
			sb.WriteString("\n")
			sb.WriteString(DetailNameStyle.Render("  " + host.Name))
			sb.WriteString("\n")

			connStr := host.User + "@" + host.Address
			if host.Port != 22 {
				connStr += fmt.Sprintf(":%d", host.Port)
			}
			sb.WriteString(DetailConnStyle.Render("  " + connStr))
			sb.WriteString("\n\n")

			// Divider
			sb.WriteString("  " + divider + "\n\n")

			// Detail fields
			line := func(lbl, val string) {
				sb.WriteString("  " + DetailLabelStyle.Render(lbl) + DetailValueStyle.Render(val) + "\n")
			}
			line("User", host.User)
			line("Address", host.Address)
			line("Port", fmt.Sprintf("%d", host.Port))
			if host.KeyPath != "" {
				line("Key", host.KeyPath)
			} else {
				line("Auth", "SSH Agent / Password")
			}

			sb.WriteString("\n  " + divider + "\n\n")

			// Action hint
			sb.WriteString(DetailHintStyle.Render("  ⏎ Enter to connect"))
		}
	} else {
		grp := m.SelectedGroup()
		if grp != nil {
			sb.WriteString("\n")
			sb.WriteString(DetailNameStyle.Render("  " + grp.Name))
			sb.WriteString("\n\n")

			hostCount := len(m.Hosts)
			var countText string
			switch hostCount {
			case 0:
				countText = "No hosts"
			case 1:
				countText = "1 host"
			default:
				countText = fmt.Sprintf("%d hosts", hostCount)
			}
			sb.WriteString(MutedStyle.Render("  " + countText))
			sb.WriteString("\n\n")

			sb.WriteString("  " + divider + "\n\n")
			sb.WriteString(DetailHintStyle.Render("  → to browse hosts"))
		}
	}

	detailStyle := ColumnStyle.Width(colWidth).Height(m.Height)
	return detailStyle.Render(sb.String())
}
