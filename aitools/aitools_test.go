package aitools

import (
	"sort"
	"strings"
	"testing"

	"github.com/gazebo-web/fuel-server/mcp"
)

func TestCatalog(t *testing.T) {
	var names []string
	for _, s := range NewRegistry(Config{}).Specs() {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	want := []string{
		"fuel_get_model_details", "fuel_get_model_sdf", "fuel_get_world_sdf", "fuel_search_models", "fuel_search_worlds",
		"model_create",
		"project_checkpoint", "project_create", "project_edit_file", "project_get", "project_list",
		"project_list_files", "project_list_models", "project_list_versions", "project_read_file", "project_write_file",
		"scene_summary",
		"sdf_compute_inertia",
		"sdf_get_element_schema", "sdf_list_child_elements", "sdf_validate",
	}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("catalog changed:\n got %v\nwant %v", names, want)
	}
}

func TestEveryToolHasScopeAndDescription(t *testing.T) {
	reg := NewRegistry(Config{})
	for _, s := range reg.Specs() {
		tool, _ := reg.Get(s.Name)
		if len(tool.RequiredScopes) == 0 || s.Description == "" || s.InputSchema["type"] != "object" {
			t.Errorf("%s: scopes=%v description=%q schema=%v", s.Name, tool.RequiredScopes, s.Description, s.InputSchema)
		}
	}
}

func TestURIResolverFormatChecks(t *testing.T) {
	// No FuelHosts: format checks only, no database access.
	resolve := (&tools{}).uriResolver(&mcp.ToolContext{})
	for _, ok := range []string{
		"https://fuel.gazebosim.org/1.0/OpenRobotics/models/Table",
		"https://fuel.gazebosim.org/1.0/OpenRobotics/models/Prius%20Hybrid/2",
		"model://table",
		"meshes/thing.dae",
	} {
		if err := resolve(ok); err != nil {
			t.Errorf("%q: unexpected error %v", ok, err)
		}
	}
	for _, bad := range []string{
		"https://fuel.gazebosim.org/1.0/OpenRobotics/worlds/Depot",
		"https://example.com/table.sdf",
	} {
		if err := resolve(bad); err == nil {
			t.Errorf("%q: expected a format error", bad)
		}
	}
}

func TestAutoSaveMessage(t *testing.T) {
	if got := autoSaveMessage("project_edit_file", "world.sdf", ""); got != "auto-save: project_edit_file world.sdf" {
		t.Errorf("got %q", got)
	}
	if got := autoSaveMessage("project_edit_file", "world.sdf", " add shelves "); got != "auto-save: add shelves" {
		t.Errorf("got %q", got)
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("AI_TOOLS_FUEL_HOSTS", " fuel.gazebosim.org, ,staging-fuel.gazebosim.org")
	cfg := ConfigFromEnv()
	if strings.Join(cfg.FuelHosts, "|") != "fuel.gazebosim.org|staging-fuel.gazebosim.org" {
		t.Fatalf("hosts %v", cfg.FuelHosts)
	}
	if !(&tools{cfg: cfg}).isFuelHost("FUEL.gazebosim.org") {
		t.Fatal("host match should be case-insensitive")
	}
}

func TestSDFComputeInertia(t *testing.T) {
	tl := &tools{}
	res, _ := tl.sdfComputeInertia(nil, map[string]any{
		"shape": "box", "size": []any{0.5, 0.4, 0.3}, "density": 600.0, "pose": "0 0 0.15 0 0 0"})
	text := res.Content[0].Text
	if res.IsError || !strings.Contains(text, `"mass": 36`) || !strings.Contains(text, `<pose>0 0 0.15 0 0 0</pose>`) {
		t.Fatalf("box: %s", text)
	}
	for _, bad := range []map[string]any{
		{"shape": "box", "size": []any{1.0, 1.0}, "mass": 1.0},
		{"shape": "cylinder", "radius": 0.1, "mass": 1.0},
		{"shape": "sphere", "radius": 0.1},
	} {
		if res, _ := tl.sdfComputeInertia(nil, bad); !res.IsError {
			t.Errorf("%v: want an error, got %s", bad, res.Content[0].Text)
		}
	}
}
