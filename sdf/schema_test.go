package sdf

import (
	"strings"
	"testing"
)

func TestDefaultRegistryLoads(t *testing.T) {
	r, err := Default()
	if err != nil {
		t.Fatalf("default registry load: %v", err)
	}
	versions := r.Versions()
	if len(versions) == 0 {
		t.Fatalf("expected embedded schema versions, got none")
	}
	// The repo ships 1.8 and 1.9 at minimum; the test should fail loudly
	// if either disappears so we notice an accidental delete.
	for _, want := range []string{"1.8", "1.9"} {
		found := false
		for _, v := range versions {
			if v == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing SDF version %q (have %v)", want, versions)
		}
	}
}

func TestGetCommonElements(t *testing.T) {
	r := MustDefault()
	for _, name := range []string{"world", "model", "link", "joint", "physics", "scene"} {
		if _, ok := r.Get("1.9", name); !ok {
			t.Errorf("expected element %q in SDF 1.9", name)
		}
	}
}

func TestChildNamesResolvesIncludes(t *testing.T) {
	children := MustDefault().ChildNames("1.9", "geometry")
	joined := strings.Join(children, ",")
	for _, want := range []string{"box", "mesh", "sphere"} {
		if !strings.Contains(","+joined+",", ","+want+",") {
			t.Errorf("geometry children %v missing %q", children, want)
		}
	}
	if strings.Contains(joined, "_shape") {
		t.Errorf("children should be element names, not filenames: %v", children)
	}
}

func TestGetPrefersCanonicalFile(t *testing.T) {
	// model_state.sdf and link_state.sdf also have <model>/<link> roots.
	r := MustDefault()
	for _, name := range []string{"model", "link"} {
		e, _ := r.Get("1.9", name)
		canonical, _ := r.ElementForFile("1.9", name+".sdf")
		if e != canonical {
			t.Errorf("Get(%q) is not the definition from %s.sdf", name, name)
		}
	}
}

func TestNestedElements(t *testing.T) {
	r := MustDefault()
	// include has no schema file; world and model define it inline.
	e, ok := r.Get("1.9", "include")
	if !ok || e.Name != "include" {
		t.Fatalf("include: %v %v", e, ok)
	}
	children := strings.Join(r.ChildNames("1.9", "include"), ",")
	if !strings.Contains(children, "uri") || !strings.Contains(children, "pose") {
		t.Errorf("include's children: %s", children)
	}
	if _, ok := r.Get("1.9", "no_such_element"); ok {
		t.Error("unknown element found")
	}
}

func TestIncludeHoldsPlugin(t *testing.T) {
	// The AI Builder loads gz_ros2_control per robot inside its <include>.
	world := `<?xml version="1.0"?><sdf version="1.9"><world name="w">
	  <include><uri>https://fuel.gazebosim.org/1.0/OpenRobotics/models/Table</uri><name>amr_1</name>
	    <plugin filename="gz_ros2_control-system" name="gz_ros2_control::GazeboSimROS2ControlPlugin">
	      <ros><namespace>amr_1</namespace></ros><parameters>config/controllers.yaml</parameters>
	    </plugin></include></world></sdf>`
	for _, v := range []string{"1.9", "1.12"} {
		doc := strings.Replace(world, `version="1.9"`, `version="`+v+`"`, 1)
		if r := Validate(doc); len(r.Errors) > 0 || len(r.Warnings) > 0 {
			t.Errorf("SDF %s: %+v %+v", v, r.Errors, r.Warnings)
		}
	}
}
