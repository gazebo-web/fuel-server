package main

import (
	"encoding/xml"
	"math"
	"strconv"
	"strings"
)

// The size and place of what the agent built from a floor plan or a map:
// the box, seen from above, around the world's tall shapes (walls, racks,
// shelves), so floors and ground planes don't count. Boxes and cylinders
// are measured, in inline models and in included project models; meshes
// and Fuel models are not.

// tallShape is how tall (m) a shape must be to count.
const tallShape = 1.0

type xnode struct {
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
	Text    string     `xml:",chardata"`
	Nodes   []xnode    `xml:",any"`
}

func (n *xnode) all(name string) []*xnode {
	var out []*xnode
	for i := range n.Nodes {
		if n.Nodes[i].XMLName.Local == name {
			out = append(out, &n.Nodes[i])
		}
	}
	return out
}

func (n *xnode) first(name string) *xnode {
	if all := n.all(name); len(all) > 0 {
		return all[0]
	}
	return nil
}

func (n *xnode) text(name string) string {
	if c := n.first(name); c != nil {
		return strings.TrimSpace(c.Text)
	}
	return ""
}

// xform is a rotation and a translation.
type xform struct {
	r [3][3]float64
	t [3]float64
}

var identity = xform{r: [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}}

func (a xform) mul(b xform) xform {
	var out xform
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			for k := 0; k < 3; k++ {
				out.r[i][j] += a.r[i][k] * b.r[k][j]
			}
		}
		out.t[i] = a.t[i]
		for k := 0; k < 3; k++ {
			out.t[i] += a.r[i][k] * b.t[k]
		}
	}
	return out
}

func (a xform) apply(p [3]float64) [3]float64 {
	var out [3]float64
	for i := 0; i < 3; i++ {
		out[i] = a.t[i] + a.r[i][0]*p[0] + a.r[i][1]*p[1] + a.r[i][2]*p[2]
	}
	return out
}

// pose is an element's <pose> (x y z roll pitch yaw), relative to its
// parent.
func pose(n *xnode) xform {
	p := n.first("pose")
	if p == nil {
		return identity
	}
	v := floats(p.Text)
	if len(v) != 6 {
		return identity
	}
	for _, a := range p.Attrs {
		if a.Name.Local == "degrees" && a.Value == "true" {
			for i := 3; i < 6; i++ {
				v[i] *= math.Pi / 180
			}
		}
	}
	cr, sr, cp, sp, cy, sy := math.Cos(v[3]), math.Sin(v[3]), math.Cos(v[4]), math.Sin(v[4]), math.Cos(v[5]), math.Sin(v[5])
	return xform{r: [3][3]float64{
		{cy * cp, cy*sp*sr - sy*cr, cy*sp*cr + sy*sr},
		{sy * cp, sy*sp*sr + cy*cr, sy*sp*cr - cy*sr},
		{-sp, cp * sr, cp * cr},
	}, t: [3]float64{v[0], v[1], v[2]}}
}

func floats(s string) []float64 {
	var out []float64
	for _, f := range strings.Fields(s) {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// footprint is the box, from above, around a world's tall shapes.
type footprint struct {
	min, max [2]float64
	shapes   int
}

func (f *footprint) add(lo, hi [3]float64) {
	if hi[2]-lo[2] < tallShape {
		return
	}
	if f.shapes == 0 {
		f.min, f.max = [2]float64{lo[0], lo[1]}, [2]float64{hi[0], hi[1]}
	}
	for i := 0; i < 2; i++ {
		f.min[i], f.max[i] = math.Min(f.min[i], lo[i]), math.Max(f.max[i], hi[i])
	}
	f.shapes++
}

func (f footprint) size() [2]float64 { return [2]float64{f.max[0] - f.min[0], f.max[1] - f.min[1]} }

// worldFootprint measures a world's tall shapes; load reads included
// project models.
func worldFootprint(world string, load modelLoader) footprint {
	var f footprint
	var root xnode
	if xml.Unmarshal([]byte(world), &root) != nil {
		return f
	}
	w := root.first("world")
	if w == nil {
		return f
	}
	walkModel(w, identity, load, map[string]bool{}, &f)
	return f
}

// walkModel adds the shapes of a world or model, placed by t.
func walkModel(m *xnode, t xform, load modelLoader, inside map[string]bool, f *footprint) {
	for _, nested := range m.all("model") {
		walkModel(nested, t.mul(pose(nested)), load, inside, f)
	}
	for _, inc := range m.all("include") {
		name, ok := strings.CutPrefix(inc.text("uri"), "model://")
		name = strings.TrimSuffix(name, "/")
		if !ok || inside[name] || load == nil {
			continue
		}
		src, ok := load(name)
		var doc xnode
		if !ok || xml.Unmarshal([]byte(src), &doc) != nil || doc.first("model") == nil {
			continue
		}
		inside[name] = true
		walkModel(doc.first("model"), t.mul(pose(inc)), load, inside, f)
		delete(inside, name)
	}
	for _, link := range m.all("link") {
		lt := t.mul(pose(link))
		for _, v := range link.all("visual") {
			addShape(v, lt.mul(pose(v)), f)
		}
	}
}

// addShape adds a visual's box or cylinder.
func addShape(v *xnode, t xform, f *footprint) {
	g := v.first("geometry")
	if g == nil {
		return
	}
	var half [3]float64
	if b := g.first("box"); b != nil {
		s := floats(b.text("size"))
		if len(s) != 3 {
			return
		}
		half = [3]float64{s[0] / 2, s[1] / 2, s[2] / 2}
	} else if c := g.first("cylinder"); c != nil {
		r, l := floats(c.text("radius")), floats(c.text("length"))
		if len(r) != 1 || len(l) != 1 {
			return
		}
		half = [3]float64{r[0], r[0], l[0] / 2}
	} else {
		return
	}
	lo := [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi := [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, sx := range []float64{-1, 1} {
		for _, sy := range []float64{-1, 1} {
			for _, sz := range []float64{-1, 1} {
				p := t.apply([3]float64{sx * half[0], sy * half[1], sz * half[2]})
				for i := range p {
					lo[i], hi[i] = math.Min(lo[i], p[i]), math.Max(hi[i], p[i])
				}
			}
		}
	}
	f.add(lo, hi)
}
