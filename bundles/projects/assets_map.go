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

	// The AI's view is the mapped part (view_px), at least AIViewSize on its
	// long side (small maps are scaled up, cells kept sharp), so the grid's
	// labels fit.
	var crop image.Image = src
	if v := info["view_px"].([]int); true {
		r := image.Rect(v[0], v[1], v[2], v[3]).Add(src.Bounds().Min)
		if sub, ok := src.(interface {
			SubImage(image.Rectangle) image.Image
		}); ok && r != src.Bounds() {
			crop = sub.SubImage(r)
		}
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

// mapInfo measures a map: its size and extent in meters, how much of it is
// free, occupied and unknown, and the extent of the obstacles.
func mapInfo(m *navMap, img image.Image) AssetInfo {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	r := m.Resolution
	ox, oy, yaw := m.Origin[0], m.Origin[1], m.Origin[2]
	occ, free := 0.65, 0.196
	if m.OccupiedThresh != nil {
		occ = *m.OccupiedThresh
	}
	if m.FreeThresh != nil {
		free = *m.FreeThresh
	}
	var nOcc, nFree int
	minC, minR, maxC, maxR := w, h, -1, -1
	kc0, kr0, kc1, kr1 := w, h, -1, -1 // the known (free or occupied) cells
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g := color8(img, b.Min.X+x, b.Min.Y+y)
			// nav2_map_server: occupancy is (255 - v)/255, or v/255 negated.
			p := float64(255-g) / 255
			if m.Negate != 0 {
				p = float64(g) / 255
			}
			switch {
			case p > occ:
				nOcc++
				minC, maxC = min(minC, x), max(maxC, x)
				minR, maxR = min(minR, y), max(maxR, y)
			case p < free:
				nFree++
			default:
				continue
			}
			kc0, kc1, kr0, kr1 = min(kc0, x), max(kc1, x), min(kr0, y), max(kr1, y)
		}
	}
	round := func(v float64) float64 { return math.Round(v*1000) / 1000 }
	world := func(c, row float64) []float64 { // cell center; row 0 is the top (max y)
		return []float64{round(ox + (c+0.5)*r), round(oy + (float64(h)-row-0.5)*r)}
	}
	total := float64(w * h)
	info := AssetInfo{
		"width": w, "height": h,
		"resolution": r,
		"origin":     []float64{ox, oy, yaw},
		"size_m":     []float64{round(float64(w) * r), round(float64(h) * r)},
		"extent":     map[string]any{"min": []float64{round(ox), round(oy)}, "max": []float64{round(ox + float64(w)*r), round(oy + float64(h)*r)}},
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
	if maxC >= 0 {
		lo, hi := world(float64(minC), float64(maxR)), world(float64(maxC), float64(minR))
		info["obstacle_extent"] = map[string]any{"min": lo, "max": hi}
	}
	// The picture shows the mapped part, a little margin around it.
	view := image.Rect(0, 0, w, h)
	if kc1 >= 0 {
		m := max(4, (kc1-kc0+kr1-kr0)/50)
		view = image.Rect(max(0, kc0-m), max(0, kr0-m), min(w, kc1+1+m), min(h, kr1+1+m))
	}
	info["view_px"] = []int{view.Min.X, view.Min.Y, view.Max.X, view.Max.Y}
	info["view_extent"] = map[string]any{
		"min": []float64{round(ox + float64(view.Min.X)*r), round(oy + float64(h-view.Max.Y)*r)},
		"max": []float64{round(ox + float64(view.Max.X)*r), round(oy + float64(h-view.Min.Y)*r)},
	}
	if yaw != 0 {
		info["reading"] = info["reading"].(string) + fmt.Sprintf(" The map is rotated by yaw %g rad about its "+
			"origin; the extent and the grid's labels ignore that rotation.", yaw)
	}
	return info
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
