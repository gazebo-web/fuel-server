package projects

import (
	"fmt"
	"image"
	"math"
	"path"
	"regexp"
	"strings"

	"golang.org/x/image/draw"
	"gopkg.in/yaml.v3"
)

// Occupancy maps: a ROS/Nav2 map YAML and its image (PGM or PNG), as
// SLAM writes them. The YAML gives the image its size and place in the
// world, so the pair is one project file of kind map: its info has the
// scale, origin, extent and where the obstacles are, in meters, and the
// AI's grid is labelled in meters.

// AssetMap is an occupancy map (YAML + image) with its scale and origin.
const AssetMap = "map"

// navMap is a map YAML (nav2_map_server's format).
type navMap struct {
	Image          string    `yaml:"image"`
	Resolution     float64   `yaml:"resolution"`
	Origin         []float64 `yaml:"origin"`
	Negate         int       `yaml:"negate"`
	OccupiedThresh *float64  `yaml:"occupied_thresh"`
	FreeThresh     *float64  `yaml:"free_thresh"`
	Mode           string    `yaml:"mode"`
}

const maxMapYAML = 64 << 10

// parseNavMap reads a map YAML: it must name an image and a positive
// resolution.
func parseNavMap(b []byte) (*navMap, bool) {
	if len(b) > maxMapYAML {
		return nil, false
	}
	var m navMap
	if yaml.Unmarshal(b, &m) != nil || strings.TrimSpace(m.Image) == "" || !(m.Resolution > 0) {
		return nil, false
	}
	for len(m.Origin) < 3 {
		m.Origin = append(m.Origin, 0)
	}
	return &m, true
}

func isYAML(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

// mapImage is the path of a map YAML's image in a zip, if it is there.
func mapImage(yamlPath string, m *navMap, has func(string) bool) (string, bool) {
	p := path.Clean(path.Join(path.Dir(yamlPath), strings.ReplaceAll(m.Image, "\\", "/")))
	if has(p) {
		return p, true
	}
	// Absolute paths from the machine that made the map: try the name.
	p = path.Join(path.Dir(yamlPath), path.Base(m.Image))
	return p, has(p)
}

var yamlImageRe = regexp.MustCompile(`(?m)^(\s*image\s*:\s*).*$`)

// mapPair is a map's YAML and image as a zip of two files side by side,
// the YAML's image line pointing at the image.
func mapPair(yamlName string, yamlData []byte, imageName string, imageData []byte) ([]byte, error) {
	img := path.Base(imageName)
	y := yamlImageRe.ReplaceAll(yamlData, []byte("${1}"+img))
	return zipOf(map[string][]byte{path.Base(yamlName): y, img: imageData})
}

// processMap reads a map zip (a YAML and its image) and returns its views
// and info.
func processMap(data []byte) (map[string][]byte, AssetInfo, error) {
	files, err := readZip(data)
	if err != nil {
		return nil, nil, err
	}
	byName := map[string][]byte{}
	for _, f := range files {
		byName[f.name] = f.data
	}
	var m *navMap
	var yamlName, imageName string
	for _, f := range files {
		if mm, ok := parseNavMap(f.data); ok && isYAML(f.name) {
			if p, ok := mapImage(f.name, mm, func(n string) bool { return byName[n] != nil }); ok {
				m, yamlName, imageName = mm, f.name, p
				break
			}
		}
	}
	if m == nil {
		return nil, nil, fmt.Errorf("no map YAML with its image")
	}
	src, err := DecodeImage(byName[imageName])
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %v", imageName, err)
	}
	info := mapInfo(m, src)
	info["yaml_file"], info["image_file"] = yamlName, imageName
	if y := byName[yamlName]; len(y) <= 4096 {
		info["yaml"] = string(y)
	}

	// The AI's view is the mapped part, at least AIViewSize on its long side
	// (small maps are scaled up, cells kept sharp), so the grid's labels fit.
	// A rotated map is turned to line up with the map frame.
	var crop image.Image = src
	if v, ok := info["view_px"].([]int); ok {
		r := image.Rect(v[0], v[1], v[2], v[3]).Add(src.Bounds().Min)
		if sub, ok := src.(interface {
			SubImage(image.Rectangle) image.Image
		}); ok && r != src.Bounds() {
			crop = sub.SubImage(r)
		}
	} else {
		crop = turnMap(m, src, nums(info["view_extent"].(map[string]any)["min"]),
			nums(info["view_extent"].(map[string]any)["max"]))
	}
	b := crop.Bounds()
	scale := float64(AIViewSize) / float64(max(b.Dx(), b.Dy()))
	var ai image.Image = crop
	if scale > 1 {
		w, h := int(math.Round(float64(b.Dx())*scale)), int(math.Round(float64(b.Dy())*scale))
		dst := image.NewGray(image.Rect(0, 0, w, h))
		draw.NearestNeighbor.Scale(dst, dst.Bounds(), crop, b, draw.Src, nil)
		ai = dst
	}
	aiJPG, err := encodeJPEG(flatten(fit(ai, AIViewSize)), 90)
	if err != nil {
		return nil, nil, err
	}
	thumb, err := encodeJPEG(flatten(fit(crop, thumbSize)), 80)
	if err != nil {
		return nil, nil, err
	}
	return map[string][]byte{"ai.jpg": aiJPG, "thumb.jpg": thumb}, info, nil
}

// mapFrame places a map image in the map frame: the YAML's origin is the
// image's lower-left corner, turned by its yaw. Columns and rows count
// cells from the image's top left, and may be fractional.
type mapFrame struct {
	ox, oy, r, cos, sin float64
	h                   int
}

func newMapFrame(m *navMap, h int) mapFrame {
	return mapFrame{ox: m.Origin[0], oy: m.Origin[1], r: m.Resolution,
		cos: math.Cos(m.Origin[2]), sin: math.Sin(m.Origin[2]), h: h}
}

// world is the map-frame point, in meters, of an image position.
func (f mapFrame) world(c, row float64) (x, y float64) {
	u, v := c*f.r, (float64(f.h)-row)*f.r
	return f.ox + f.cos*u - f.sin*v, f.oy + f.sin*u + f.cos*v
}

// cell is the image position of a map-frame point.
func (f mapFrame) cell(x, y float64) (c, row float64) {
	dx, dy := x-f.ox, y-f.oy
	return (f.cos*dx + f.sin*dy) / f.r, float64(f.h) - (-f.sin*dx+f.cos*dy)/f.r
}

// bounds is an axis-aligned box in the map frame.
type bounds struct{ x0, y0, x1, y1 float64 }

func emptyBounds() bounds { return bounds{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)} }

func (b *bounds) add(x, y float64) {
	b.x0, b.y0, b.x1, b.y1 = min(b.x0, x), min(b.y0, y), max(b.x1, x), max(b.y1, y)
}

func (b bounds) empty() bool { return b.x0 > b.x1 }

// mapInfo measures a map: its size and extent in meters, how much of it is
// free, occupied and unknown, and the extent of the obstacles. Extents are
// in the map frame, so a rotated map's are the boxes around its turned cells.
func mapInfo(m *navMap, img image.Image) AssetInfo {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	r := m.Resolution
	ox, oy, yaw := m.Origin[0], m.Origin[1], m.Origin[2]
	f := newMapFrame(m, h)
	occ, free := 0.65, 0.196
	if m.OccupiedThresh != nil {
		occ = *m.OccupiedThresh
	}
	if m.FreeThresh != nil {
		free = *m.FreeThresh
	}
	var nOcc, nFree int
	obstacles, known := emptyBounds(), emptyBounds() // cell centers
	kc0, kr0, kc1, kr1 := w, h, -1, -1               // the known (free or occupied) cells
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g := color8(img, b.Min.X+x, b.Min.Y+y)
			// nav2_map_server: occupancy is (255 - v)/255, or v/255 negated.
			p := float64(255-g) / 255
			if m.Negate != 0 {
				p = float64(g) / 255
			}
			cx, cy := f.world(float64(x)+0.5, float64(y)+0.5)
			switch {
			case p > occ:
				nOcc++
				obstacles.add(cx, cy)
			case p < free:
				nFree++
			default:
				continue
			}
			known.add(cx, cy)
			kc0, kc1, kr0, kr1 = min(kc0, x), max(kc1, x), min(kr0, y), max(kr1, y)
		}
	}
	round := func(v float64) float64 { return math.Round(v*1000) / 1000 }
	pair := func(x, y float64) []float64 { return []float64{round(x), round(y)} }
	extent := emptyBounds()
	for _, c := range [][2]float64{{0, 0}, {float64(w), 0}, {0, float64(h)}, {float64(w), float64(h)}} {
		extent.add(f.world(c[0], c[1]))
	}
	total := float64(w * h)
	info := AssetInfo{
		"width": w, "height": h,
		"resolution": r,
		"origin":     []float64{ox, oy, yaw},
		"size_m":     []float64{round(float64(w) * r), round(float64(h) * r)},
		"extent":     map[string]any{"min": pair(extent.x0, extent.y0), "max": pair(extent.x1, extent.y1)},
		"occupied":   round(float64(nOcc) / total),
		"free":       round(float64(nFree) / total),
		"unknown":    round(1 - float64(nOcc+nFree)/total),
		"negate":     m.Negate,
		"thresholds": map[string]any{"occupied": occ, "free": free},
		"reading": "Dark cells are obstacles (walls, racks), light cells free floor, grey unknown. The cell " +
			"in column c, row r (row 0 at the top) is at x = origin_x + (c + 0.5)·resolution, " +
			"y = origin_y + (height − r − 0.5)·resolution, in meters in the map frame. The picture shows " +
			"view_extent, the mapped part.",
	}
	if !obstacles.empty() {
		info["obstacle_extent"] = map[string]any{"min": pair(obstacles.x0, obstacles.y0),
			"max": pair(obstacles.x1, obstacles.y1)}
	}
	// The picture shows the mapped part, a little margin around it.
	margin := 4
	if kc1 >= 0 {
		margin = max(4, (kc1-kc0+kr1-kr0)/50)
	}
	if yaw == 0 {
		view := image.Rect(0, 0, w, h)
		if kc1 >= 0 {
			view = image.Rect(max(0, kc0-margin), max(0, kr0-margin), min(w, kc1+1+margin), min(h, kr1+1+margin))
		}
		info["view_px"] = []int{view.Min.X, view.Min.Y, view.Max.X, view.Max.Y}
		info["view_extent"] = map[string]any{
			"min": pair(ox+float64(view.Min.X)*r, oy+float64(h-view.Max.Y)*r),
			"max": pair(ox+float64(view.Max.X)*r, oy+float64(h-view.Min.Y)*r),
		}
		return info
	}
	// A rotated map's picture is turned to line up with the map frame. It
	// shows the box around the known cells, or the whole map if none is known.
	view := extent
	if !known.empty() {
		pad := (float64(margin) + 1) * r // the margin, plus the cells' own size
		view = bounds{known.x0 - pad, known.y0 - pad, known.x1 + pad, known.y1 + pad}
		view = bounds{max(view.x0, extent.x0), max(view.y0, extent.y0), min(view.x1, extent.x1), min(view.y1, extent.y1)}
	}
	info["view_extent"] = map[string]any{"min": pair(view.x0, view.y0), "max": pair(view.x1, view.y1)}
	info["reading"] = fmt.Sprintf("Dark cells are obstacles (walls, racks), light cells free floor, grey unknown. "+
		"The map image is rotated by yaw %g rad about its origin, its lower-left corner. The cell in column c, "+
		"row r (row 0 at the top of the image) is at u = (c + 0.5)·resolution, v = (height − r − 0.5)·resolution "+
		"along the image, so x = origin_x + u·cos(yaw) − v·sin(yaw), y = origin_y + u·sin(yaw) + v·cos(yaw), "+
		"in meters in the map frame. The picture is turned so its edges run along the map frame's x and y "+
		"axes, and shows view_extent, the mapped part. The extents are boxes in the map frame.", yaw)
	return info
}

// maxTurnedMap caps the long side of a rotated map's turned picture.
const maxTurnedMap = 4096

// turnMap draws the part of a rotated map between lo and hi (map-frame
// meters) lined up with the map frame, a cell per pixel up to maxTurnedMap.
// Points off the map are unknown: Nav2's grey 205, or 50 when negated.
func turnMap(m *navMap, img image.Image, lo, hi []float64) image.Image {
	unknown := uint8(205)
	if m.Negate != 0 {
		unknown = 50
	}
	b := img.Bounds()
	f := newMapFrame(m, b.Dy())
	px := max(m.Resolution, max(hi[0]-lo[0], hi[1]-lo[1])/maxTurnedMap)
	w := max(1, int(math.Ceil((hi[0]-lo[0])/px)))
	h := max(1, int(math.Ceil((hi[1]-lo[1])/px)))
	out := image.NewGray(image.Rect(0, 0, w, h))
	for j := 0; j < h; j++ {
		for i := 0; i < w; i++ {
			c, row := f.cell(lo[0]+(float64(i)+0.5)*px, hi[1]-(float64(j)+0.5)*px)
			x, y := int(math.Floor(c)), int(math.Floor(row))
			v := unknown
			if x >= 0 && y >= 0 && x < b.Dx() && y < b.Dy() {
				v = color8(img, b.Min.X+x, b.Min.Y+y)
			}
			out.Pix[j*out.Stride+i] = v
		}
	}
	return out
}

// color8 is a pixel's gray level, 0-255.
func color8(img image.Image, x, y int) uint8 {
	if g, ok := img.(*image.Gray); ok {
		return g.GrayAt(x, y).Y
	}
	r, gg, bb, _ := img.At(x, y).RGBA()
	return uint8((299*r + 587*gg + 114*bb) / 1000 >> 8)
}

// MapGridLabels label a map's AI view in meters: x along the top, y down
// the left, from its info's view_extent (the part of the map the view
// shows). ok is false when the info isn't a map's.
func MapGridLabels(info AssetInfo) (xLabel, yLabel func(frac float64) string, ok bool) {
	ext, _ := info["view_extent"].(map[string]any)
	lo, hi := nums(ext["min"]), nums(ext["max"])
	if len(lo) < 2 || len(hi) < 2 {
		return nil, nil, false
	}
	xLabel = func(f float64) string { return fmt.Sprintf("x %.1f m", lo[0]+f*(hi[0]-lo[0])) }
	yLabel = func(f float64) string { return fmt.Sprintf("y %.1f m", hi[1]-f*(hi[1]-lo[1])) }
	return xLabel, yLabel, true
}

func num(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	}
	return 0, false
}

func nums(v any) []float64 {
	switch x := v.(type) {
	case []float64:
		return x
	case []any:
		var out []float64
		for _, e := range x {
			if f, ok := num(e); ok {
				out = append(out, f)
			}
		}
		return out
	}
	return nil
}
