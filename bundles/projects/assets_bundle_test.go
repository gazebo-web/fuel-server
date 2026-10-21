package projects

import (
	"archive/zip"
	"bytes"
	"sort"
	"strings"
	"testing"
)

const shelfSDF = `<?xml version="1.0"?><sdf version="1.9"><model name="shelf"><static>true</static><link name="l">
<visual name="v"><geometry><mesh><uri>model://Shelf_01/meshes/shelf.dae</uri></mesh></geometry></visual></link></model></sdf>`

func zipEntries(t *testing.T, data []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	sort.Strings(out)
	return out
}

func TestExpandBundle(t *testing.T) {
	png := string(testPNG(t, 4, 4))
	data := zipBytes(t, map[string]string{
		// A Gazebo model, with a nested folder that isn't a model of its own.
		"warehouse/models/Shelf_01/model.config":       `<model><name>Shelf</name><sdf version="1.9">model.sdf</sdf></model>`,
		"warehouse/models/Shelf_01/model.sdf":          shelfSDF,
		"warehouse/models/Shelf_01/meshes/shelf.dae":   `<COLLADA><init_from>../materials/wood.png</init_from></COLLADA>`,
		"warehouse/models/Shelf_01/materials/wood.png": png,
		"warehouse/models/Shelf_01/thumbnails/1.png":   png,
		"warehouse/models/Bad Name!/model.config":      `<model><sdf version="1.9">missing.sdf</sdf></model>`,
		"warehouse/models/Bad Name!/notes.txt":         "x",
		// An OBJ with its material library and texture, and a glTF with its buffer.
		"warehouse/props/chair.obj":          "mtllib chair.mtl\nv 0 0 0\nf 1 1 1\n",
		"warehouse/props/chair.mtl":          "newmtl wood\nmap_Kd -s 1 1 1 tex/chair_wood.png\n",
		"warehouse/props/tex/chair_wood.png": png,
		"warehouse/props/cart.gltf":          `{"buffers":[{"uri":"cart.bin"}],"images":[{"uri":"data:image/png;base64,AA"}]}`,
		"warehouse/props/cart.bin":           "bin",
		"warehouse/props/crate.glb":          "glTF....",
		// A texture set, a plan and a photo, and things to skip.
		"warehouse/textures/concrete/Concrete_Color.jpg":    png,
		"warehouse/textures/concrete/Concrete_NormalGL.png": png,
		"warehouse/plan.pdf":                                "%PDF-1.4",
		"warehouse/photo.png":                               png,
		"warehouse/README.md":                               "hello",
		"warehouse/inner.zip":                               "PK",
	})
	ups, models, skipped, err := expandBundle("warehouse.zip", data)
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].dir != "warehouse/models/Bad Name!" && models[1].dir != "warehouse/models/Bad Name!" {
		t.Fatalf("models: %+v", models)
	}
	var shelf modelFolder
	for _, m := range models {
		if strings.HasSuffix(m.dir, "Shelf_01") {
			shelf = m
		}
	}
	if len(shelf.files) != 5 || shelf.files["materials/wood.png"] == nil {
		t.Errorf("shelf files: %v", len(shelf.files))
	}

	byName := map[string]AssetUpload{}
	for _, u := range ups {
		byName[u.Name] = u
	}
	var names []string
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "cart.gltf,chair.obj,concrete textures,crate.glb,photo.png,plan.pdf" {
		t.Fatalf("uploads: %v", names)
	}
	if got := zipEntries(t, byName["chair.obj"].Data); strings.Join(got, ",") !=
		"warehouse/props/chair.mtl,warehouse/props/chair.obj,warehouse/props/tex/chair_wood.png" {
		t.Errorf("chair group: %v", got)
	}
	if got := zipEntries(t, byName["cart.gltf"].Data); strings.Join(got, ",") != "warehouse/props/cart.bin,warehouse/props/cart.gltf" {
		t.Errorf("cart group: %v", got)
	}
	if string(byName["crate.glb"].Data) != "glTF...." {
		t.Error("a mesh without references stays a file of its own")
	}
	if got := zipEntries(t, byName["concrete textures"].Data); len(got) != 2 {
		t.Errorf("texture set: %v", got)
	}
	if byName["photo.png"].Info["from_zip"] != "warehouse.zip" || byName["photo.png"].Info["path"] != "warehouse/photo.png" {
		t.Errorf("info: %v", byName["photo.png"].Info)
	}
	sort.Strings(skipped)
	if strings.Join(skipped, ",") != "warehouse/README.md,warehouse/inner.zip" {
		t.Errorf("skipped: %v", skipped)
	}

	// Each grouped mesh sniffs as one mesh with its files.
	for _, n := range []string{"chair.obj", "cart.gltf"} {
		if kind, _, info, err := sniffAsset(byName[n].Data); err != nil || kind != AssetMesh || info["main"] == nil {
			t.Errorf("%s: %s %v %v", n, kind, info, err)
		}
	}
	if kind, _, _, _ := sniffAsset(byName["concrete textures"].Data); kind != AssetTexture {
		t.Errorf("texture set sniffs as %s", kind)
	}
	// And the whole zip sniffs as a bundle.
	if kind, _, _, err := sniffAsset(data); kind != assetBundle || err != nil {
		t.Errorf("bundle sniffs as %s %v", kind, err)
	}
}

func TestModelHelpers(t *testing.T) {
	taken := map[string]bool{"shelf_01": true}
	for in, want := range map[string]string{"x/Shelf_01": "Shelf_01_2", "x/Bad Name!": "Bad_Name_", ".": "model", "x/caddy": "caddy"} {
		if got := modelName(in, taken); got != want {
			t.Errorf("modelName(%q) = %q, want %q", in, got, want)
		}
	}
	m := modelFolder{dir: "m/Shelf_01", files: map[string][]byte{"model.sdf": []byte(shelfSDF),
		"model.config": []byte(`<model><sdf version="1.9">model.sdf</sdf></model>`)}}
	rewriteModelURIs(m, "Shelf_01", "Shelf_01_2")
	if !strings.Contains(string(m.files["model.sdf"]), "model://Shelf_01_2/meshes/shelf.dae") {
		t.Errorf("not rewritten: %s", m.files["model.sdf"])
	}
	// AWS-style file:// paths relative to another layout.
	aws := modelFolder{dir: "models/Bin_01", files: map[string][]byte{
		"meshes/Bin_01_visual.DAE": []byte("<COLLADA/>"),
		"model.sdf": []byte(`<sdf version="1.6"><model name="Bin_01"><link name="l"><visual name="v"><geometry><mesh>` +
			`<uri>file://models/Bin_01/meshes/Bin_01_visual.DAE</uri></mesh></geometry></visual><collision name="c">` +
			`<geometry><mesh><uri>file:///home/me/elsewhere/missing.dae</uri></mesh></geometry></collision></link></model></sdf>`)}}
	rewriteModelURIs(aws, "Bin_01", "Bin_01")
	got := string(aws.files["model.sdf"])
	if !strings.Contains(got, "<uri>model://Bin_01/meshes/Bin_01_visual.DAE</uri>") ||
		!strings.Contains(got, "<uri>file:///home/me/elsewhere/missing.dae</uri>") {
		t.Errorf("file:// URIs: %s", got)
	}
	info := modelInfo("Shelf_01_2", m)
	if info["uri"] != "model://Shelf_01_2" || info["validation"].(map[string]any)["errors"] != 0 {
		t.Errorf("info: %v", info)
	}
	bad := modelInfo("x", modelFolder{files: map[string][]byte{"model.config": []byte(`<sdf version="1.9">gone.sdf</sdf>`)}})
	if !strings.Contains(bad["validation"].(map[string]any)["first"].(string), "gone.sdf") {
		t.Errorf("missing sdf: %v", bad)
	}
	if v := modelThumbnail(modelFolder{files: map[string][]byte{"thumbnails/1.png": testPNG(t, 8, 8)}}); v["thumb.jpg"] == nil {
		t.Error("no thumbnail view")
	}
}
