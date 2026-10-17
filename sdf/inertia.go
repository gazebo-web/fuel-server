package sdf

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Shape is an SDF primitive geometry with its dimensions in meters.
// Cylinders and capsules are along Z, as in SDF.
type Shape struct {
	Kind   string     // box, cylinder, sphere, capsule, ellipsoid
	Size   [3]float64 // box: x y z; ellipsoid: radii x y z
	Radius float64    // cylinder, sphere, capsule
	Length float64    // cylinder, capsule (the cylindrical part, as in SDF)
}

// Inertial is a mass and the inertia tensor about the shape's center.
type Inertial struct {
	Mass          float64 `json:"mass"`
	Volume        float64 `json:"volume"`
	IXX, IYY, IZZ float64
}

// ShapeKinds lists the shapes PrimitiveInertial accepts.
func ShapeKinds() []string {
	return []string{"box", "capsule", "cylinder", "ellipsoid", "sphere"}
}

// Volume returns the shape's volume in m³.
func (s Shape) Volume() (float64, error) {
	if err := s.check(); err != nil {
		return 0, err
	}
	r, l := s.Radius, s.Length
	switch s.Kind {
	case "box":
		return s.Size[0] * s.Size[1] * s.Size[2], nil
	case "cylinder":
		return math.Pi * r * r * l, nil
	case "sphere":
		return 4.0 / 3 * math.Pi * r * r * r, nil
	case "capsule":
		return math.Pi*r*r*l + 4.0/3*math.Pi*r*r*r, nil
	default: // ellipsoid
		return 4.0 / 3 * math.Pi * s.Size[0] * s.Size[1] * s.Size[2], nil
	}
}

func (s Shape) check() error {
	pos := func(name string, v float64) error {
		if !(v > 0) || math.IsInf(v, 0) {
			return fmt.Errorf("%s %s must be positive, got %g", s.Kind, name, v)
		}
		return nil
	}
	switch s.Kind {
	case "box", "ellipsoid":
		for i, axis := range []string{"x", "y", "z"} {
			if err := pos(axis, s.Size[i]); err != nil {
				return err
			}
		}
	case "sphere":
		return pos("radius", s.Radius)
	case "cylinder", "capsule":
		if err := pos("radius", s.Radius); err != nil {
			return err
		}
		if s.Kind == "capsule" && s.Length == 0 {
			return nil // a capsule with no cylinder part is a sphere
		}
		return pos("length", s.Length)
	default:
		return fmt.Errorf("unknown shape %q (want one of %s)", s.Kind, strings.Join(ShapeKinds(), ", "))
	}
	return nil
}

// PrimitiveInertial returns the inertial of a solid shape of uniform
// density, given either its mass or its density (kg/m³).
func PrimitiveInertial(s Shape, mass, density float64) (Inertial, error) {
	vol, err := s.Volume()
	if err != nil {
		return Inertial{}, err
	}
	switch {
	case mass > 0 && density > 0:
		return Inertial{}, fmt.Errorf("give mass or density, not both")
	case density > 0:
		mass = density * vol
	case !(mass > 0):
		return Inertial{}, fmt.Errorf("mass or density must be positive")
	}
	in := Inertial{Mass: mass, Volume: vol}
	r, l := s.Radius, s.Length
	switch s.Kind {
	case "box":
		x, y, z := s.Size[0], s.Size[1], s.Size[2]
		in.IXX, in.IYY, in.IZZ = mass*(y*y+z*z)/12, mass*(x*x+z*z)/12, mass*(x*x+y*y)/12
	case "cylinder":
		in.IXX = mass * (3*r*r + l*l) / 12
		in.IYY, in.IZZ = in.IXX, mass*r*r/2
	case "sphere":
		in.IXX = 2 * mass * r * r / 5
		in.IYY, in.IZZ = in.IXX, in.IXX
	case "capsule":
		// Split the mass between the cylinder and the two hemispheres by
		// volume; each hemisphere's center of mass is 3r/8 from its face.
		vc := math.Pi * r * r * l
		mc, ms := mass*vc/vol, mass*(vol-vc)/vol
		in.IXX = mc*(l*l/12+r*r/4) + ms*(2*r*r/5+l*l/4+3*l*r/8)
		in.IYY, in.IZZ = in.IXX, mc*r*r/2+ms*2*r*r/5
	case "ellipsoid":
		a, b, c := s.Size[0], s.Size[1], s.Size[2]
		in.IXX, in.IYY, in.IZZ = mass*(b*b+c*c)/5, mass*(a*a+c*c)/5, mass*(a*a+b*b)/5
	}
	return in, nil
}

// XML returns an <inertial> element. pose, if not empty, is the inertial
// frame's pose relative to the link (the shape's center).
func (in Inertial) XML(pose string) string {
	var b strings.Builder
	b.WriteString("<inertial>\n")
	if pose = strings.TrimSpace(pose); pose != "" {
		fmt.Fprintf(&b, "  <pose>%s</pose>\n", pose)
	}
	fmt.Fprintf(&b, "  <mass>%s</mass>\n  <inertia>\n", num(in.Mass))
	fmt.Fprintf(&b, "    <ixx>%s</ixx><ixy>0</ixy><ixz>0</ixz>\n", num(in.IXX))
	fmt.Fprintf(&b, "    <iyy>%s</iyy><iyz>0</iyz>\n", num(in.IYY))
	fmt.Fprintf(&b, "    <izz>%s</izz>\n  </inertia>\n</inertial>", num(in.IZZ))
	return b.String()
}

// num formats with 6 significant digits.
func num(x float64) string { return strconv.FormatFloat(x, 'g', 6, 64) }

// densityBounds are the densities (kg/m³) outside which a link's mass is
// probably wrong for its collision volume. The low bound allows hollow
// objects modelled as solids (an empty carton); the high one is above
// tungsten.
var densityBounds = [2]float64{5, 25000}
