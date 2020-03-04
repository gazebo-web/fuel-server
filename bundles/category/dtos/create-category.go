package category

type CreateCategory struct {
	Name     string  `json:"name"`
	Slug     *string `json:"slug,omitempty"`
	ParentID *uint   `json:"parent_id,omitempty"`
}
