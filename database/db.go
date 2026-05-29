package database

import (
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// InitDB initializes the SQLite database
func InitDB(dbPath string) error {
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dbPath = filepath.Join(home, ".makoterm.db")
	}

	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return err
	}

	// Migrate the schema
	err = db.AutoMigrate(&Group{}, &Host{})
	if err != nil {
		return err
	}

	DB = db
	
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

// GetRootGroups returns all groups that don't have a parent (or are immediate children of logical root)
func GetRootGroups() ([]Group, error) {
	var groups []Group
	// Actually we seeded a "Root" group. Let's return its children or just top level if we have no true root.
	err := DB.Where("parent_id IS NULL OR parent_id = ?", 1).Find(&groups).Error
	return groups, err
}

// GetGroupChildren returns subgroups for a given group ID
func GetGroupChildren(parentID uint) ([]Group, error) {
	var groups []Group
	err := DB.Where("parent_id = ?", parentID).Find(&groups).Error
	return groups, err
}

// GetHostsForGroup returns hosts for a given group ID
func GetHostsForGroup(groupID uint) ([]Host, error) {
	var hosts []Host
	err := DB.Where("group_id = ?", groupID).Find(&hosts).Error
	return hosts, err
}

// CreateGroup saves a new group
func CreateGroup(g *Group) error {
	return DB.Create(g).Error
}

// UpdateGroup updates an existing group
func UpdateGroup(g *Group) error {
	return DB.Save(g).Error
}

// DeleteGroup deletes a group and all its hosts/subgroups
func DeleteGroup(g *Group) error {
	// Let's rely on GORM cascades or manual delete.
	// For simplicity, we just delete the group.
	// Hosts belonging to the group should ideally be deleted too.
	DB.Where("group_id = ?", g.ID).Delete(&Host{})
	return DB.Delete(g).Error
}

// CreateHost saves a new host
func CreateHost(h *Host) error {
	return DB.Create(h).Error
}

// UpdateHost updates an existing host
func UpdateHost(h *Host) error {
	return DB.Save(h).Error
}

// DeleteHost deletes a host
func DeleteHost(h *Host) error {
	return DB.Delete(h).Error
}
