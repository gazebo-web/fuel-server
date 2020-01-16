package models

import (
	"bitbucket.org/ignitionrobotics/ign-go"
	"github.com/jinzhu/gorm"
	"strings"
)

// Tag is a string that can be used to mark other resources, such as models
// and worlds
//
// swagger:model
type Tag struct {
	gorm.Model

	Name *string `gorm:"not null;unique" json:"name,omitempty"`
}

// Tags is an array of Tag
//
// swagger:model
type Tags []Tag

// CreateTags populates DB Tags with the given tags.
// This function also trims tags before trying to add them.
func CreateTags(s *gorm.DB, tags []string) error {
	var tag Tag
	// Only create the tags that don't already exist.
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		tag = Tag{}
		if s.Where("name = ?", t).First(&tag).RecordNotFound() {
			newTag := Tag{Name: &t}
			if db := s.Create(&newTag); db.Error != nil {
				return db.Error
			}
		}
	}
	return nil
}

// FindTagsByName get a slice of DB Tags by name
func FindTagsByName(s *gorm.DB, tagNames []string) (*Tags, error) {
	var tags Tags
	if err := s.Where("Name in (?)", tagNames).Find(&tags).Error; err != nil {
		return nil, err
	}
	return &tags, nil
}

// TagsToStrSlice creates a string slice from the given Tags.
func TagsToStrSlice(tags Tags) []string {
	sl := make([]string, 0)
	for _, t := range tags {
		sl = append(sl, *t.Name)
	}
	return sl
}

// StrToTags -- convenient function to convert from user provided tags, as a
// comma-separated string to a slice of Tag objects, backed at DB.
func StrToTags(tx *gorm.DB, tagsStr string) (*Tags, error) {
	userTags := ign.StrToSlice(tagsStr)
	if err := CreateTags(tx, userTags); err != nil {
		return nil, err
	}
	pTags, err := FindTagsByName(tx, userTags)
	if err != nil {
		return nil, err
	}
	return pTags, nil
}
