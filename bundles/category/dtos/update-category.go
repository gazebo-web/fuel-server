package category

type UpdateCategory struct {
	Name     *string `json:"name,omitempty"`
	Slug     *string `json:"slug,omitempty"`
	ParentID *uint   `json:"parent_id,omitempty"`
}
