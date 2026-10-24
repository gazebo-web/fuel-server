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
