package database

import (
	"os"
	"path/filepath"
	"testing"
)

// setupTestDB creates a temporary database for testing and returns a cleanup function.
func setupTestDB(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	return func() {
		DB = nil
	}
}

func TestInitDB_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	info, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("DB file not created: %v", err)
	}

	// Verify file permissions are 0600 (owner-only)
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("DB file permissions = %o, want 0600", perm)
	}
}

func TestInitDB_SeedsData(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	groups, err := GetRootGroups()
	if err != nil {
		t.Fatalf("GetRootGroups failed: %v", err)
	}

	// Should have at least Root + Production + Development
	if len(groups) < 3 {
		t.Errorf("expected at least 3 seeded groups, got %d", len(groups))
	}

	// Find Production group and check its hosts
	var prodID uint
	for _, g := range groups {
		if g.Name == "Production" {
			prodID = g.ID
			break
		}
	}
	if prodID == 0 {
		t.Fatal("Production group not found in seeded data")
	}

	hosts, err := GetHostsForGroup(prodID)
	if err != nil {
		t.Fatalf("GetHostsForGroup failed: %v", err)
	}
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts in Production, got %d", len(hosts))
	}
}

func TestGetRootGroup(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	root, err := GetRootGroup()
	if err != nil {
		t.Fatalf("GetRootGroup failed: %v", err)
	}
	if root.Name != "Root" {
		t.Errorf("root group name = %q, want %q", root.Name, "Root")
	}
	if root.ParentID != nil {
		t.Errorf("root group should have nil ParentID")
	}
}

func TestCreateGroup(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	root, _ := GetRootGroup()
	g := &Group{Name: "Test Group", ParentID: &root.ID}
	if err := CreateGroup(g); err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if g.ID == 0 {
		t.Error("expected non-zero ID after CreateGroup")
	}

	groups, _ := GetRootGroups()
	found := false
	for _, grp := range groups {
		if grp.Name == "Test Group" {
			found = true
			break
		}
	}
	if !found {
		t.Error("created group not found in GetRootGroups")
	}
}

func TestUpdateGroup(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	groups, _ := GetRootGroups()
	if len(groups) == 0 {
		t.Fatal("no groups to update")
	}

	g := &groups[0]
	originalName := g.Name
	g.Name = "Updated Name"
	if err := UpdateGroup(g); err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}

	groups, _ = GetRootGroups()
	found := false
	for _, grp := range groups {
		if grp.Name == "Updated Name" {
			found = true
		}
		if grp.Name == originalName && grp.ID == g.ID {
			t.Error("old name still present after update")
		}
	}
	if !found {
		t.Error("updated name not found")
	}
}

func TestDeleteGroup_Recursive(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Find Production group (has hosts)
	groups, _ := GetRootGroups()
	var prod *Group
	for i := range groups {
		if groups[i].Name == "Production" {
			prod = &groups[i]
			break
		}
	}
	if prod == nil {
		t.Fatal("Production group not found")
	}

	// Verify it has hosts
	hosts, _ := GetHostsForGroup(prod.ID)
	if len(hosts) == 0 {
		t.Fatal("Production should have hosts for this test")
	}

	// Delete the group
	if err := DeleteGroup(prod); err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}

	// Hosts should be gone
	hosts, _ = GetHostsForGroup(prod.ID)
	if len(hosts) != 0 {
		t.Errorf("expected 0 hosts after group deletion, got %d", len(hosts))
	}

	// Group should be gone
	groups, _ = GetRootGroups()
	for _, g := range groups {
		if g.ID == prod.ID {
			t.Error("deleted group still found")
		}
	}
}

func TestCreateHost(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	groups, _ := GetRootGroups()
	var groupID uint
	for _, g := range groups {
		if g.Name == "Development" {
			groupID = g.ID
			break
		}
	}
	if groupID == 0 {
		t.Fatal("Development group not found")
	}

	h := &Host{
		GroupID: &groupID,
		Name:    "Test Host",
		Address: "10.0.0.1",
		Port:    22,
		User:    "testuser",
	}
	if err := CreateHost(h); err != nil {
		t.Fatalf("CreateHost failed: %v", err)
	}
	if h.ID == 0 {
		t.Error("expected non-zero ID after CreateHost")
	}

	hosts, _ := GetHostsForGroup(groupID)
	found := false
	for _, host := range hosts {
		if host.Name == "Test Host" && host.Address == "10.0.0.1" {
			found = true
		}
	}
	if !found {
		t.Error("created host not found")
	}
}

func TestDeleteHost(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Find a host to delete
	groups, _ := GetRootGroups()
	var groupID uint
	for _, g := range groups {
		if g.Name == "Production" {
			groupID = g.ID
			break
		}
	}

	hosts, _ := GetHostsForGroup(groupID)
	if len(hosts) == 0 {
		t.Fatal("no hosts to delete")
	}

	hostToDelete := &hosts[0]
	if err := DeleteHost(hostToDelete); err != nil {
		t.Fatalf("DeleteHost failed: %v", err)
	}

	hostsAfter, _ := GetHostsForGroup(groupID)
	if len(hostsAfter) != len(hosts)-1 {
		t.Errorf("expected %d hosts after delete, got %d", len(hosts)-1, len(hostsAfter))
	}
}

func TestGetGroupChildren(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	root, _ := GetRootGroup()
	children, err := GetGroupChildren(root.ID)
	if err != nil {
		t.Fatalf("GetGroupChildren failed: %v", err)
	}
	// Root should have Production and Development as children
	if len(children) < 2 {
		t.Errorf("expected at least 2 children of Root, got %d", len(children))
	}
}

func TestGetHostsForGroup_Empty(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	// Create a group with no hosts
	root, _ := GetRootGroup()
	g := &Group{Name: "Empty Group", ParentID: &root.ID}
	CreateGroup(g)

	hosts, err := GetHostsForGroup(g.ID)
	if err != nil {
		t.Fatalf("GetHostsForGroup failed: %v", err)
	}
	if len(hosts) != 0 {
		t.Errorf("expected 0 hosts in empty group, got %d", len(hosts))
	}
}
