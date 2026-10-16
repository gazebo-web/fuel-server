package projects

import (
	"path/filepath"
	"testing"

	"github.com/gazebo-web/fuel-server/globals"
)

// TestNewProjectAndUUIDResolvesLocation verifies the location path
// contract: {ResourceDir}/{owner}/projects/{uuid}. Tests run without a
// live fuel-server, so we set globals.ResourceDir to a temp value.
func TestNewProjectAndUUIDResolvesLocation(t *testing.T) {
	orig := globals.ResourceDir
	t.Cleanup(func() { globals.ResourceDir = orig })
	globals.ResourceDir = t.TempDir()

	name := "warehouse-demo"
	desc := "a warehouse and an amr"
	owner := "nkoenig"
	creator := owner
	gz := "Harmonic"
	sdfv := "1.9"
	ros := "Jazzy"

	p, err := NewProjectAndUUID(&name, &desc, &owner, &creator, &gz, &sdfv, &ros)
	if err != nil {
		t.Fatalf("NewProjectAndUUID: %v", err)
	}
	if p.Location == nil || *p.Location == "" {
		t.Fatalf("Location not populated")
	}
	if p.UUID == nil || *p.UUID == "" {
		t.Fatalf("UUID not populated")
	}

	want := filepath.Join(globals.ResourceDir, owner, "projects", *p.UUID)
	if *p.Location != want {
		t.Errorf("Location = %q, want %q", *p.Location, want)
	}

	// Resource-interface accessors all work on the returned struct.
	if got := p.GetName(); got == nil || *got != name {
		t.Errorf("GetName = %v, want %q", got, name)
	}
	if got := p.GetOwner(); got == nil || *got != owner {
		t.Errorf("GetOwner = %v, want %q", got, owner)
	}
	if got := p.GetLocation(); got == nil || *got != *p.Location {
		t.Errorf("GetLocation mismatch")
	}
	if got := p.GetUUID(); got == nil || *got != *p.UUID {
		t.Errorf("GetUUID mismatch")
	}
}
