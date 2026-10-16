package sdf

import "testing"

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

func TestFullSchemaNonEmpty(t *testing.T) {
	r := MustDefault()
	s, ok := r.FullSchema("1.9")
	if !ok {
		t.Fatalf("expected full schema for 1.9")
	}
	if len(s) < 10_000 {
		t.Errorf("full schema smaller than expected: %d bytes", len(s))
	}
}

func TestValidateAcceptsMinimalWorld(t *testing.T) {
	sdf := `<sdf version="1.9"><world name="w"/></sdf>`
	r := Validate(sdf)
	if !r.Valid {
		t.Errorf("expected valid; errors=%+v", r.Errors)
	}
}

func TestValidateFlagsMissingRoot(t *testing.T) {
	r := Validate(`<foo/>`)
	if r.Valid {
		t.Errorf("expected invalid for non-sdf root")
	}
}

func TestValidateFlagsMissingVersion(t *testing.T) {
	r := Validate(`<sdf><model name="m"/></sdf>`)
	if r.Valid {
		t.Errorf("expected invalid without version attribute")
	}
}
