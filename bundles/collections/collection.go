package collections

import (
	"github.com/gazebo-web/fuel-server/bundles/users"
	"github.com/gazebo-web/gz-go/v7"
	"github.com/jinzhu/gorm"
	"time"
)

const (
	// collections is the type sub-folder under user folder on disk
	collections string = "collections"

	// TModel is used to represent the asset type "model"
	TModel string = "model"

	// TWorld is used to represent the asset type "world"
	TWorld string = "world"
)

// Collection represents a collection of assets.
//
// A collection has a name, owner and optional description.
//
// swagger:model dbCollection
type Collection struct {
	// Override default GORM Model fields
	ID        uint      `gorm:"primary_key" json:"-"`
	CreatedAt time.Time `gorm:"type:timestamp(3) NULL"`
	UpdatedAt time.Time
	// Added 2 milliseconds to DeletedAt field, and added it to the unique index
	// to help disambiguate when soft deleted rows are involved.
	DeletedAt *time.Time `gorm:"type:timestamp(2) NULL; unique_index:idx_colname_owner" sql:"index"`

	// The name of the collection
	Name *string `gorm:"unique_index:idx_colname_owner" json:"name,omitempty"`

	// Unique identifier
	UUID *string `json:"-"`

	// A description of the collection (max 65,535 chars)
	Description *string `gorm:"type:text" json:"description,omitempty"`

	// Location on disk
	Location *string `json:"-"`

	// The owner of this collection (must exist in UniqueOwners). Can be user or org.
	Owner *string `gorm:"unique_index:idx_colname_owner" json:"owner,omitempty"`

	// The username of the User that created this collection (usually got from the JWT)
	Creator *string `json:"-"`

	// Private - True to make this a private resource
	Private *bool `json:"private,omitempty"`

	// A list of thumbnail urls from the associated models/worlds.
	ThumbnailUrls []string `gorm:"-" json:"thumbnails,omitempty"`
}

// Collections is an array of Collection
// swagger:model dbCollections
type Collections []Collection

// validateAssetType validates that the given string is a valid asset type.
// Returns an gz.ErrMsg otherwise.
func validateAssetType(aType string) *gz.ErrMsg {
	if aType != TModel && aType != TWorld {
		return gz.NewErrorMessageWithArgs(gz.ErrorFormInvalidValue, nil, []string{aType})
	}
	return nil
}

// CollectionAsset represents an association between a collection and a resource.
// It was implemented with a "type" to support adding new types easily.
type CollectionAsset struct {
	// Override default GORM Model fields
	ID uint `gorm:"primary_key" json:"-"`
	// The collection ID
	ColID uint `json:"-"`
	// The related asset ID (eg model ID)
	AssetID uint `json:"-"`
	// The name of the related asset
	AssetName string `json:"asset_name,omitempty"`
	// The owner of the related asset (org / user)
	AssetOwner string `json:"asset_owner,omitempty"`
	// The asset type (model | world).
	Type string `json:"type,omitempty"`
}

// CollectionAssets is a list of Collection assets
// swagger:model dbCollectionAssets
type CollectionAssets []CollectionAsset

// CloneCollection encapsulates data required to clone a collection
type CloneCollection struct {
	// The name of the collection
	// required: false
	Name string `json:"name" validate:"omitempty,noforwardslash,min=3,nopercent" form:"name"`
	// Optional Owner of the collection. Must be a user or an org.
	// If not set, the current user will be used as owner
	Owner string `json:"owner" form:"owner"`
	// Private privacy/visibility setting
	Private *bool `json:"private,omitempty" validate:"omitempty" form:"private"`
}

// GetID returns the ID
func (c *Collection) GetID() uint {
	return c.ID
}

// GetName returns the name
func (c *Collection) GetName() *string {
	return c.Name
}

// GetOwner returns the owner
func (c *Collection) GetOwner() *string {
	return c.Owner
}

// SetOwner sets the owner
func (c *Collection) SetOwner(owner string) {
	*c.Owner = owner
}

// GetLocation returns the location on disk
func (c *Collection) GetLocation() *string {
	return c.Location
}

// SetLocation sets the location path
func (c *Collection) SetLocation(location string) {
	*c.Location = location
}

// GetUUID returns the UUID
func (c *Collection) GetUUID() *string {
	return c.UUID
}

// QueryForCollections returns a gorm query configured to query Collections.
func QueryForCollections(q *gorm.DB) *gorm.DB {
	return q.Model(&Collection{}).Order("id")
}

// ByName queries a Collection by name and owner.
func ByName(tx *gorm.DB, name, owner string) (*Collection, error) {
	var res Collection
	if err := QueryForCollections(tx).Where("owner = ? AND name = ?", owner, name).
		First(&res).Error; err != nil {
		return nil, err
	}
	return &res, nil
}

// FindAssociation queries CollectionAssets by name, owner and type.
func FindAssociation(tx *gorm.DB, colID uint, owner, name,
	assetType string) (*CollectionAsset, error) {

	var res CollectionAsset
	blankQuery := tx.New()
	if err := blankQuery.Model(&CollectionAsset{}).Where("col_id = ?", colID).
		Where("asset_owner = ? AND asset_name = ? AND type = ?", owner, name, assetType).
		First(&res).Error; err != nil {
		return nil, err
	}
	return &res, nil
}

// FindAssociations returns a list of CollectionAssets from a given Collection.
func FindAssociations(tx *gorm.DB, col *Collection, limit int) (*CollectionAssets, error) {
	blankQuery := tx.New()
	q := blankQuery.Model(&CollectionAsset{}).Where("col_id = ?", col.ID)
	if limit > 0 {
		q = q.Limit(limit)
	}

	var list CollectionAssets
	if err := q.Find(&list).Error; err != nil {
		return nil, err
	}
	return &list, nil
}

// NewCollection creates a new Model struct
func NewCollection(name, desc, owner, creator *string,
	private bool) (Collection, error) {

	uuidStr, _, err := users.NewUUID(*owner, collections)
	if err != nil {
		return Collection{}, err
	}

	path := users.GetResourcePath(*owner, uuidStr, collections)

	collection := Collection{Name: name, UUID: &uuidStr, Location: &path,
		Owner: owner, Creator: creator, Description: desc, Private: &private,
	}
	return collection, nil
}

// CreateCollection encapsulates data required to create a collection
type CreateCollection struct {
	// The name
	// required: true
	Name string `json:"name" validate:"required,noforwardslash,min=3,nopercent"`
	// Optional Owner. Must be a user or an org.
	// If not set, the current user will be used as owner
	Owner string `json:"owner" form:"owner"`
	// Optional description
	Description string `json:"description" form:"description"`
	// Optional privacy/visibility setting.
	Private *bool `json:"private" validate:"omitempty" form:"private"`
}

// UpdateCollection encapsulates data that can be updated in a collection
type UpdateCollection struct {
	// Optional description
	Description *string `json:"description" form:"description"`
	// Optional collection logo
	File string `json:"file" validate:"omitempty,gt=0" form:"-"`
	// Private privacy/visibility setting
	Private *bool `json:"private" validate:"omitempty" form:"private"`
}

// IsEmpty returns true is the struct is empty.
func (uc UpdateCollection) IsEmpty() bool {
	return uc.Description == nil && uc.Private == nil
}

// NameOwnerPair describes a name and owner to find an asset.
type NameOwnerPair struct {
	// The name
	// required: true
	Name string `json:"name" validate:"required,noforwardslash,nopercent"`
	// Asset Owner. Must be a user or an org.
	// required: true
	Owner string `json:"owner" validate:"required"`
}
