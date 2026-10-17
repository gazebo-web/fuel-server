package projects

import (
	"fmt"
	"sort"
	"strings"
)

// Release is the SDF version and ROS 2 distro that projects for a Gazebo
// release use (design §7).
type Release struct {
	SDF  string
	ROS2 string
}

// DefaultRelease is used when a project is created without one.
const DefaultRelease = "Harmonic"

// releases lists the Gazebo releases new projects can target. Harmonic is
// the oldest supported (design v3.1, D-4); projects created for older
// releases still open, but can't be created or switched to.
var releases = map[string]Release{
	"Harmonic": {SDF: "1.9", ROS2: "Jazzy"},
}

// SupportedReleases returns the release names, sorted.
func SupportedReleases() []string {
	out := make([]string, 0, len(releases))
	for r := range releases {
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

// LookupRelease resolves a release name (case-insensitive; empty means
// DefaultRelease) to its canonical name and versions.
func LookupRelease(name string) (string, Release, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultRelease
	}
	for canonical, r := range releases {
		if strings.EqualFold(canonical, name) {
			return canonical, r, nil
		}
	}
	return "", Release{}, fmt.Errorf("unsupported Gazebo release %q; supported: %s",
		name, strings.Join(SupportedReleases(), ", "))
}
