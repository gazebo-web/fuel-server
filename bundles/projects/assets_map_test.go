package projects

import (
	"bytes"
	"image"
	"image/jpeg"
	"strings"
	"testing"
)

// testMap is a 40×20 map at 0.05 m/px (2 × 1 m): unknown grey around a
// free room, with a wall (black) along its top edge.
func testMap() *image.Gray {
	img := image.NewGray(image.Rect(0, 0, 40, 20))
	for i := range img.Pix {
		img.Pix[i] = 205 // unknown
	}
	for y := 2; y < 18; y++ {
		for x := 4; x < 36; x++ {
			img.Pix[y*40+x] = 254 // free
		}
	}
	for x := 4; x < 36; x++ {
		img.Pix[2*40+x] = 0 // wall
	}
	return img
}

const testMapYAML = "image: /home/robot/maps/room.pgm\nresolution: 0.05\norigin: [-1.0, -0.5, 0.0]\n" +
	"negate: 0\noccupied_thresh: 0.65\nfree_thresh: 0.196\n"

func TestPGM(t *testing.T) {
	img := testMap()
	got, format, err := image.Decode(bytes.NewReader(encodePGM(img)))
	if err != nil || format != "pgm" {
		t.Fatalf("%v %s", err, format)
	}
	if g := got.(*image.Gray); !bytes.Equal(g.Pix, img.Pix) {
		t.Error("pixels differ")
	}
	plain := "P2\n# a comment\n3 2\n15\n0 15 7\n15 0 15\n"
	got, _, err = image.Decode(strings.NewReader(plain))
	if err != nil || got.(*image.Gray).Pix[1] != 255 || got.(*image.Gray).Pix[2] != 119 {
		t.Fatalf("P2: %v %v", err, got)
	}
	sixteen := append([]byte("P5 2 1 65535\n"), 0xff, 0xff, 0x00, 0x00)
	if got, _, err = image.Decode(bytes.NewReader(sixteen)); err != nil || got.(*image.Gray).Pix[0] != 255 {
		t.Fatalf("16-bit: %v", err)
	}
	if _, _, err := image.Decode(strings.NewReader("P5 4 4 255\nab")); err == nil {
		t.Error("truncated PGM accepted")
	}
	if kind, mime, _, err := sniffAsset(encodePGM(img)); kind != AssetImage || mime != pgmMime || err != nil {
		t.Errorf("sniff: %s %s %v", kind, mime, err)
	}
}

func TestMapUpload(t *testing.T) {
	pgm := encodePGM(testMap())
	pair := zipBytes(t, map[string]string{"maps/room.yaml": testMapYAML, "maps/room.pgm": string(pgm)})
	kind, _, info, err := sniffAsset(pair)
	if err != nil || kind != AssetMap || info["main"] != "maps/room.yaml" {
		t.Fatalf("sniff: %s %v %v", kind, info, err)
	}
	views, info, err := processMap(pair)
	if err != nil {
		t.Fatal(err)
	}
	if info["resolution"] != 0.05 || info["width"] != 40 || info["height"] != 20 {
		t.Errorf("info: %v", info)
	}
	if got := info["size_m"].([]float64); got[0] != 2 || got[1] != 1 {
		t.Errorf("size: %v", got)
	}
	ext := info["extent"].(map[string]any)
	if ext["min"].([]float64)[0] != -1 || ext["max"].([]float64)[1] != 0.5 {
		t.Errorf("extent: %v", ext)
	}
	// The wall is row 2: y = -0.5 + (20 - 2 - 0.5)·0.05 = 0.375; x from -0.775 to 0.775.
	obs := info["obstacle_extent"].(map[string]any)
	if lo, hi := obs["min"].([]float64), obs["max"].([]float64); lo[0] != -0.775 || hi[0] != 0.775 || lo[1] != 0.375 || hi[1] != 0.375 {
		t.Errorf("obstacles: %v", obs)
	}
	if info["occupied"].(float64) <= 0 || info["unknown"].(float64) <= 0 || info["free"].(float64) <= 0 {
		t.Errorf("fractions: %v", info)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(views["ai.jpg"]))
	if err != nil || cfg.Width != AIViewSize {
		t.Errorf("ai view scaled up to %d: %v", cfg.Width, err)
	}
	xl, yl, ok := MapGridLabels(info)
	// This map's margin reaches its edges, so the view is all of it.
	if !ok || xl(0) != "x -1.0 m" || xl(0.5) != "x 0.0 m" || yl(0) != "y 0.5 m" || yl(1) != "y -0.5 m" {
		t.Errorf("labels: %v %q %q %q %q", ok, xl(0), xl(0.5), yl(0), yl(1))
	}

	// A map that is mostly unknown border: the view is the mapped part.
	big := image.NewGray(image.Rect(0, 0, 400, 300))
	for i := range big.Pix {
		big.Pix[i] = 205
	}
	for y := 100; y < 150; y++ {
		for x := 200; x < 300; x++ {
			big.Pix[y*400+x] = 254
		}
	}
	_, binfo, err := processMap(zipBytes(t, map[string]string{"m.yaml": "image: m.pgm\nresolution: 0.1\norigin: [0, 0, 0]\n",
		"m.pgm": string(encodePGM(big))}))
	if err != nil {
		t.Fatal(err)
	}
	if v := binfo["view_px"].([]int); v[0] != 196 || v[1] != 96 || v[2] != 304 || v[3] != 154 {
		t.Errorf("view: %v", v)
	}
	bx, by, _ := MapGridLabels(binfo)
	if bx(0) != "x 19.6 m" || by(0) != "y 20.4 m" {
		t.Errorf("cropped labels: %q %q", bx(0), by(0))
	}
	if _, err := GridOverlayLabels(views["ai.jpg"], xl, yl); err != nil {
		t.Error(err)
	}

	// In a zip of other things, the YAML finds its image; alone, a YAML is
	// refused with a hint.
	data := zipBytes(t, map[string]string{"site/maps/room.yaml": testMapYAML, "site/maps/room.pgm": string(pgm),
		"site/photo.png": string(testPNG(t, 4, 4))})
	ups, _, skipped, err := expandBundle("site.zip", data)
	if err != nil || len(ups) != 2 || len(skipped) != 0 {
		t.Fatalf("bundle: %v %v %v", len(ups), skipped, err)
	}
	var mapUp AssetUpload
	for _, u := range ups {
		if u.Name == "room.yaml" {
			mapUp = u
		}
	}
	if kind, _, _, _ := sniffAsset(mapUp.Data); kind != AssetMap {
		t.Errorf("unpacked map sniffs as %q", kind)
	}
	if !strings.Contains(strings.Join(zipEntries(t, mapUp.Data), ","), "room.pgm") {
		t.Error("the map lost its image")
	}
	if _, _, _, err := sniffAsset([]byte(testMapYAML)); err == nil || !strings.Contains(err.Error(), "with its image") {
		t.Errorf("YAML alone: %v", err)
	}
}

func TestASCIIStorage(t *testing.T) {
	info := AssetInfo{"text": "Raum 4,2 m × 3 m − café 🪑"}
	v, err := info.Value()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range v.(string) {
		if r >= 0x80 {
			t.Fatalf("not ASCII: %s", v)
		}
	}
	var back AssetInfo
	if err := back.Scan(v); err != nil || back["text"] != info["text"] {
		t.Fatalf("round trip: %v %v", back, err)
	}
	e := AssetError("the image is 9000×9000")
	ev, _ := e.Value()
	var eb AssetError
	if err := eb.Scan(ev); err != nil || eb != e || strings.ContainsRune(ev.(string), '×') {
		t.Fatalf("error round trip: %q %q %v", ev, eb, err)
	}
	if v, _ := AssetError("plain").Value(); v != "plain" {
		t.Errorf("plain text changed: %v", v)
	}
}

func TestRotatedMap(t *testing.T) {
	// testMap turned a quarter turn: the image's width runs along +y and its
	// height along -x, so x = -1 - v and y = -0.5 + u.
	yaml := "image: room.pgm\nresolution: 0.05\norigin: [-1.0, -0.5, 1.5707963267948966]\n"
	views, info, err := processMap(zipBytes(t, map[string]string{"room.yaml": yaml,
		"room.pgm": string(encodePGM(testMap()))}))
	if err != nil {
		t.Fatal(err)
	}
	ext := info["extent"].(map[string]any)
	if lo, hi := ext["min"].([]float64), ext["max"].([]float64); lo[0] != -2 || lo[1] != -0.5 || hi[0] != -1 || hi[1] != 1.5 {
		t.Errorf("extent: %v", ext)
	}
	// The wall (row 2, v = 0.875) runs along y at x = -1.875.
	obs := info["obstacle_extent"].(map[string]any)
	if lo, hi := obs["min"].([]float64), obs["max"].([]float64); lo[0] != -1.875 || hi[0] != -1.875 || lo[1] != -0.275 || hi[1] != 1.275 {
		t.Errorf("obstacles: %v", obs)
	}
	if _, ok := info["view_px"]; ok {
		t.Error("a turned view has no pixel crop")
	}
	if !strings.Contains(info["reading"].(string), "cos(yaw)") {
		t.Errorf("reading: %s", info["reading"])
	}
	xl, yl, ok := MapGridLabels(info)
	if !ok || xl(0) != "x -2.0 m" || xl(1) != "x -1.0 m" || yl(0) != "y 1.5 m" || yl(1) != "y -0.5 m" {
		t.Errorf("labels: %v %q %q %q %q", ok, xl(0), xl(1), yl(0), yl(1))
	}
	if _, err := jpeg.DecodeConfig(bytes.NewReader(views["ai.jpg"])); err != nil {
		t.Error(err)
	}

	// The turned picture is 1 m wide and 2 m tall; the wall is column 2,
	// rows 4 to 35, with free floor beside it.
	m, _ := parseNavMap([]byte(yaml))
	turned := turnMap(m, testMap(), []float64{-2, -0.5}, []float64{-1, 1.5}).(*image.Gray)
	if b := turned.Bounds(); b.Dx() != 20 || b.Dy() != 40 {
		t.Fatalf("size: %v", b)
	}
	if turned.GrayAt(2, 4).Y != 0 || turned.GrayAt(2, 35).Y != 0 || turned.GrayAt(3, 20).Y != 254 ||
		turned.GrayAt(2, 3).Y != 205 || turned.GrayAt(0, 0).Y != 205 {
		t.Errorf("pixels: %d %d %d %d %d", turned.GrayAt(2, 4).Y, turned.GrayAt(2, 35).Y,
			turned.GrayAt(3, 20).Y, turned.GrayAt(2, 3).Y, turned.GrayAt(0, 0).Y)
	}
}
