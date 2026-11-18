package commonres

import (
	"path/filepath"
	"testing"

	"github.com/gazebo-web/fuel-server/globals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testResource is a minimal Resource used to build zip locations.
type testResource struct {
	owner string
	uuid  *string
}

func (r *testResource) GetName() *string            { return nil }
func (r *testResource) GetOwner() *string           { return &r.owner }
func (r *testResource) SetOwner(owner string)       { r.owner = owner }
func (r *testResource) GetLocation() *string        { return nil }
func (r *testResource) SetLocation(location string) {}
func (r *testResource) GetUUID() *string            { return r.uuid }

// TestZipLocationRequiresValidUUID checks that the zip location is only built
// for resources with a valid UUID.
func TestZipLocationRequiresValidUUID(t *testing.T) {
	resourceDir := globals.ResourceDir
	globals.ResourceDir = t.TempDir()
	t.Cleanup(func() { globals.ResourceDir = resourceDir })

	valid := "0b1c5a8e-3f57-4bd0-9a9a-5f2c1d7e8a90"
	zipPath, err := getOrCreateZipLocation(&testResource{owner: "owner", uuid: &valid}, "models", 2)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(globals.ResourceDir, "owner", "models", ".zips", valid+"v2.zip"), zipPath)

	invalid := []string{
		"",
		"not-a-uuid",
		"../../0b1c5a8e-3f57-4bd0-9a9a-5f2c1d7e8a90",
		"{0b1c5a8e-3f57-4bd0-9a9a-5f2c1d7e8a90}",
		"0b1c5a8e3f574bd09a9a5f2c1d7e8a90",
	}
	for _, id := range invalid {
		id := id
		t.Run(id, func(t *testing.T) {
			_, err := getOrCreateZipLocation(&testResource{owner: "owner", uuid: &id}, "models", 1)
			assert.Error(t, err)
		})
	}

	_, err = getOrCreateZipLocation(&testResource{owner: "owner"}, "models", 1)
	assert.Error(t, err)
}
