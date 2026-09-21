package ui

import (
	"os"
	"path/filepath"
	"testing"

	"makoterm/database"

	tea "github.com/charmbracelet/bubbletea"
)

// setupTestDB initializes a temporary database for UI tests.
func setupTestDB(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	return func() {
		database.DB = nil
	}
}

// --- Form validation tests ---

func TestFormSave_GroupAdd_EmptyName(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	form := NewForm(FormTypeGroupAdd, nil, nil)
	// Don't set any value — name is empty
	err := form.Save()
	if err == nil {
		t.Fatal("expected error for empty group name")
	}
}

func TestFormSave_GroupAdd_Valid(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	form := NewForm(FormTypeGroupAdd, nil, nil)
	form.inputs[0].SetValue("My New Group")
	err := form.Save()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormSave_HostAdd_EmptyAddress(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	groups, _ := database.GetRootGroups()
	if len(groups) == 0 {
		t.Fatal("no groups available for test")
	}

	form := NewForm(FormTypeHostAdd, &groups[0], nil)
	form.inputs[0].SetValue("Test Host")
	form.inputs[1].SetValue("") // empty address
	form.inputs[2].SetValue("22")

	err := form.Save()
	if err == nil {
		t.Fatal("expected error for empty address")
	}
}

func TestFormSave_HostAdd_InvalidPort(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	groups, _ := database.GetRootGroups()
	if len(groups) == 0 {
		t.Fatal("no groups available for test")
	}

	tests := []struct {
		name string
		port string
	}{
		{"empty port", ""},
		{"zero port", "0"},
		{"negative port", "-1"},
		{"too large port", "70000"},
		{"non-numeric port", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := NewForm(FormTypeHostAdd, &groups[0], nil)
			form.inputs[0].SetValue("Host")
			form.inputs[1].SetValue("10.0.0.1")
			form.inputs[2].SetValue(tt.port)
			form.inputs[3].SetValue("root")

			err := form.Save()
			if err == nil {
				t.Errorf("expected error for port=%q", tt.port)
			}
		})
	}
}

func TestFormSave_HostAdd_Valid(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	groups, _ := database.GetRootGroups()
	if len(groups) == 0 {
		t.Fatal("no groups available for test")
	}

	form := NewForm(FormTypeHostAdd, &groups[0], nil)
	form.inputs[0].SetValue("Valid Host")
	form.inputs[1].SetValue("10.0.0.1")
	form.inputs[2].SetValue("22")
	form.inputs[3].SetValue("admin")
	form.inputs[4].SetValue("")
	form.inputs[5].SetValue("")

	err := form.Save()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormSave_HostEdit_Valid(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	host := &database.Host{
		Name:    "Original",
		Address: "1.2.3.4",
		Port:    22,
		User:    "admin",
	}
	groups, _ := database.GetRootGroups()
	if len(groups) > 0 {
		host.GroupID = &groups[0].ID
	}
	database.CreateHost(host)

	form := NewForm(FormTypeHostEdit, nil, host)
	form.inputs[0].SetValue("Updated Host")
	form.inputs[1].SetValue("5.6.7.8")
	form.inputs[2].SetValue("2222")
	form.inputs[3].SetValue("newuser")
	form.inputs[4].SetValue("")
	form.inputs[5].SetValue("/home/user/.ssh/id_rsa")

	err := form.Save()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host.Name != "Updated Host" {
		t.Errorf("host name = %q, want %q", host.Name, "Updated Host")
	}
	if host.Port != 2222 {
		t.Errorf("host port = %d, want 2222", host.Port)
	}
	if host.KeyPath != "/home/user/.ssh/id_rsa" {
		t.Errorf("host key path = %q, want %q", host.KeyPath, "/home/user/.ssh/id_rsa")
	}
}

// --- MillerColumns tests ---

func TestMillerColumns_Navigation(t *testing.T) {
	mc := MillerColumns{
		Width:  120,
		Height: 30,
	}
	groups := []database.Group{
		{Name: "Group1"},
		{Name: "Group2"},
		{Name: "Group3"},
	}
	hosts := []database.Host{
		{Name: "Host1"},
		{Name: "Host2"},
	}
	mc.UpdateData(groups, hosts)

	// Initial state
	if mc.GroupCursor != 0 {
		t.Errorf("initial GroupCursor = %d, want 0", mc.GroupCursor)
	}
	if mc.ActiveCol != 0 {
		t.Errorf("initial ActiveCol = %d, want 0", mc.ActiveCol)
	}

	// Move down
	mc.MoveDown()
	if mc.GroupCursor != 1 {
		t.Errorf("after MoveDown: GroupCursor = %d, want 1", mc.GroupCursor)
	}

	// Move down again
	mc.MoveDown()
	if mc.GroupCursor != 2 {
		t.Errorf("after 2x MoveDown: GroupCursor = %d, want 2", mc.GroupCursor)
	}

	// Move down past end — should stay at 2
	mc.MoveDown()
	if mc.GroupCursor != 2 {
		t.Errorf("past end: GroupCursor = %d, want 2", mc.GroupCursor)
	}

	// Move up
	mc.MoveUp()
	if mc.GroupCursor != 1 {
		t.Errorf("after MoveUp: GroupCursor = %d, want 1", mc.GroupCursor)
	}

	// Move right to hosts column
	mc.MoveRight()
	if mc.ActiveCol != 1 {
		t.Errorf("after MoveRight: ActiveCol = %d, want 1", mc.ActiveCol)
	}

	// Move down in hosts column
	mc.MoveDown()
	if mc.HostCursor != 1 {
		t.Errorf("host MoveDown: HostCursor = %d, want 1", mc.HostCursor)
	}

	// Move left back to groups
	mc.MoveLeft()
	if mc.ActiveCol != 0 {
		t.Errorf("after MoveLeft: ActiveCol = %d, want 0", mc.ActiveCol)
	}

	// Move left at leftmost — should stay
	mc.MoveLeft()
	if mc.ActiveCol != 0 {
		t.Errorf("past left: ActiveCol = %d, want 0", mc.ActiveCol)
	}
}

func TestMillerColumns_EmptyState(t *testing.T) {
	mc := MillerColumns{
		Width:  120,
		Height: 30,
	}
	mc.UpdateData(nil, nil)

	if mc.SelectedGroup() != nil {
		t.Error("expected nil SelectedGroup for empty groups")
	}
	if mc.SelectedHost() != nil {
		t.Error("expected nil SelectedHost for empty hosts")
	}

	// MoveDown on empty should not panic
	mc.MoveDown()
	mc.MoveUp()
	mc.MoveRight()
}

func TestMillerColumns_ClampCursors(t *testing.T) {
	mc := MillerColumns{
		Width:       120,
		Height:      30,
		GroupCursor: 10,
		HostCursor:  5,
	}
	groups := []database.Group{{Name: "G1"}, {Name: "G2"}}
	hosts := []database.Host{{Name: "H1"}}
	mc.UpdateData(groups, hosts)

	if mc.GroupCursor != 1 {
		t.Errorf("clamped GroupCursor = %d, want 1", mc.GroupCursor)
	}
	if mc.HostCursor != 0 {
		t.Errorf("clamped HostCursor = %d, want 0", mc.HostCursor)
	}
}

func TestMillerColumns_View_NoZeroSize(t *testing.T) {
	mc := MillerColumns{
		Width:  0,
		Height: 0,
	}
	result := mc.View()
	if result != "" {
		t.Errorf("expected empty string for zero-size, got %q", result)
	}
}

// --- Model state transitions ---

func TestModel_QuitState(t *testing.T) {
	_ = os.Setenv("HOME", t.TempDir()) // prevent touching real DB
	cleanup := setupTestDB(t)
	defer cleanup()

	m := InitialModel()

	// Simulate Ctrl+C
	newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	model := newModel.(Model)

	if !model.ShouldQuit {
		t.Error("expected ShouldQuit to be true after Ctrl+C")
	}
	if cmd == nil {
		t.Error("expected tea.Quit command")
	}
}

func TestModel_DeleteConfirmation(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	m := InitialModel()
	m.width = 120
	m.height = 30
	m.miller.Width = 120
	m.miller.Height = 26

	// Press 'd' to initiate delete (on groups column)
	if len(m.miller.Groups) == 0 {
		t.Skip("no groups to test delete confirmation")
	}

	newModel, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	model := newModel.(Model)

	if model.state != StateConfirmDelete {
		t.Errorf("expected StateConfirmDelete, got %d", model.state)
	}
	if model.pending == nil {
		t.Fatal("expected non-nil pending delete")
	}

	// Press 'n' to cancel
	newModel, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = newModel.(Model)

	if model.state != StateNormal {
		t.Errorf("expected StateNormal after cancel, got %d", model.state)
	}
	if model.pending != nil {
		t.Error("expected nil pending after cancel")
	}
}
