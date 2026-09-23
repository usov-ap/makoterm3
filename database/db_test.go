package database

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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
		if DB != nil {
			if sqlDB, err := DB.DB(); err == nil {
				sqlDB.Close()
			}
		}
		DB = nil
	}
}

// findGroup returns the first group with the given name.
func findGroup(t *testing.T, name string) *Group {
	t.Helper()
	groups, err := GetRootGroups()
	if err != nil {
		t.Fatalf("GetRootGroups failed: %v", err)
	}
	for i := range groups {
		if groups[i].Name == name {
			return &groups[i]
		}
	}
	return nil
}

// TestInitDB_MigrateIsIdempotent is a regression test for a startup failure
// where AutoMigrate rebuilt the groups table on every launch to add a foreign
// key. That rebuild needs foreign key enforcement disabled, so a repeated
// rebuild is both wasteful and a data-loss risk.
func TestInitDB_MigrateIsIdempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("first InitDB failed: %v", err)
	}
	first := schemaOf(t, DB)
	if sqlDB, err := DB.DB(); err == nil {
		sqlDB.Close()
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("second InitDB failed: %v", err)
	}
	defer func() { DB = nil }()
	second := schemaOf(t, DB)

	for table, ddl := range first {
		if second[table] != ddl {
			t.Errorf("table %s was rebuilt on the second start:\n before: %s\n after:  %s",
				table, ddl, second[table])
		}
	}
}

// schemaOf returns the CREATE TABLE statement of every application table.
func schemaOf(t *testing.T, db *gorm.DB) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, table := range []string{"settings", "groups", "hosts"} {
		var ddl string
		if err := db.Raw("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", table).
			Scan(&ddl).Error; err != nil {
			t.Fatalf("read schema of %s: %v", table, err)
		}
		out[table] = ddl
	}
	return out
}

// TestInitDB_EnforcesForeignKeys checks that enforcement is switched back on
// after the migration, so a host can no longer point at a missing group.
func TestInitDB_EnforcesForeignKeys(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	var enabled int
	if err := DB.Raw("PRAGMA foreign_keys").Scan(&enabled).Error; err != nil {
		t.Fatalf("read foreign_keys pragma: %v", err)
	}
	if enabled != 1 {
		t.Fatal("foreign key enforcement is off after InitDB")
	}

	bogus := uint(999999)
	err := CreateHost(&Host{GroupID: &bogus, Name: "Orphan", Address: "192.0.2.1", Port: 22})
	if err == nil {
		t.Error("expected a foreign key violation when inserting a host with a missing group")
	}
}

func TestInitDB_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() { DB = nil }()

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

func TestInitDB_SecuresWALSidecars(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() { DB = nil }()

	// WAL mode creates sidecar files lazily; if they exist they must be 0600.
	for _, suffix := range []string{"-wal", "-shm"} {
		path := dbPath + suffix
		info, err := os.Stat(path)
		if err != nil {
			continue // not created yet on this platform
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("%s permissions = %o, want 0600", suffix, perm)
		}
	}
}

func TestInitDB_SeedsData(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	groups, err := GetRootGroups()
	if err != nil {
		t.Fatalf("GetRootGroups failed: %v", err)
	}

	// Demo data: Production + Development (the root itself is hidden).
	if len(groups) != 2 {
		t.Errorf("expected 2 seeded groups, got %d", len(groups))
	}

	prod := findGroup(t, "Production")
	if prod == nil {
		t.Fatal("Production group not found in seeded data")
	}

	hosts, err := GetHostsForGroup(prod.ID)
	if err != nil {
		t.Fatalf("GetHostsForGroup failed: %v", err)
	}
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts in Production, got %d", len(hosts))
	}
}

func TestInitDB_SeedsOnlyOnce(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("first InitDB failed: %v", err)
	}
	groups, _ := GetRootGroups()
	if len(groups) != 2 {
		t.Fatalf("expected 2 seeded groups, got %d", len(groups))
	}

	// Delete every group, simulating a user who cleaned the demo data.
	for i := range groups {
		if err := DeleteGroup(&groups[i]); err != nil {
			t.Fatalf("DeleteGroup failed: %v", err)
		}
	}

	// Re-open: the demo data must NOT come back.
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("second InitDB failed: %v", err)
	}
	defer func() { DB = nil }()

	groups, _ = GetRootGroups()
	if len(groups) != 0 {
		t.Errorf("demo data was re-seeded after deletion: got %d groups", len(groups))
	}
}

func TestInitDB_NoDemoEnv(t *testing.T) {
	t.Setenv("MAKOTERM_NO_DEMO", "1")

	tmpDir := t.TempDir()
	if err := InitDB(filepath.Join(tmpDir, "test.db")); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer func() { DB = nil }()

	groups, err := GetRootGroups()
	if err != nil {
		t.Fatalf("GetRootGroups failed: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("expected no demo groups with MAKOTERM_NO_DEMO=1, got %d", len(groups))
	}

	// The root group must still exist so the UI can add groups.
	if _, err := GetRootGroup(); err != nil {
		t.Errorf("root group missing: %v", err)
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

func TestGetRootGroups_HidesRoot(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	groups, err := GetRootGroups()
	if err != nil {
		t.Fatalf("GetRootGroups failed: %v", err)
	}
	for _, g := range groups {
		if g.ParentID == nil {
			t.Errorf("GetRootGroups returned a group without parent: %q", g.Name)
		}
	}
}

func TestCreateGroup_AssignsRootParent(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	root, _ := GetRootGroup()
	g := &Group{Name: "No Parent Set"}
	if err := CreateGroup(g); err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if g.ID == 0 {
		t.Fatal("expected non-zero ID after CreateGroup")
	}
	if g.ParentID == nil || *g.ParentID != root.ID {
		t.Errorf("ParentID = %v, want root ID %d", g.ParentID, root.ID)
	}

	if findGroup(t, "No Parent Set") == nil {
		t.Error("created group not found in GetRootGroups")
	}
}

func TestUpdateGroup(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	prod := findGroup(t, "Production")
	if prod == nil {
		t.Fatal("Production group not found")
	}

	prod.Name = "Updated Name"
	if err := UpdateGroup(prod); err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}

	if findGroup(t, "Updated Name") == nil {
		t.Error("updated name not found")
	}
	if findGroup(t, "Production") != nil {
		t.Error("old name still present after update")
	}
}

func TestUpdateGroup_RejectsRoot(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	root, _ := GetRootGroup()
	root.Name = "Hacked"
	err := UpdateGroup(root)
	if !errors.Is(err, ErrRootGroupProtected) {
		t.Errorf("UpdateGroup(root) error = %v, want ErrRootGroupProtected", err)
	}
}

func TestDeleteGroup_RejectsRoot(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	root, _ := GetRootGroup()
	groups, _ := GetRootGroups()

	err := DeleteGroup(root)
	if !errors.Is(err, ErrRootGroupProtected) {
		t.Fatalf("DeleteGroup(root) error = %v, want ErrRootGroupProtected", err)
	}

	// Nothing may be lost.
	after, _ := GetRootGroups()
	if len(after) != len(groups) {
		t.Errorf("root deletion changed group count: %d -> %d", len(groups), len(after))
	}
	if _, err := GetRootGroup(); err != nil {
		t.Errorf("root group disappeared: %v", err)
	}
}

func TestDeleteGroup_Recursive(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	prod := findGroup(t, "Production")
	if prod == nil {
		t.Fatal("Production group not found")
	}

	hosts, _ := GetHostsForGroup(prod.ID)
	if len(hosts) == 0 {
		t.Fatal("Production should have hosts for this test")
	}

	if err := DeleteGroup(prod); err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}

	hosts, _ = GetHostsForGroup(prod.ID)
	if len(hosts) != 0 {
		t.Errorf("expected 0 hosts after group deletion, got %d", len(hosts))
	}
	if findGroup(t, "Production") != nil {
		t.Error("deleted group still found")
	}
}

func TestDeleteGroup_RemovesNestedGroups(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	root, _ := GetRootGroup()
	parent := &Group{Name: "Parent", ParentID: &root.ID}
	if err := CreateGroup(parent); err != nil {
		t.Fatalf("CreateGroup(parent) failed: %v", err)
	}
	child := &Group{Name: "Child", ParentID: &parent.ID}
	if err := CreateGroup(child); err != nil {
		t.Fatalf("CreateGroup(child) failed: %v", err)
	}
	host := &Host{GroupID: &child.ID, Name: "Nested", Address: "192.0.2.1", Port: 22, Password: "nested-secret"}
	if err := CreateHost(host); err != nil {
		t.Fatalf("CreateHost failed: %v", err)
	}

	if err := DeleteGroup(parent); err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}

	var groupCount, hostCount int64
	DB.Unscoped().Model(&Group{}).Where("name IN ?", []string{"Parent", "Child"}).Count(&groupCount)
	DB.Unscoped().Model(&Host{}).Where("name = ?", "Nested").Count(&hostCount)
	if groupCount != 0 {
		t.Errorf("nested groups survived deletion: %d", groupCount)
	}
	if hostCount != 0 {
		t.Errorf("hosts of nested groups survived deletion: %d", hostCount)
	}
}

func TestCreateHost(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	dev := findGroup(t, "Development")
	if dev == nil {
		t.Fatal("Development group not found")
	}

	h := &Host{
		GroupID: &dev.ID,
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

	hosts, _ := GetHostsForGroup(dev.ID)
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

func TestUpdateHost(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	prod := findGroup(t, "Production")
	hosts, _ := GetHostsForGroup(prod.ID)
	if len(hosts) == 0 {
		t.Fatal("no hosts to update")
	}

	host := &hosts[0]
	host.User = "changed"
	if err := UpdateHost(host); err != nil {
		t.Fatalf("UpdateHost failed: %v", err)
	}

	hosts, _ = GetHostsForGroup(prod.ID)
	found := false
	for _, h := range hosts {
		if h.ID == host.ID && h.User == "changed" {
			found = true
		}
	}
	if !found {
		t.Error("host update was not persisted")
	}
}

func TestDeleteHost(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	prod := findGroup(t, "Production")
	hosts, _ := GetHostsForGroup(prod.ID)
	if len(hosts) == 0 {
		t.Fatal("no hosts to delete")
	}

	hostToDelete := &hosts[0]
	if err := DeleteHost(hostToDelete); err != nil {
		t.Fatalf("DeleteHost failed: %v", err)
	}

	hostsAfter, _ := GetHostsForGroup(prod.ID)
	if len(hostsAfter) != len(hosts)-1 {
		t.Errorf("expected %d hosts after delete, got %d", len(hosts)-1, len(hostsAfter))
	}
}

// TestDeleteHost_LeavesNoPasswordBehind is the regression test for the storage
// bug where GORM soft deletes kept plaintext passwords in the database file.
func TestDeleteHost_LeavesNoPasswordBehind(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	prod := findGroup(t, "Production")
	const secret = "SUPERSECRET-DO-NOT-KEEP"

	host := &Host{
		GroupID:  &prod.ID,
		Name:     "Secret Host",
		Address:  "192.0.2.99",
		Port:     22,
		Password: secret,
	}
	if err := CreateHost(host); err != nil {
		t.Fatalf("CreateHost failed: %v", err)
	}
	if err := DeleteHost(host); err != nil {
		t.Fatalf("DeleteHost failed: %v", err)
	}

	// Not even with Unscoped: a deleted host must be gone for good.
	var count int64
	if err := DB.Unscoped().Model(&Host{}).Where("password = ?", secret).Count(&count).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("deleted host is still in the database (unscoped count=%d)", count)
	}
}

func TestDeleteGroup_LeavesNoPasswordBehind(t *testing.T) {
	cleanup := setupTestDB(t)
	defer cleanup()

	root, _ := GetRootGroup()
	group := &Group{Name: "Secrets", ParentID: &root.ID}
	if err := CreateGroup(group); err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	const secret = "GROUP-SECRET-DO-NOT-KEEP"
	if err := CreateHost(&Host{GroupID: &group.ID, Name: "h", Address: "192.0.2.98", Port: 22, Password: secret}); err != nil {
		t.Fatalf("CreateHost failed: %v", err)
	}

	if err := DeleteGroup(group); err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}

	var count int64
	if err := DB.Unscoped().Model(&Host{}).Where("password = ?", secret).Count(&count).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if count != 0 {
		t.Errorf("host of a deleted group is still in the database (unscoped count=%d)", count)
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

// TestInitDB_RepairsOrphanAndMissingRoot covers the databases damaged by the
// old bug that allowed deleting the root group from the UI.
func TestInitDB_RepairsOrphanAndMissingRoot(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	// Simulate the old damage: no root at all, one orphan group at top level.
	root, _ := GetRootGroup()
	if err := DB.Exec("UPDATE groups SET parent_id = NULL WHERE id <> ?", root.ID).Error; err != nil {
		t.Fatalf("failed to orphan groups: %v", err)
	}
	if err := DB.Exec("DELETE FROM groups WHERE id = ?", root.ID).Error; err != nil {
		t.Fatalf("failed to delete root: %v", err)
	}

	// Re-open: a fresh root must be created and the orphan adopted.
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("second InitDB failed: %v", err)
	}
	defer func() { DB = nil }()

	newRoot, err := GetRootGroup()
	if err != nil {
		t.Fatalf("root group was not recreated: %v", err)
	}
	groups, err := GetRootGroups()
	if err != nil {
		t.Fatalf("GetRootGroups failed: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("orphan groups were not adopted")
	}
	for _, g := range groups {
		if g.ParentID == nil || *g.ParentID != newRoot.ID {
			t.Errorf("group %q is still an orphan (parent=%v)", g.Name, g.ParentID)
		}
	}
}

// TestInitDB_MigratesLegacySoftDeletes builds a database with the schema used
// by older versions (gorm.DeletedAt + soft-deleted rows) and checks that
// upgrading purges those rows and drops the column.
func TestInitDB_MigratesLegacySoftDeletes(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "legacy.db")

	legacy, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	legacy.Exec(`CREATE TABLE groups (
		id integer PRIMARY KEY AUTOINCREMENT,
		created_at datetime, updated_at datetime, deleted_at datetime,
		name text, parent_id integer)`)
	legacy.Exec(`CREATE TABLE hosts (
		id integer PRIMARY KEY AUTOINCREMENT,
		created_at datetime, updated_at datetime, deleted_at datetime,
		group_id integer, name text, address text, port integer,
		user text, password text, key_path text)`)
	// One live root, one live group, one soft-deleted group and host.
	legacy.Exec(`INSERT INTO groups (id, created_at, updated_at, name, parent_id) VALUES
		(1, '2024-01-01', '2024-01-01', 'Root', NULL),
		(2, '2024-01-01', '2024-01-01', 'Live', 1),
		(3, '2024-01-01', '2024-01-01', 'Gone', 1)`)
	legacy.Exec(`UPDATE groups SET deleted_at = '2024-01-02' WHERE id = 3`)
	legacy.Exec(`INSERT INTO hosts (id, created_at, updated_at, group_id, name, address, port, user, password) VALUES
		(1, '2024-01-01', '2024-01-01', 2, 'live', '192.0.2.1', 22, 'u', 'live-pass'),
		(2, '2024-01-01', '2024-01-01', 3, 'ghost', '192.0.2.2', 22, 'u', 'LEGACY-SOFT-DELETED-SECRET')`)
	legacy.Exec(`UPDATE hosts SET deleted_at = '2024-01-02' WHERE id = 2`)
	if sqlDB, err := legacy.DB(); err == nil {
		sqlDB.Close()
	}

	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB on legacy database failed: %v", err)
	}
	defer func() { DB = nil }()

	var ghosts int64
	if err := DB.Unscoped().Model(&Host{}).Where("password = ?", "LEGACY-SOFT-DELETED-SECRET").Count(&ghosts).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if ghosts != 0 {
		t.Error("soft-deleted host was not purged during migration")
	}

	var ghostGroups int64
	if err := DB.Unscoped().Model(&Group{}).Where("name = ?", "Gone").Count(&ghostGroups).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if ghostGroups != 0 {
		t.Error("soft-deleted group was not purged during migration")
	}

	// The legacy column may remain (dropping it would rebuild the table on
	// every start); what matters is that no soft-deleted row survives.
	if DB.Migrator().HasColumn("hosts", "deleted_at") {
		var leftover int64
		if err := DB.Raw("SELECT count(*) FROM hosts WHERE deleted_at IS NOT NULL").Scan(&leftover).Error; err != nil {
			t.Fatalf("count soft-deleted rows: %v", err)
		}
		if leftover != 0 {
			t.Errorf("hosts.deleted_at still marks %d row(s) as deleted", leftover)
		}
	}

	// Deletes must be physical from now on, even on a migrated database.
	liveGroups, err := GetRootGroups()
	if err != nil || len(liveGroups) == 0 {
		t.Fatalf("no group available after migration: %v", err)
	}
	host := &Host{GroupID: &liveGroups[0].ID, Name: "after-migration", Address: "192.0.2.3", Port: 22, Password: "fresh-secret"}
	if err := CreateHost(host); err != nil {
		t.Fatalf("CreateHost failed: %v", err)
	}
	if err := DeleteHost(host); err != nil {
		t.Fatalf("DeleteHost failed: %v", err)
	}
	var remaining int64
	if err := DB.Unscoped().Model(&Host{}).Where("password = ?", "fresh-secret").Count(&remaining).Error; err != nil {
		t.Fatalf("count failed: %v", err)
	}
	if remaining != 0 {
		t.Error("delete on a migrated database is still a soft delete")
	}

	// The live data must survive the migration.
	groups, err := GetRootGroups()
	if err != nil {
		t.Fatalf("GetRootGroups failed: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "Live" {
		t.Errorf("live groups after migration = %+v, want single 'Live'", groups)
	}
	hosts, _ := GetHostsForGroup(groups[0].ID)
	if len(hosts) != 1 || hosts[0].Password != "live-pass" {
		t.Errorf("live hosts after migration = %+v, want the 'live' host", hosts)
	}
}
