// Package database stores MakoTerm groups, hosts and credentials in a local
// SQLite file (by default ~/.makoterm.db) using GORM.
//
// Security notes:
//   - Deletes are physical (Unscoped). A deleted host must not leave its
//     password behind in the database file.
//   - Those passwords are stored in plaintext; the only protection is the file
//     mode (0600). Prefer SSH keys.
package database

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DB is the process-wide database handle. It is initialised once by InitDB.
//
// Deprecated: prefer passing *gorm.DB explicitly. It is kept as a package
// variable because the UI layer calls the helper functions below directly.
var DB *gorm.DB

// ErrRootGroupProtected is returned when a caller tries to delete the root
// group, which owns every other group and must always survive.
var ErrRootGroupProtected = errors.New("the root group cannot be deleted")

// defaultDBName is the database file created in the user's home directory.
const defaultDBName = ".makoterm.db"

// dsnOptions are SQLite pragmas applied on every connection.
//
//   - busy_timeout  — wait instead of failing with "database is locked" when a
//     second MakoTerm instance touches the file.
//   - journal_mode  — WAL is more crash-resistant and allows concurrent reads.
//   - secure_delete — overwrite deleted content with zeros, so plaintext
//     passwords do not survive in free pages.
//
// foreign_keys is deliberately NOT set here: AutoMigrate rebuilds tables to add
// constraints, and SQLite fails that copy while enforcement is on. migrate()
// enables it explicitly once the schema is up to date.
const dsnOptions = "?_busy_timeout=5000&_journal_mode=WAL&_secure_delete=on"

// InitDB initializes the SQLite database.
// If dbPath is empty, defaults to ~/.makoterm.db
func InitDB(dbPath string) error {
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home directory: %w", err)
		}
		dbPath = filepath.Join(home, defaultDBName)
	}

	db, err := gorm.Open(sqlite.Open(dbPath+dsnOptions), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("access database handle: %w", err)
	}
	// A single connection keeps PRAGMAs (foreign_keys) effective and avoids
	// "database is locked" between pooled connections. The application is
	// single-threaded and the workload is tiny, so there is no downside.
	sqlDB.SetMaxOpenConns(1)

	// Make sure the file exists on disk before tightening its permissions.
	if err := db.Exec("SELECT 1").Error; err != nil {
		return fmt.Errorf("probe database: %w", err)
	}

	// Restrict permissions BEFORE any schema or data is written: the database
	// may contain plaintext passwords.
	if err := secureDatabaseFiles(dbPath); err != nil {
		return err
	}

	if err := migrate(db); err != nil {
		return err
	}

	DB = db

	if err := ensureRootGroup(db); err != nil {
		return err
	}

	if err := seedIfEmpty(db); err != nil {
		return err
	}

	return nil
}

// migrate brings the schema up to date and cleans up rows left behind by
// versions that used GORM soft deletes.
//
// The phases must stay in this order:
//  1. remove rows that older versions only marked as deleted — they are
//     invisible to the current queries but still contain host passwords;
//  2. AutoMigrate, which adds foreign keys. SQLite rebuilds a table to add a
//     constraint and refuses the row copy while enforcement is on, so foreign
//     keys are disabled for the duration and enabled again at the end.
//
// Obsolete deleted_at columns are intentionally left in place: GORM ignores
// unmapped columns once they are NULL, and dropping them would mean rebuilding
// the tables at every start, which risks user data for no security gain.
func migrate(db *gorm.DB) error {
	if err := purgeLegacySoftDeletedRows(db); err != nil {
		return err
	}

	if err := setForeignKeys(db, false); err != nil {
		return err
	}
	migrateErr := db.AutoMigrate(&Setting{}, &Group{}, &Host{})
	// Re-enable enforcement even when the migration fails, and report the
	// migration problem first: it is the interesting one.
	restoreErr := setForeignKeys(db, true)

	if migrateErr != nil {
		return fmt.Errorf("migrate schema: %w", migrateErr)
	}
	return restoreErr
}

// setForeignKeys toggles SQLite foreign key enforcement on the connection.
// It requires a single pooled connection, which InitDB guarantees.
func setForeignKeys(db *gorm.DB, on bool) error {
	value := "OFF"
	if on {
		value = "ON"
	}
	if err := db.Exec("PRAGMA foreign_keys = " + value).Error; err != nil {
		return fmt.Errorf("set foreign_keys %s: %w", value, err)
	}
	return nil
}

// purgeLegacySoftDeletedRows physically removes rows that older versions only
// marked as deleted (deleted_at IS NOT NULL).
func purgeLegacySoftDeletedRows(db *gorm.DB) error {
	for _, table := range []string{"groups", "hosts"} {
		if !db.Migrator().HasColumn(table, "deleted_at") {
			continue
		}

		res := db.Exec("DELETE FROM " + table + " WHERE deleted_at IS NOT NULL")
		if res.Error != nil {
			return fmt.Errorf("purge soft-deleted rows from %s: %w", table, res.Error)
		}
		if res.RowsAffected > 0 {
			fmt.Fprintf(os.Stderr,
				"MakoTerm: removed %d previously soft-deleted row(s) from %s (they still contained stored credentials)\n",
				res.RowsAffected, table)
		}
	}
	return nil
}

// secureDatabaseFiles sets 0600 on the database file and its WAL sidecars, then
// deletes them after they are removed (see vacuum).
func secureDatabaseFiles(dbPath string) error {
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if _, err := os.Stat(path); err != nil {
			continue // not created yet
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("set permissions on %s: %w", path, err)
		}
	}
	return nil
}

// ensureRootGroup guarantees that exactly one root group (ParentID == nil)
// exists and adopts orphan groups, e.g. after a group tree was deleted by an
// older version that allowed deleting the root.
func ensureRootGroup(db *gorm.DB) error {
	var roots []Group
	if err := db.Where("parent_id IS NULL").Order("id").Find(&roots).Error; err != nil {
		return fmt.Errorf("find root group: %w", err)
	}

	switch len(roots) {
	case 0:
		root := Group{Name: "Root"}
		if err := db.Create(&root).Error; err != nil {
			return fmt.Errorf("create root group: %w", err)
		}
		// Adopt every orphan left behind.
		if err := db.Model(&Group{}).Where("parent_id IS NULL AND id <> ?", root.ID).
			Update("parent_id", root.ID).Error; err != nil {
			return fmt.Errorf("adopt orphan groups: %w", err)
		}
		return nil
	case 1:
		return nil
	default:
		// Several roots (possible with data written by other tools): keep the
		// oldest and attach the rest to it.
		root := roots[0]
		if err := db.Model(&Group{}).Where("parent_id IS NULL AND id <> ?", root.ID).
			Update("parent_id", root.ID).Error; err != nil {
			return fmt.Errorf("merge extra root groups: %w", err)
		}
		return nil
	}
}

// seedIfEmpty creates the demo tree the first time the application runs.
//
// Seeding happens at most once per database, tracked by a marker row, so a
// user who deletes every group does not get the demo data back on the next
// start.
//
// The seeded entries are deliberately generic: documentation addresses
// (RFC 5737) and no credentials. Delete them from the UI, or skip seeding
// entirely with MAKOTERM_NO_DEMO=1.
func seedIfEmpty(db *gorm.DB) error {
	seeded, err := demoSeeded(db)
	if err != nil {
		return err
	}
	if seeded || os.Getenv("MAKOTERM_NO_DEMO") != "" {
		return nil
	}

	// Databases created by older versions have no marker. If they already
	// contain user groups, mark them as seeded instead of adding demo data.
	var groupCount int64
	if err := db.Model(&Group{}).Count(&groupCount).Error; err != nil {
		return fmt.Errorf("count groups: %w", err)
	}
	if groupCount > 1 {
		return settingSet(db, settingDemoSeeded, "1")
	}

	root, err := getRootGroup(db)
	if err != nil {
		return err
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		prod := Group{Name: "Production", ParentID: &root.ID}
		dev := Group{Name: "Development", ParentID: &root.ID}
		if err := tx.Create(&prod).Error; err != nil {
			return err
		}
		if err := tx.Create(&dev).Error; err != nil {
			return err
		}

		hosts := []Host{
			{GroupID: &prod.ID, Name: "Web Server", Address: "192.0.2.10", Port: 22, User: "admin"},
			{GroupID: &prod.ID, Name: "DB Server", Address: "192.0.2.11", Port: 22, User: "dbadmin"},
			{GroupID: &dev.ID, Name: "Staging", Address: "192.0.2.20", Port: 2222, User: "deploy"},
		}
		if err := tx.Create(&hosts).Error; err != nil {
			return err
		}
		return settingSet(tx, settingDemoSeeded, "1")
	})
	if err != nil {
		return fmt.Errorf("seed demo data: %w", err)
	}
	return nil
}

// GetRootGroup returns the root group (the one with no parent).
func GetRootGroup() (*Group, error) {
	return getRootGroup(DB)
}

func getRootGroup(db *gorm.DB) (*Group, error) {
	var root Group
	if err := db.Where("parent_id IS NULL").Order("id").First(&root).Error; err != nil {
		return nil, fmt.Errorf("find root group: %w", err)
	}
	return &root, nil
}

// GetRootGroups returns the top-level groups visible in the UI.
//
// The root group itself is hidden: it is an implementation detail and must not
// be renamed or deleted from the UI.
func GetRootGroups() ([]Group, error) {
	root, err := GetRootGroup()
	if err != nil {
		return nil, err
	}
	var groups []Group
	if err := DB.Where("parent_id = ?", root.ID).Order("name").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

// GetAllGroups returns every group except the root, including nested ones.
// Used by tests and diagnostics.
func GetAllGroups() ([]Group, error) {
	var groups []Group
	err := DB.Where("parent_id IS NOT NULL").Order("id").Find(&groups).Error
	return groups, err
}

// GetGroupChildren returns subgroups for a given group ID.
func GetGroupChildren(parentID uint) ([]Group, error) {
	var groups []Group
	err := DB.Where("parent_id = ?", parentID).Find(&groups).Error
	return groups, err
}

// GetHostsForGroup returns hosts for a given group ID.
func GetHostsForGroup(groupID uint) ([]Host, error) {
	var hosts []Host
	err := DB.Where("group_id = ?", groupID).Find(&hosts).Error
	return hosts, err
}

// CreateGroup saves a new group.
func CreateGroup(g *Group) error {
	if g.ParentID == nil {
		root, err := GetRootGroup()
		if err != nil {
			return err
		}
		g.ParentID = &root.ID
	}
	return DB.Create(g).Error
}

// UpdateGroup updates an existing group.
func UpdateGroup(g *Group) error {
	if g.ParentID == nil {
		return ErrRootGroupProtected
	}
	return DB.Save(g).Error
}

// DeleteGroup deletes a group, its hosts, and all nested subgroups.
// Deletion is physical, and the free pages are zeroed afterwards so that
// stored passwords cannot be recovered from the file.
func DeleteGroup(g *Group) error {
	if err := deleteGroup(DB, g); err != nil {
		return err
	}
	return vacuum()
}

func deleteGroup(db *gorm.DB, g *Group) error {
	// A group without a parent is the root; refuse to touch it.
	if g.ParentID == nil {
		return ErrRootGroupProtected
	}
	if root, err := getRootGroup(db); err == nil && g.ID == root.ID {
		return ErrRootGroupProtected
	}

	return db.Transaction(func(tx *gorm.DB) error {
		return deleteGroupRecursive(tx, g.ID)
	})
}

// deleteGroupRecursive removes a group and all its descendants. It runs inside
// a transaction opened by deleteGroup.
func deleteGroupRecursive(tx *gorm.DB, groupID uint) error {
	// Delete hosts belonging to this group
	if err := tx.Unscoped().Where("group_id = ?", groupID).Delete(&Host{}).Error; err != nil {
		return fmt.Errorf("delete hosts in group %d: %w", groupID, err)
	}

	// Find and delete child groups recursively
	var children []Group
	if err := tx.Where("parent_id = ?", groupID).Find(&children).Error; err != nil {
		return fmt.Errorf("find children of group %d: %w", groupID, err)
	}
	for _, child := range children {
		if err := deleteGroupRecursive(tx, child.ID); err != nil {
			return err
		}
	}

	// Delete the group itself
	if err := tx.Unscoped().Delete(&Group{}, groupID).Error; err != nil {
		return fmt.Errorf("delete group %d: %w", groupID, err)
	}

	return nil
}

// CreateHost saves a new host.
func CreateHost(h *Host) error {
	return DB.Create(h).Error
}

// UpdateHost updates an existing host.
func UpdateHost(h *Host) error {
	return DB.Save(h).Error
}

// DeleteHost deletes a host. Deletion is physical so that a stored password
// does not linger in the database file.
func DeleteHost(h *Host) error {
	if err := DB.Unscoped().Delete(h).Error; err != nil {
		return err
	}
	return vacuum()
}

// vacuum rewrites the database file, which (together with SECURE_DELETE)
// removes any trace of deleted credentials from free pages.
//
// It is cheap for a personal connection list and safe to call repeatedly: the
// only failure mode is VACUUM running while another statement is open, in
// which case the error is reported to the caller verbatim.
func vacuum() error {
	if DB == nil {
		return nil
	}
	if err := DB.Exec("VACUUM").Error; err != nil {
		return fmt.Errorf("vacuum database: %w", err)
	}
	return nil
}
