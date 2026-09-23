package ui

import (
	"fmt"
	"strings"
	"testing"

	"makoterm/database"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── Layout invariants ───────────────────────────────────────────────
//
// These tests encode the rule that the rendered UI never grows taller than the
// terminal. That was the root cause of several rendering bugs: lipgloss
// Height() is a minimum, so a column with scroll indicators pushed the footer
// off screen.

// sizedModel returns a Model with a database and the given terminal size,
// as if a tea.WindowSizeMsg had just arrived.
func sizedModel(t *testing.T, width, height int) Model {
	t.Helper()
	cleanup := setupTestDB(t)
	t.Cleanup(cleanup)

	m := InitialModel()
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(Model)
}

// manyGroups fills the model's group list with n entries.
func manyGroups(m *Model, n int) {
	groups := make([]database.Group, n)
	for i := range groups {
		groups[i] = database.Group{ID: uint(i + 1), Name: fmt.Sprintf("group-%02d", i)}
	}
	m.miller.Groups = groups
	m.miller.GroupCursor = 0
	m.miller.GroupOffset = 0
	m.miller.UpdateData(groups, m.miller.Hosts)
}

func TestView_NeverExceedsTerminal(t *testing.T) {
	sizes := []struct{ w, h int }{
		{40, 10}, {80, 24}, {120, 30}, {200, 60}, {30, 8}, {20, 5}, {10, 3},
	}

	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.w, size.h), func(t *testing.T) {
			m := sizedModel(t, size.w, size.h)
			manyGroups(&m, 40)

			if got := lipgloss.Height(m.View()); got > size.h {
				t.Errorf("View() height = %d, terminal height = %d", got, size.h)
			}
		})
	}
}

// TestView_ScrollIndicatorsDoNotOverflow is the regression test for the
// original bug: rendering a scrolled list with both ↑ and ↓ indicators made
// the columns one row taller than the space reserved for them.
func TestView_ScrollIndicatorsDoNotOverflow(t *testing.T) {
	for _, height := range []int{8, 12, 20, 24, 40} {
		t.Run(fmt.Sprintf("height-%d", height), func(t *testing.T) {
			m := sizedModel(t, 100, height)
			manyGroups(&m, 50)

			// Scroll to the middle: both indicators are visible.
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
			m = updated.(Model)
			for i := 0; i < 25; i++ {
				updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
				m = updated.(Model)
			}

			if got := lipgloss.Height(m.View()); got > height {
				t.Errorf("scrolled View() height = %d, terminal height = %d", got, height)
			}
		})
	}
}

// TestMillerColumns_WidthIsRespected guards against long host or group names
// wrapping and thereby growing a column vertically.
func TestMillerColumns_WidthIsRespected(t *testing.T) {
	mc := MillerColumns{Width: 90, Height: 20}
	long := database.Group{Name: strings.Repeat("very-long-group-name ", 5)}
	mc.UpdateData([]database.Group{long}, nil)

	for _, line := range strings.Split(mc.View(), "\n") {
		if w := lipgloss.Width(line); w > 90 {
			t.Errorf("line wider than the terminal: %d > 90 (%q)", w, line)
		}
	}
}

// ── Scrolling ───────────────────────────────────────────────────────

// TestMillerColumns_OffsetFollowsCursorUp is the regression test for the
// up-scroll bug: MoveUp moved the cursor but left the offset behind, so the
// selection disappeared above the visible window.
func TestMillerColumns_OffsetFollowsCursorUp(t *testing.T) {
	mc := MillerColumns{Width: 120, Height: 12}
	groups := make([]database.Group, 60)
	for i := range groups {
		groups[i] = database.Group{Name: fmt.Sprintf("g%02d", i)}
	}
	mc.UpdateData(groups, nil)

	// Scroll deep into the list.
	for i := 0; i < 40; i++ {
		mc.MoveDown()
	}
	if mc.GroupOffset == 0 {
		t.Fatal("list did not scroll down")
	}

	// Walk all the way back up: the cursor must stay inside the window.
	for mc.GroupCursor > 0 {
		mc.MoveUp()
		if mc.GroupCursor < mc.GroupOffset {
			t.Fatalf("cursor %d is above the window [%d..]", mc.GroupCursor, mc.GroupOffset)
		}
	}
	if mc.GroupOffset != 0 {
		t.Errorf("offset = %d after scrolling back to the top, want 0", mc.GroupOffset)
	}
}

// TestMillerColumns_ClampOffsetsRecoversAfterResize covers the case where the
// offsets were computed for a small terminal and the window then grew.
func TestMillerColumns_ClampOffsetsRecoversAfterResize(t *testing.T) {
	mc := MillerColumns{Width: 120, Height: 6}
	groups := make([]database.Group, 40)
	for i := range groups {
		groups[i] = database.Group{Name: fmt.Sprintf("g%02d", i)}
	}
	mc.UpdateData(groups, nil)

	for i := 0; i < 30; i++ {
		mc.MoveDown()
	}
	smallOffset := mc.GroupOffset
	if smallOffset == 0 {
		t.Fatal("list did not scroll")
	}

	// The terminal grows a lot: the cursor must still be visible.
	mc.Height = 30
	mc.ClampOffsets()
	if mc.GroupCursor < mc.GroupOffset || mc.GroupCursor >= mc.GroupOffset+mc.listCapacity() {
		t.Errorf("cursor %d outside window [%d..%d] after resize",
			mc.GroupCursor, mc.GroupOffset, mc.GroupOffset+mc.listCapacity()-1)
	}

	// And the offset must not point past the end of the list.
	if maxStart := len(groups) - mc.listCapacity(); mc.GroupOffset > maxStart {
		t.Errorf("offset %d beyond the last full page %d", mc.GroupOffset, maxStart)
	}
}

// TestMillerColumns_NoBlankRowsAtTail checks the window is clamped so the last
// page is full instead of showing empty rows below the final item.
func TestMillerColumns_NoBlankRowsAtTail(t *testing.T) {
	mc := MillerColumns{Width: 120, Height: 12}
	groups := make([]database.Group, 10)
	for i := range groups {
		groups[i] = database.Group{Name: fmt.Sprintf("g%02d", i)}
	}
	mc.UpdateData(groups, nil)
	for i := 0; i < len(groups)-1; i++ {
		mc.MoveDown()
	}

	view := mc.View()
	if !strings.Contains(view, "g09") {
		t.Error("last group is not visible after scrolling to the end")
	}
	if mc.GroupOffset+mc.listCapacity() < len(groups) {
		t.Errorf("offset %d leaves blank rows below the tail", mc.GroupOffset)
	}
}

// ── Footer ──────────────────────────────────────────────────────────

func TestRenderFooter_FitsNarrowTerminal(t *testing.T) {
	for _, width := range []int{20, 30, 40, 60, 80, 120} {
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			m := sizedModel(t, width, 20)
			footer := m.renderFooter()
			if w := lipgloss.Width(footer); w > width {
				t.Errorf("footer width = %d, terminal width = %d", w, width)
			}
			if h := lipgloss.Height(footer); h != 1 {
				t.Errorf("footer height = %d, want 1", h)
			}
		})
	}
}

// ── Form ────────────────────────────────────────────────────────────

// TestForm_UpdateWithNoFields guards the modulo-by-zero panic that a form
// without inputs would trigger.
func TestForm_UpdateWithNoFields(t *testing.T) {
	var form Form
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Form.Update panicked with no inputs: %v", r)
		}
	}()
	updated, cmd := form.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cmd != nil {
		t.Error("expected no command for an empty form")
	}
	if len(updated.inputs) != 0 {
		t.Error("empty form gained inputs")
	}
}

// TestModel_AddWithoutTargetStaysNormal checks that pressing 'a' where there is
// no valid target does not open a form. The UI normally prevents reaching the
// hosts column without a group, so the state is forced: the point is that the
// key handler itself must not create an unusable form.
func TestModel_AddWithoutTargetStaysNormal(t *testing.T) {
	m := sizedModel(t, 100, 24)
	// No groups at all, but the cursor sits in the hosts column.
	m.miller.Groups = nil
	m.miller.Hosts = nil
	m.miller.ActiveCol = 1
	m.miller.ClampOffsets()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	got := updated.(Model)
	if got.state == StateForm {
		t.Errorf("'a' with no selected group opened a form with %d fields", len(got.form.inputs))
	}
	if len(got.miller.Hosts) != 0 {
		t.Error("expected no hosts")
	}

	// Same for 'e'.
	updated, _ = got.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if got := updated.(Model); got.state == StateForm {
		t.Error("'e' with no selected host opened a form")
	}
}
