package database

import (
	"fmt"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB initializes the SQLite database.
// If dbPath is empty, defaults to ~/.makoterm.db
func InitDB(dbPath string) error {
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home directory: %w", err)
		}
		dbPath = filepath.Join(home, ".makoterm.db")
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Migrate the schema
	if err := db.AutoMigrate(&Group{}, &Host{}); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}

	DB = db

	// Set file permissions to 0600 (owner-only read/write) since DB may contain passwords
	if err := os.Chmod(dbPath, 0600); err != nil {
		return fmt.Errorf("set db permissions: %w", err)
	}

	// Seed root group if empty
	var count int64
	db.Model(&Group{}).Count(&count)
	if count == 0 {
		root := Group{Name: "Root"}
		db.Create(&root)

		// Add some sample data for demonstration
		prod := Group{Name: "Production", ParentID: &root.ID}
		dev := Group{Name: "Development", ParentID: &root.ID}
		db.Create(&prod)
		db.Create(&dev)

		db.Create(&Host{GroupID: &prod.ID, Name: "Web Server", Address: "127.0.0.1", Port: 2222, User: "admin"})
		db.Create(&Host{GroupID: &prod.ID, Name: "DB Server", Address: "127.0.0.1", Port: 2223, User: "dbadmin"})
		db.Create(&Host{GroupID: &dev.ID, Name: "Staging", Address: "localhost", Port: 22, User: "tolkin"})
	}

	return nil
}

// GetRootGroup returns the root group (the one with no parent).
func GetRootGroup() (*Group, error) {
	var root Group
	err := DB.Where("parent_id IS NULL").First(&root).Error
	if err != nil {
		return nil, err
	}
	return &root, nil
}

// GetRootGroups returns top-level groups visible in the UI.
// These are children of the root group plus any orphan groups.
func GetRootGroups() ([]Group, error) {
	root, err := GetRootGroup()
	if err != nil {
		// No root group — return all groups
		var groups []Group
		err := DB.Find(&groups).Error
		return groups, err
	}

	var groups []Group
	err = DB.Where("parent_id IS NULL OR parent_id = ?", root.ID).Find(&groups).Error
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
	return DB.Create(g).Error
}

// UpdateGroup updates an existing group.
func UpdateGroup(g *Group) error {
	return DB.Save(g).Error
}

// DeleteGroup deletes a group, its hosts, and all nested subgroups recursively.
func DeleteGroup(g *Group) error {
	return deleteGroupRecursive(g.ID)
}

// deleteGroupRecursive removes a group and all its descendants.
func deleteGroupRecursive(groupID uint) error {
	// Delete hosts belonging to this group
	if err := DB.Where("group_id = ?", groupID).Delete(&Host{}).Error; err != nil {
		return fmt.Errorf("delete hosts in group %d: %w", groupID, err)
	}

	// Find and delete child groups recursively
	var children []Group
	if err := DB.Where("parent_id = ?", groupID).Find(&children).Error; err != nil {
		return fmt.Errorf("find children of group %d: %w", groupID, err)
	}
	for _, child := range children {
		if err := deleteGroupRecursive(child.ID); err != nil {
			return err
		}
	}

	// Delete the group itself
	if err := DB.Delete(&Group{}, groupID).Error; err != nil {
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

// DeleteHost deletes a host.
func DeleteHost(h *Host) error {
	return DB.Delete(h).Error
}
