package database

import (
	"time"
)

// Group represents a folder or grouping for hosts.
//
// Deletion is physical (see DeleteGroup): passwords stored in hosts must not
// survive a delete, so the models deliberately avoid soft deletes and
// gorm.DeletedAt.
//
// The group tree is currently used one level deep: a single root group
// (ParentID == nil) owns every group visible in the UI. The ParentID field is
// kept so the schema can grow into real nesting without a migration.
type Group struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Name     string
	ParentID *uint
	Parent   *Group  `gorm:"foreignkey:ParentID"`
	Children []Group `gorm:"foreignkey:ParentID"`
	Hosts    []Host  `gorm:"foreignkey:GroupID"`
}

// Host represents an SSH connection target.
//
// Deletion is physical (see DeleteHost) so that a stored password does not
// remain in the database file after the host is removed.
type Host struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	GroupID *uint
	Group   *Group `gorm:"foreignkey:GroupID"`
	Name    string // Display name
	Address string // Hostname or IP
	Port    int    // SSH Port, default 22
	User    string // SSH user
	// Password is stored in plaintext. This is a documented limitation: the
	// database file is protected with 0600 permissions only. Prefer SSH keys.
	Password string
	KeyPath  string // Path to a specific identity file, if any
}
