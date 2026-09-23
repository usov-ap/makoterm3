package ui

import (
	"fmt"
	"strings"

	"makoterm/database"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	// minColWidth keeps the three columns usable on a narrow terminal.
	minColWidth = 18

	// columnBorder is the border decoration shared by every column; its width
	// is needed to compute the inner content width.
	columnBorder = "│"

	scrollIndicatorUp   = "  ↑"
	scrollIndicatorDown = "  ↓"
)

// MillerColumns handles the three-column navigation layout.
//
// The active column is [0] GROUPS or [1] HOSTS; [2] DETAILS is read-only.
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
	m.ensureVisible()
}

// ── Geometry ────────────────────────────────────────────────────────

// colWidth returns the width of a single column, borders included.
func (m *MillerColumns) colWidth() int {
	w := (m.Width - 4) / 3
	if w < minColWidth {
		w = minColWidth
	}
	return w
}

// listCapacity returns how many item rows fit in a column.
//
// A column block occupies Height rows in total: top border (1) + header (1) +
// item rows + bottom border (1). So at most Height-3 item rows fit, and up to
// two of them are spent on the scroll indicators.
func (m *MillerColumns) listCapacity() int {
	n := m.Height - 3
	if n < 1 {
		n = 1
	}
	return n
}

// clampOffsets keeps the scroll offsets valid for the current terminal size
// and cursor positions. Called on every resize.
func (m *MillerColumns) ClampOffsets() {
	m.clampCursors()
	m.ensureVisible()
}

// ensureVisible derives the offsets from the cursor positions: the offset is
// never stored state that can drift, only a window onto the list containing
// the cursor. The result is clamped to the end of the list so that a short
// tail does not leave blank rows.
func (m *MillerColumns) ensureVisible() {
	m.GroupOffset = windowStart(m.GroupCursor, len(m.Groups), m.listCapacity(), m.GroupOffset)
	m.HostOffset = windowStart(m.HostCursor, len(m.Hosts), m.listCapacity(), m.HostOffset)
}

// windowStart returns the first visible index of a list such that both the
// cursor and the previous window stay in view.
func windowStart(cursor, total, capacity, previous int) int {
	if total <= 0 || capacity < 1 {
		return 0
	}
	if cursor >= total {
		cursor = total - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	start := previous
	if start < 0 {
		start = 0
	}
	if cursor < start {
		start = cursor
	}
	if cursor > start+capacity-1 {
		start = cursor - capacity + 1
	}

	// When the list does not fill the window, clamp to the last full page so
	// no empty rows are rendered above the tail.
	if maxStart := total - capacity; start > maxStart {
		start = maxStart
	}
	if start < 0 {
		start = 0
	}
	return start
}

func (m *MillerColumns) MoveUp() {
	if m.ActiveCol == 0 && m.GroupCursor > 0 {
		m.GroupCursor--
	} else if m.ActiveCol == 1 && m.HostCursor > 0 {
		m.HostCursor--
	}
	m.ensureVisible()
}

func (m *MillerColumns) MoveDown() {
	if m.ActiveCol == 0 && m.GroupCursor < len(m.Groups)-1 {
		m.GroupCursor++
	} else if m.ActiveCol == 1 && m.HostCursor < len(m.Hosts)-1 {
		m.HostCursor++
	}
	m.ensureVisible()
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

// ── View ────────────────────────────────────────────────────────────

// View renders the three-column layout. It does not mutate state, and the
// result is never taller than Height.
func (m *MillerColumns) View() string {
	if m.Width == 0 || m.Height == 0 {
		return ""
	}

	colWidth := m.colWidth()
	borderWidth := lipgloss.Width(columnBorder)
	contentWidth := colWidth - 2*borderWidth
	if contentWidth < 1 {
		contentWidth = 1
	}

	groupView := m.renderListColumn(listColumn{
		title:    "GROUPS",
		items:    groupNames(m.Groups),
		cursor:   m.GroupCursor,
		offset:   m.GroupOffset,
		active:   m.ActiveCol == 0,
		empty:    "No groups yet.\nPress a to add one.",
		colWidth: colWidth,
		content:  contentWidth,
	})
	hostView := m.renderListColumn(listColumn{
		title:    "HOSTS",
		items:    hostNames(m.Hosts),
		cursor:   m.HostCursor,
		offset:   m.HostOffset,
		active:   m.ActiveCol == 1,
		empty:    m.hostsEmptyMessage(),
		colWidth: colWidth,
		content:  contentWidth,
	})
	detailView := m.renderDetails(colWidth, contentWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top, groupView, hostView, detailView)
}

// listColumn bundles everything renderListColumn needs.
type listColumn struct {
	title    string
	items    []string
	cursor   int
	offset   int
	active   bool
	empty    string
	colWidth int
	content  int
}

func (m *MillerColumns) hostsEmptyMessage() string {
	if m.ActiveCol >= 1 {
		return "No hosts in group.\nPress a to add one."
	}
	return "Select a group."
}

// renderListColumn renders one scrollable list column.
//
// Height discipline: the returned string is exactly m.Height rows tall (or
// fewer on a very short terminal), so three columns joined horizontally can
// never overflow the space the layout reserved for them.
func (m *MillerColumns) renderListColumn(c listColumn) string {
	style := ColumnStyle
	if c.active {
		style = ActiveColumnStyle
	}

	// Rows available for list content, scroll indicators included.
	capacity := m.listCapacity()
	showUp := c.offset > 0
	showDown := c.offset+capacity < len(c.items)

	itemRows := capacity
	if showUp {
		itemRows--
	}
	if showDown {
		itemRows--
	}
	if itemRows < 1 {
		itemRows = 1
	}

	lines := make([]string, 0, capacity+1)
	lines = append(lines, ColumnHeaderStyle.Render(c.title))

	if len(c.items) == 0 {
		lines = append(lines, EmptyStyle.Render(c.empty))
	} else {
		if showUp {
			lines = append(lines, ItemStyle.Render(scrollIndicatorUp))
		}
		for i := c.offset; i < len(c.items) && i < c.offset+itemRows; i++ {
			lines = append(lines, m.renderItem(c, i))
		}
		if showDown {
			lines = append(lines, ItemStyle.Render(scrollIndicatorDown))
		}
	}

	content := strings.Join(lines, "\n")
	if maxHeight := intMax(0, m.Height); maxHeight > 0 {
		content = clipHeight(content, maxHeight)
		// Keep the block rectangular: pad short columns so the three borders
		// align and the box is exactly Height rows tall.
		content = style.Width(c.colWidth).Height(maxHeight).MaxHeight(maxHeight).Render(content)
	}
	return content
}

func (m *MillerColumns) renderItem(c listColumn, i int) string {
	cursor := "   "
	style := ItemStyle
	if i == c.cursor {
		if c.active {
			cursor = " ▸ "
			style = ItemSelectedStyle
		} else {
			style = ItemDimSelectedStyle
		}
	}
	// Leave room for the cursor and the style padding (0,1 → 2 columns).
	return style.Width(c.content).Render(truncate(cursor+c.items[i], c.content))
}

func groupNames(groups []database.Group) []string {
	names := make([]string, len(groups))
	for i, g := range groups {
		names[i] = g.Name
	}
	return names
}

func hostNames(hosts []database.Host) []string {
	names := make([]string, len(hosts))
	for i, h := range hosts {
		names[i] = h.Name
	}
	return names
}

// ── Details column ──────────────────────────────────────────────────

func (m *MillerColumns) renderDetails(colWidth, contentWidth int) string {
	var sb strings.Builder

	sb.WriteString(ColumnHeaderStyle.Render("DETAILS"))
	sb.WriteString("\n")

	dividerWidth := contentWidth - 4
	if dividerWidth < 4 {
		dividerWidth = 4
	}
	divider := DetailDividerStyle.Render(strings.Repeat("─", dividerWidth))

	if m.ActiveCol == 1 {
		host := m.SelectedHost()
		if host != nil {
			sb.WriteString("\n")
			sb.WriteString(DetailNameStyle.Render(truncate("  "+host.Name, contentWidth)))
			sb.WriteString("\n")

			connStr := host.User + "@" + host.Address
			if host.Port != 22 {
				connStr += fmt.Sprintf(":%d", host.Port)
			}
			sb.WriteString(DetailConnStyle.Render(truncate("  "+connStr, contentWidth)))
			sb.WriteString("\n\n")

			sb.WriteString("  " + divider + "\n\n")

			line := func(lbl, val string) {
				valueWidth := intMax(1, contentWidth-2-DetailLabelStyle.GetWidth())
				sb.WriteString("  " + DetailLabelStyle.Render(lbl) +
					DetailValueStyle.Render(truncate(val, valueWidth)) + "\n")
			}
			line("User", host.User)
			line("Address", host.Address)
			line("Port", fmt.Sprintf("%d", host.Port))
			line("Auth", authDescription(host))

			sb.WriteString("\n  " + divider + "\n\n")
			sb.WriteString(DetailHintStyle.Render("  ⏎ Enter to connect"))
		}
	} else {
		grp := m.SelectedGroup()
		if grp != nil {
			sb.WriteString("\n")
			sb.WriteString(DetailNameStyle.Render(truncate("  "+grp.Name, contentWidth)))
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

	height := intMax(0, m.Height)
	if height == 0 {
		return ""
	}
	return ColumnStyle.Width(colWidth).Height(height).MaxHeight(height).Render(clipHeight(sb.String(), height))
}

// authDescription reports how MakoTerm will actually authenticate, instead of
// a generic "SSH Agent / Password" that is shown even when nothing is set.
func authDescription(host *database.Host) string {
	switch {
	case host.KeyPath != "" && host.Password != "":
		return host.KeyPath + " (+password)"
	case host.KeyPath != "":
		return host.KeyPath
	case host.Password != "":
		return "Password (stored)"
	default:
		return "SSH Agent / prompt"
	}
}

// truncate shortens a string to at most width display cells, appending an
// ellipsis when content is lost. Without it, lipgloss wraps long values and
// the column grows past its allocated height.
func truncate(s string, width int) string {
	if width <= 0 || ansi.StringWidth(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	return ansi.Truncate(s, width, "…")
}

func intMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}
