package category

import (
	"errors"
	"github.com/jinzhu/gorm"
)

// Category is a type of label used to group resources, such as models and
// worlds, together. A category consists of a name, ID, and parentID. The
// parentID field should refer to a parent category, and supports a hierarchy
// of categories.
//
// swagger:model
type Category struct {
	gorm.Model

	// Name is the name of the category
	Name *string `gorm:"not null;unique" json:"name"`

	// Slug is the human-friendly URL path to the category
	Slug *string `gorm:"not null;unique" json:"slug"`

	// ParentID is an optional parent ID.
	ParentID *uint `json:"parent_id,omitempty"`
}

// ByName returns a category by the given name.
func ByName(tx *gorm.DB, name string) (*Category, error) {
	var cat Category
	q := tx.Model(&Category{}).Where("name = ?", name)

	if err := q.First(&cat).Error; err != nil {
		return nil, err
	}

	return &cat, nil
}

// ByNames returns a slice of Categories from the given slice of names.
func ByNames(tx *gorm.DB, names []string) (*Categories, error) {
	var cats Categories
	q := tx.Model(&Category{}).Where("name IN (?)", names)
	if err := q.Find(&cats).Error; err != nil {
		return nil, err
	}
	if len(cats) != len(names) {
		return nil, errors.New("resource does not exist")
	}
	return &cats, nil
}

// BySlug returns a category by the given slug.
func BySlug(tx *gorm.DB, slug string) (*Category, error) {
	var cat Category

	q := tx.Model(&Category{}).Where("slug = ?", slug)

	if err := q.First(&cat).Error; err != nil {
		return nil, err
	}

	return &cat, nil
}

// HasParent returns true if the category has a ParentID assigned.
func (c Category) HasParent() bool {
	return c.ParentID != nil
}

// IsParent returns true if there are categories that have the current category ID as Parent ID.
func (c Category) IsParent(tx *gorm.DB) (*bool, error) {
	var count int64
	if err := tx.Model(&Category{}).Where("parent_id = ?", c.ID).Count(&count).Error; err != nil {
		return nil, err
	}
	isParent := count > 0
	return &isParent, nil
}

// Categories is an array of Category
//
// swagger:model
type Categories []Category

// CategoriesToStrSlice returns a slice of category names by the given categories slice
func CategoriesToStrSlice(categories Categories) []string {
	var sl []string
	for _, c := range categories {
		sl = append(sl, *c.Name)
	}
	return sl
}

// StrSliceToCategories returns a slice of categories by the given category names
func StrSliceToCategories(tx *gorm.DB, sl []string) (*Categories, error) {
	var categories *Categories
	var err error
	if categories, err = ByNames(tx, sl); err != nil {
		return nil, err
	}
	return categories, nil
}
