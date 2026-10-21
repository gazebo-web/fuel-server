package projects

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

func zipBytes(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, 0, color.NRGBA{255, 0, 0, 255}) // a red top row
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pngChunk(typ string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, typ...)
	out = append(out, data...)
	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(append([]byte(typ), data...)))
}

func TestSniffAsset(t *testing.T) {
	stl := make([]byte, 84+50)
	binary.LittleEndian.PutUint32(stl[80:], 1)
	cases := []struct {
		name, kind, mime string
		data             []byte
	}{
		{"png", AssetImage, "image/png", testPNG(t, 2, 2)},
		{"jpeg", AssetImage, "image/jpeg", []byte{0xff, 0xd8, 0xff, 0xe0}},
		{"webp", AssetImage, "image/webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 ")},
		{"pdf", AssetDocument, "application/pdf", []byte("%PDF-1.7\n")},
		{"glb", AssetMesh, "model/gltf-binary", []byte("glTF\x02\x00\x00\x00")},
		{"binary stl", AssetMesh, "model/stl", stl},
		{"ascii stl", AssetMesh, "model/stl", []byte("solid cube\n facet normal 0 0 1\n")},
		{"obj", AssetMesh, "model/obj", []byte("# chair\nv 0 0 0\nv 1 0 0\nv 0 1 0\nf 1 2 3\n")},
		{"dae", AssetMesh, "model/vnd.collada+xml", []byte(`<?xml version="1.0"?><COLLADA xmlns="x">`)},
		{"svg", AssetDrawing, "image/svg+xml", []byte(`<?xml version="1.0"?>` + "\n" + `<svg width="10">`)},
		{"dxf", AssetDrawing, "image/vnd.dxf", []byte("999\nmade by hand\n  0\nSECTION\n  2\nHEADER\n")},
		{"mesh zip", AssetMesh, "application/zip", zipBytes(t, map[string]string{
			"chair/chair.obj": "v 0 0 0", "chair/chair.mtl": "", "chair/wood.png": "", "__MACOSX/x": ""})},
		{"texture zip", AssetTexture, "application/zip", zipBytes(t, map[string]string{
			"brick_color.jpg": "", "brick_normal.png": "", "brick_roughness.jpg": ""})},
	}
	for _, c := range cases {
		kind, mime, _, err := sniffAsset(c.data)
		if err != nil || kind != c.kind || mime != c.mime {
			t.Errorf("%s: got %q %q %v, want %q %q", c.name, kind, mime, err, c.kind, c.mime)
		}
	}
	_, _, info, _ := sniffAsset(zipBytes(t, map[string]string{"m/a.obj": "", "m/a.mtl": ""}))
	if info["main"] != "m/a.obj" {
		t.Errorf("zip main = %v", info["main"])
	}

	bad := map[string][]byte{
		"empty":       nil,
		"exe":         []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00"),
		"plain text":  []byte("hello, this is a note"),
		"traversal":   zipBytes(t, map[string]string{"../evil.obj": ""}),
		"invalid zip": []byte("PK\x03\x04garbage"),
	}
	for name, data := range bad {
		if kind, _, _, err := sniffAsset(data); err == nil {
			t.Errorf("%s: accepted as %s", name, kind)
		}
	}

	// Several meshes, or a mix of files: a bundle, unpacked on upload.
	for name, data := range map[string][]byte{
		"two meshes": zipBytes(t, map[string]string{"a.obj": "", "b.stl": ""}),
		"mixed zip":  zipBytes(t, map[string]string{"notes.txt": "", "a.png": ""}),
	} {
		if kind, _, _, err := sniffAsset(data); kind != assetBundle || err != nil {
			t.Errorf("%s: %s %v", name, kind, err)
		}
	}

	// A zip whose entries claim to unpack past the limit.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i < 2; i++ {
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: strings.Repeat("x", i+1) + ".png", Method: zip.Deflate})
		_, _ = w.Write(make([]byte, MaxAssetZipUnpacked/2+1))
	}
	_ = zw.Close()
	if _, _, _, err := sniffAsset(buf.Bytes()); err == nil || !strings.Contains(err.Error(), "unpacks") {
		t.Errorf("zip bomb: %v", err)
	}
}

// exifJPEG is a w×h JPEG with an EXIF segment holding an orientation
// and a pretend GPS string.
func exifJPEG(t *testing.T, w, h, orientation int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < 6; y++ { // a red band at the top
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	tiff := []byte("MM\x00\x2a\x00\x00\x00\x08")
	tiff = binary.BigEndian.AppendUint16(tiff, 1)
	tiff = append(tiff, 0x01, 0x12, 0x00, 0x03, 0, 0, 0, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, uint16(orientation))
	tiff = append(tiff, 0, 0, 0, 0, 0, 0)
	tiff = append(tiff, "GPS 37.4N 122.1W"...)
	seg := append([]byte("Exif\x00\x00"), tiff...)
	app1 := append([]byte{0xff, 0xe1}, binary.BigEndian.AppendUint16(nil, uint16(len(seg)+2))...)
	app1 = append(app1, seg...)
	data := buf.Bytes()
	return append(append(append([]byte{}, data[:2]...), app1...), data[2:]...)
}

func TestProcessImageStripsMetadata(t *testing.T) {
	for _, o := range []int{1, 6} {
		data := exifJPEG(t, 40, 20, o)
		if jpegOrientation(data) != o {
			t.Fatalf("orientation %d not read", o)
		}
		// A Multi-Picture file: a second image, with its own EXIF, after the first.
		data = append(data, exifJPEG(t, 8, 8, 1)...)
		clean, views, info, err := processImage(data, "image/jpeg")
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(clean, []byte("GPS")) {
			t.Errorf("orientation %d: metadata kept", o)
		}
		if got := jpegOrientation(clean); got != o {
			t.Errorf("orientation %d: the original says %d", o, got)
		}
		if o == 1 && bytes.Contains(clean, []byte("Exif")) {
			t.Error("an EXIF segment was kept with nothing to say")
		}
		// The original keeps its pixels as stored; the views are upright.
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(clean))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Width != 40 || cfg.Height != 20 {
			t.Errorf("orientation %d: original %dx%d", o, cfg.Width, cfg.Height)
		}
		ai, err := jpeg.Decode(bytes.NewReader(views["ai.jpg"]))
		if err != nil {
			t.Fatal(err)
		}
		wantW, wantH := 40, 20
		if o == 6 { // a quarter turn
			wantW, wantH = 20, 40
		}
		if b := ai.Bounds(); b.Dx() != wantW || b.Dy() != wantH || info["width"] != wantW || info["height"] != wantH {
			t.Errorf("orientation %d: view %v, info %v", o, b, info)
		}
		if o == 6 {
			// The red band ends up on the right.
			if r, g, _, _ := ai.At(17, 20).RGBA(); r < 2*g {
				t.Errorf("not rotated: %v", ai.At(17, 20))
			}
			if r, g, _, _ := ai.At(2, 20).RGBA(); r > 0x4000 && r > 2*g {
				t.Errorf("red on the left: %v", ai.At(2, 20))
			}
		}
		if bytes.Count(clean, []byte{0xff, 0xd8}) != 1 {
			t.Error("the second image was kept")
		}
	}

	p := testPNG(t, 3000, 10)
	withText := append(append(append([]byte{}, p[:33]...), pngChunk("tEXt", []byte("GPS\x0037N"))...), p[33:]...)
	clean, views, info, err := processImage(withText, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(clean, []byte("GPS")) {
		t.Error("PNG text kept")
	}
	if _, err := png.Decode(bytes.NewReader(clean)); err != nil {
		t.Errorf("cleaned PNG: %v", err)
	}
	cfg, _ := jpeg.DecodeConfig(bytes.NewReader(views["ai.jpg"]))
	if cfg.Width != AIViewSize || info["width"] != 3000 {
		t.Errorf("ai view %dx%d, info %v", cfg.Width, cfg.Height, info)
	}

	// A header claiming too many pixels is refused before decoding.
	huge := append([]byte{}, p[:8]...)
	ihdr := binary.BigEndian.AppendUint32(nil, 10000)
	ihdr = binary.BigEndian.AppendUint32(ihdr, 10000)
	ihdr = append(ihdr, 8, 6, 0, 0, 0)
	huge = append(huge, pngChunk("IHDR", ihdr)...)
	huge = append(huge, p[33:]...)
	if _, _, _, err := processImage(huge, "image/png"); err == nil || !strings.Contains(err.Error(), "megapixels") {
		t.Errorf("huge image: %v", err)
	}
}

func TestStripWebPMetadata(t *testing.T) {
	chunk := func(id string, data []byte) []byte {
		out := append([]byte(id), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...)
		out = append(out, data...)
		if len(data)%2 == 1 {
			out = append(out, 0)
		}
		return out
	}
	body := append(chunk("VP8X", []byte{0x0c, 0, 0, 0, 0, 0, 0, 0, 0, 0}), chunk("VP8L", []byte("pixels!"))...)
	body = append(body, chunk("EXIF", []byte("GPS here"))...)
	data := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+len(body)))...)
	data = append(append(data, "WEBP"...), body...)
	out, err := stripWebPMetadata(data)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("GPS")) || out[20] != 0 {
		t.Errorf("metadata kept: %q", out)
	}
	if int(binary.LittleEndian.Uint32(out[4:8])) != len(out)-8 || !bytes.Contains(out, []byte("pixels!")) {
		t.Errorf("broken RIFF: %q", out)
	}
}

func TestGridOverlay(t *testing.T) {
	_, views, _, err := processImage(testPNG(t, 400, 300), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	out, err := GridOverlay(views["ai.jpg"])
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfg.Width != 400 || cfg.Height != 300 {
		t.Fatalf("grid: %v %dx%d", err, cfg.Width, cfg.Height)
	}
}

func TestAssetNames(t *testing.T) {
	cases := map[string]string{
		"plan.pdf":                        "plan.pdf",
		"C:\\Users\\me\\a.png":            "a.png",
		"../../etc/passwd":                "passwd",
		"bad\x00name\n.png":               "badname.png",
		"  ":                              "file",
		strings.Repeat("a", 300) + ".obj": strings.Repeat("a", maxAssetNameLen-4) + ".obj",
	}
	for in, want := range cases {
		if got := cleanAssetName(in); got != want {
			t.Errorf("cleanAssetName(%q) = %q, want %q", in, got, want)
		}
	}
	taken := map[string]bool{"a.png": true, "a-2.png": true}
	if got := uniqueName("a.png", taken); got != "a-3.png" {
		t.Errorf("uniqueName = %q", got)
	}
	lower := map[string]bool{"plan.png": true, "plan-2.png": true}
	for in, want := range map[string]string{"Plan.png": "Plan-3.png", "PLAN-2.png": "PLAN-3.png", "other.png": "other.png"} {
		if got := uniqueFoldedName(in, lower); got != want {
			t.Errorf("uniqueFoldedName(%q) = %q, want %q", in, got, want)
		}
	}
	if got := cleanAssetName("chair 🪑.obj"); got != "chair _.obj" {
		t.Errorf("emoji: %q", got)
	}
}
