package database

import "gorm.io/gorm"

// Group represents a folder or grouping for hosts
type Group struct {
	gorm.Model
	Name     string
	ParentID *uint
	Parent   *Group  `gorm:"foreignkey:ParentID"`
	Children []Group `gorm:"foreignkey:ParentID"`
	Hosts    []Host  `gorm:"foreignkey:GroupID"`
}

// Host represents an SSH connection target
type Host struct {
	gorm.Model
	GroupID  *uint
	Group    *Group `gorm:"foreignkey:GroupID"`
	Name     string // Display name
	Address  string // Hostname or IP
	Port     int    // SSH Port, default 22
	User     string // SSH User
	Password string // Encrypted or plaintext password (for this demo, plaintext but in real app would be encrypted)
	KeyPath  string // Path to specific identity file if any
}
