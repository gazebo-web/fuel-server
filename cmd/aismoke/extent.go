package main

import (
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"math"
	"path"
	"strconv"
	"strings"
)

// The size and place of what the agent built from a floor plan or a map:
// the box, seen from above, around the world's tall shapes (walls, racks,
// shelves), so floors and ground planes don't count. Boxes, cylinders and
// GLB meshes (what model_generate and model_import make) are measured, in
// inline models and in included project models; Fuel models are not.

// fileLoader reads a project file by its path in the project.
type fileLoader func(path string) ([]byte, bool)

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

// footprint is the box, from above, around a world's tall shapes, and
// each shape's own box.
type footprint struct {
	min, max [2]float64
	shapes   int
	boxes    [][4]float64 // min x, min y, max x, max y
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
	f.boxes = append(f.boxes, [4]float64{lo[0], lo[1], hi[0], hi[1]})
}

// covers reports whether a tall shape stands on the point (x, y).
func (f footprint) covers(x, y float64) bool {
	for _, b := range f.boxes {
		if x >= b[0] && x <= b[2] && y >= b[1] && y <= b[3] {
			return true
		}
	}
	return false
}

func (f footprint) size() [2]float64 { return [2]float64{f.max[0] - f.min[0], f.max[1] - f.min[1]} }

// worldFootprint measures a world's tall shapes; files reads included
// project models and their meshes.
func worldFootprint(world string, files fileLoader) footprint {
	var f footprint
	var root xnode
	if xml.Unmarshal([]byte(world), &root) != nil {
		return f
	}
	w := root.first("world")
	if w == nil {
		return f
	}
	walkModel(w, identity, "", files, map[string]bool{}, &f)
	return f
}

// walkModel adds the shapes of a world or model, placed by t; dir is the
// model's folder in the project.
func walkModel(m *xnode, t xform, dir string, files fileLoader, inside map[string]bool, f *footprint) {
	for _, nested := range m.all("model") {
		walkModel(nested, t.mul(pose(nested)), dir, files, inside, f)
	}
	for _, inc := range m.all("include") {
		name, ok := strings.CutPrefix(inc.text("uri"), "model://")
		name = strings.TrimSuffix(name, "/")
		if !ok || inside[name] || files == nil {
			continue
		}
		src, ok := files("models/" + name + "/model.sdf")
		var doc xnode
		if !ok || xml.Unmarshal(src, &doc) != nil || doc.first("model") == nil {
			continue
		}
		inside[name] = true
		walkModel(doc.first("model"), t.mul(pose(inc)), "models/"+name, files, inside, f)
		delete(inside, name)
	}
	for _, link := range m.all("link") {
		lt := t.mul(pose(link))
		for _, v := range link.all("visual") {
			addShape(v, lt.mul(pose(v)), dir, files, f)
		}
	}
}

// addShape adds a visual's box, cylinder or GLB mesh.
func addShape(v *xnode, t xform, dir string, files fileLoader, f *footprint) {
	g := v.first("geometry")
	if g == nil {
		return
	}
	var lo, hi [3]float64 // in the visual's frame
	if b := g.first("box"); b != nil {
		s := floats(b.text("size"))
		if len(s) != 3 {
			return
		}
		lo, hi = [3]float64{-s[0] / 2, -s[1] / 2, -s[2] / 2}, [3]float64{s[0] / 2, s[1] / 2, s[2] / 2}
	} else if c := g.first("cylinder"); c != nil {
		r, l := floats(c.text("radius")), floats(c.text("length"))
		if len(r) != 1 || len(l) != 1 {
			return
		}
		lo, hi = [3]float64{-r[0], -r[0], -l[0] / 2}, [3]float64{r[0], r[0], l[0] / 2}
	} else if m := g.first("mesh"); m != nil && files != nil {
		data, ok := files(meshPath(m.text("uri"), dir))
		if !ok {
			return
		}
		if lo, hi, ok = glbBounds(data); !ok {
			return
		}
		if s := floats(m.text("scale")); len(s) == 3 {
			for i := range lo {
				lo[i], hi[i] = math.Min(lo[i]*s[i], hi[i]*s[i]), math.Max(lo[i]*s[i], hi[i]*s[i])
			}
		}
	} else {
		return
	}
	f.add(boxIn(t, lo, hi))
}

// meshPath is a mesh URI's file in the project: model://<name>/... or
// relative to the model's folder.
func meshPath(uri, dir string) string {
	if rest, ok := strings.CutPrefix(uri, "model://"); ok {
		return "models/" + rest
	}
	if strings.Contains(uri, "://") {
		return ""
	}
	return path.Join(dir, uri)
}

// boxIn is the box, in t's parent frame, around the box lo..hi in t's.
func boxIn(t xform, lo, hi [3]float64) (outLo, outHi [3]float64) {
	outLo = [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	outHi = [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for _, x := range []float64{lo[0], hi[0]} {
		for _, y := range []float64{lo[1], hi[1]} {
			for _, z := range []float64{lo[2], hi[2]} {
				p := t.apply([3]float64{x, y, z})
				for i := range p {
					outLo[i], outHi[i] = math.Min(outLo[i], p[i]), math.Max(outHi[i], p[i])
				}
			}
		}
	}
	return outLo, outHi
}

// glbBounds is the box around a binary glTF's meshes in the default
// scene, from the min and max glTF requires on POSITION accessors, with
// node transforms applied.
func glbBounds(data []byte) (lo, hi [3]float64, ok bool) {
	if len(data) < 20 || string(data[:4]) != "glTF" || string(data[16:20]) != "JSON" {
		return lo, hi, false
	}
	n := int(binary.LittleEndian.Uint32(data[12:16]))
	if 20+n > len(data) {
		return lo, hi, false
	}
	var doc struct {
		Scene  int `json:"scene"`
		Scenes []struct {
			Nodes []int `json:"nodes"`
		} `json:"scenes"`
		Nodes []struct {
			Mesh        *int      `json:"mesh"`
			Children    []int     `json:"children"`
			Matrix      []float64 `json:"matrix"`
			Translation []float64 `json:"translation"`
			Rotation    []float64 `json:"rotation"`
			Scale       []float64 `json:"scale"`
		} `json:"nodes"`
		Meshes []struct {
			Primitives []struct {
				Attributes map[string]int `json:"attributes"`
			} `json:"primitives"`
		} `json:"meshes"`
		Accessors []struct {
			Min []float64 `json:"min"`
			Max []float64 `json:"max"`
		} `json:"accessors"`
	}
	if json.Unmarshal(data[20:20+n], &doc) != nil || doc.Scene >= len(doc.Scenes) {
		return lo, hi, false
	}
	lo = [3]float64{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi = [3]float64{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	var visit func(i int, t xform, depth int)
	visit = func(i int, t xform, depth int) {
		if i < 0 || i >= len(doc.Nodes) || depth > 32 {
			return
		}
		nd := doc.Nodes[i]
		local := identity
		switch {
		case len(nd.Matrix) == 16: // column-major
			for r := 0; r < 3; r++ {
				for c := 0; c < 3; c++ {
					local.r[r][c] = nd.Matrix[c*4+r]
				}
				local.t[r] = nd.Matrix[12+r]
			}
		default:
			if len(nd.Rotation) == 4 {
				x, y, z, w := nd.Rotation[0], nd.Rotation[1], nd.Rotation[2], nd.Rotation[3]
				local.r = [3][3]float64{
					{1 - 2*(y*y+z*z), 2 * (x*y - z*w), 2 * (x*z + y*w)},
					{2 * (x*y + z*w), 1 - 2*(x*x+z*z), 2 * (y*z - x*w)},
					{2 * (x*z - y*w), 2 * (y*z + x*w), 1 - 2*(x*x+y*y)},
				}
			}
			if len(nd.Scale) == 3 {
				for r := 0; r < 3; r++ {
					for c := 0; c < 3; c++ {
						local.r[r][c] *= nd.Scale[c]
					}
				}
			}
			if len(nd.Translation) == 3 {
				local.t = [3]float64{nd.Translation[0], nd.Translation[1], nd.Translation[2]}
			}
		}
		t = t.mul(local)
		if nd.Mesh != nil && *nd.Mesh >= 0 && *nd.Mesh < len(doc.Meshes) {
			for _, p := range doc.Meshes[*nd.Mesh].Primitives {
				a, has := p.Attributes["POSITION"]
				if !has || a < 0 || a >= len(doc.Accessors) {
					continue
				}
				acc := doc.Accessors[a]
				if len(acc.Min) != 3 || len(acc.Max) != 3 {
					continue
				}
				l, h := boxIn(t, [3]float64(acc.Min), [3]float64(acc.Max))
				for k := 0; k < 3; k++ {
					lo[k], hi[k] = math.Min(lo[k], l[k]), math.Max(hi[k], h[k])
				}
				ok = true
			}
		}
		for _, c := range nd.Children {
			visit(c, t, depth+1)
		}
	}
	for _, i := range doc.Scenes[doc.Scene].Nodes {
		visit(i, identity, 0)
	}
	return lo, hi, ok
}
