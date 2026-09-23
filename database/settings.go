package database

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Setting is a small key/value table used for one-off markers such as
// "the demo data has already been seeded".
type Setting struct {
	Key   string `gorm:"primarykey"`
	Value string
}

const (
	settingDemoSeeded = "demo_seeded"
)

func settingGet(db *gorm.DB, key string) (string, error) {
	var s Setting
	err := db.First(&s, "key = ?", key).Error
	if err != nil {
		return "", err
	}
	return s.Value, nil
}

func settingSet(db *gorm.DB, key, value string) error {
	s := Setting{Key: key, Value: value}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&s).Error
}

// demoSeeded reports whether the demo data was already created for this
// database. Older databases have no marker, so the presence of user groups is
// used as the fallback signal.
func demoSeeded(db *gorm.DB) (bool, error) {
	v, err := settingGet(db, settingDemoSeeded)
	if err == nil {
		return v == "1", nil
	}
	if err != gorm.ErrRecordNotFound {
		return false, fmt.Errorf("read setting %q: %w", settingDemoSeeded, err)
	}
	return false, nil
}
