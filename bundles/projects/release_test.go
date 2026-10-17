package projects

import "testing"

func TestLookupRelease(t *testing.T) {
	for _, in := range []string{"", "Harmonic", " harmonic "} {
		name, r, err := LookupRelease(in)
		if err != nil || name != "Harmonic" || r.SDF != "1.9" || r.ROS2 != "Jazzy" {
			t.Errorf("%q: %q %+v %v", in, name, r, err)
		}
	}
	for _, in := range []string{"Fortress", "Citadel", "Harmonik"} {
		if _, _, err := LookupRelease(in); err == nil {
			t.Errorf("%q: want an error", in)
		}
	}
}
