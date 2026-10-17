package sdf

import (
	"math"
	"strings"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(b)) }

func TestPrimitiveInertial(t *testing.T) {
	cases := []struct {
		s             Shape
		mass          float64
		vol           float64
		ixx, iyy, izz float64
	}{
		{Shape{Kind: "box", Size: [3]float64{1, 2, 3}}, 12, 6, 13, 10, 5},
		{Shape{Kind: "cylinder", Radius: 1, Length: 2}, 12, 2 * math.Pi, 7, 7, 6},
		{Shape{Kind: "sphere", Radius: 1}, 5, 4 * math.Pi / 3, 2, 2, 2},
		{Shape{Kind: "ellipsoid", Size: [3]float64{1, 2, 3}}, 5, 8 * math.Pi, 13, 10, 5},
		// A capsule with no cylinder is a sphere.
		{Shape{Kind: "capsule", Radius: 1}, 5, 4 * math.Pi / 3, 2, 2, 2},
	}
	for _, c := range cases {
		in, err := PrimitiveInertial(c.s, c.mass, 0)
		if err != nil {
			t.Fatalf("%s: %v", c.s.Kind, err)
		}
		if !near(in.Volume, c.vol) || !near(in.IXX, c.ixx) || !near(in.IYY, c.iyy) || !near(in.IZZ, c.izz) {
			t.Errorf("%s: got %+v", c.s.Kind, in)
		}
	}
}

// A capsule's inertia must lie between a cylinder of its total length and
// a cylinder of its straight part, and approach the cylinder as it gets long.
func TestCapsuleInertia(t *testing.T) {
	r, l := 0.1, 1.0
	capIn, _ := PrimitiveInertial(Shape{Kind: "capsule", Radius: r, Length: l}, 0, 1000)
	inner, _ := PrimitiveInertial(Shape{Kind: "cylinder", Radius: r, Length: l}, capIn.Mass, 0)
	outer, _ := PrimitiveInertial(Shape{Kind: "cylinder", Radius: r, Length: l + 2*r}, capIn.Mass, 0)
	if !(capIn.IXX > inner.IXX && capIn.IXX < outer.IXX) {
		t.Errorf("capsule ixx %g not between %g and %g", capIn.IXX, inner.IXX, outer.IXX)
	}
	if !(capIn.IZZ < inner.IZZ) { // hemispheres hold mass nearer the axis
		t.Errorf("capsule izz %g should be below cylinder %g", capIn.IZZ, inner.IZZ)
	}
	if !near(capIn.Mass, 1000*(math.Pi*r*r*l+4*math.Pi*r*r*r/3)) {
		t.Errorf("mass from density: %g", capIn.Mass)
	}
}

func TestPrimitiveInertialErrors(t *testing.T) {
	for _, c := range []struct {
		s             Shape
		mass, density float64
		want          string
	}{
		{Shape{Kind: "cone", Radius: 1}, 1, 0, "unknown shape"},
		{Shape{Kind: "box", Size: [3]float64{1, 0, 1}}, 1, 0, "y must be positive"},
		{Shape{Kind: "cylinder", Radius: 1}, 1, 0, "length must be positive"},
		{Shape{Kind: "sphere", Radius: 1}, 0, 0, "mass or density"},
		{Shape{Kind: "sphere", Radius: 1}, 1, 1, "not both"},
		{Shape{Kind: "sphere", Radius: math.NaN()}, 1, 0, "radius must be positive"},
	} {
		if _, err := PrimitiveInertial(c.s, c.mass, c.density); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: got %v, want %q", c.s, err, c.want)
		}
	}
}

func TestInertialXMLValidates(t *testing.T) {
	in, _ := PrimitiveInertial(Shape{Kind: "box", Size: [3]float64{0.5, 0.4, 0.3}}, 0, 600)
	doc := `<sdf version="1.9"><model name="crate"><link name="l">` + in.XML("0 0 0.15 0 0 0") +
		`<collision name="c"><geometry><box><size>0.5 0.4 0.3</size></box></geometry></collision>` +
		`</link></model></sdf>`
	if r := Validate(doc); !r.Valid || len(r.Warnings) != 0 {
		t.Fatalf("errors %v warnings %v\n%s", r.Errors, r.Warnings, doc)
	}
	if !strings.Contains(in.XML(""), "<mass>36</mass>") {
		t.Errorf("mass formatting: %s", in.XML(""))
	}
}

func TestDensityWarning(t *testing.T) {
	doc := func(static, mass, geom string) string {
		return `<sdf version="1.9"><model name="m">` + static + `<link name="l"><inertial><mass>` + mass +
			`</mass><inertia><ixx>1</ixx><iyy>1</iyy><izz>1</izz></inertia></inertial>` +
			`<collision name="c"><geometry>` + geom + `</geometry></collision></link></model></sdf>`
	}
	crate := `<box><size>2 1 1</size></box>`
	for _, c := range []struct {
		static, mass, geom string
		warn               bool
	}{
		{"", "1", crate, true},    // 0.5 kg/m³
		{"", "200", crate, false}, // 100 kg/m³
		{"", "1e6", crate, true},  // denser than tungsten
		{"<static>true</static>", "1", crate, false},
		{"", "1", `<mesh><uri>meshes/crate.glb</uri></mesh>`, false}, // unknown volume
	} {
		r := Validate(doc(c.static, c.mass, c.geom))
		got := strings.Contains(r.Summary("m.sdf"), "kg/m³")
		if got != c.warn {
			t.Errorf("static=%q mass=%s geom=%s: warning=%v\n%s", c.static, c.mass, c.geom, got, r.Summary("m.sdf"))
		}
	}
}
