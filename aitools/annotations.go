package aitools

import "github.com/gazebo-web/fuel-server/mcp"

// Scopes are the permissions a client can be granted (design §8.4). The
// protected resource metadata lists them, and OAuth tokens and personal
// access tokens carry them.
var Scopes = []string{"fuel:read", "project:read", "project:write", "mesh:generate"}

// toolAnnotations tell clients what each tool does; every registered tool
// must be listed (TestToolAnnotations).
var toolAnnotations = map[string]mcp.Annotations{
	"fuel_search_models":     {ReadOnly: true, OpenWorld: true},
	"fuel_search_worlds":     {ReadOnly: true, OpenWorld: true},
	"fuel_get_model_details": {ReadOnly: true, OpenWorld: true},
	"fuel_get_model_sdf":     {ReadOnly: true, OpenWorld: true},
	"fuel_get_world_sdf":     {ReadOnly: true, OpenWorld: true},

	"sdf_validate":            {ReadOnly: true},
	"sdf_compute_inertia":     {ReadOnly: true},
	"sdf_get_element_schema":  {ReadOnly: true},
	"sdf_list_child_elements": {ReadOnly: true},
	"mesh_api_reference":      {ReadOnly: true},
	"mesh_list_materials":     {ReadOnly: true},

	"project_list":          {ReadOnly: true},
	"project_get":           {ReadOnly: true},
	"project_list_files":    {ReadOnly: true},
	"project_read_file":     {ReadOnly: true},
	"project_list_versions": {ReadOnly: true},
	"project_list_models":   {ReadOnly: true},
	"scene_summary":         {ReadOnly: true},
	"asset_list":            {ReadOnly: true},
	"asset_view":            {ReadOnly: true},

	"project_create":     {},
	"asset_upload":       {},
	"project_checkpoint": {},
	"model_create":       {},
	// These overwrite what is there: a file, or a model's whole folder.
	"project_write_file":    {Destructive: true},
	"project_edit_file":     {Destructive: true},
	"model_generate":        {Destructive: true},
	"model_import":          {Destructive: true},
	"model_generate_status": {Destructive: true},
}

// extraScopes are scopes a tool needs beyond the one it registers with.
var extraScopes = map[string][]string{
	// Generating runs code on the mesh worker (design §8.5).
	"model_generate":        {"mesh:generate"},
	"model_import":          {"mesh:generate"},
	"model_generate_status": {"mesh:generate"},
}

// annotate sets every registered tool's annotations and extra scopes.
func annotate(reg *mcp.Registry) {
	for _, spec := range reg.Specs() {
		t, _ := reg.Get(spec.Name)
		t.Annotations = toolAnnotations[spec.Name]
		t.RequiredScopes = append(t.RequiredScopes, extraScopes[spec.Name]...)
		reg.Register(t)
	}
}
