package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbeddedCorpusLoads(t *testing.T) {
	cases, err := loadCorpus("", "")
	if err != nil || len(cases) < 5 {
		t.Fatalf("corpus: %d cases, %v", len(cases), err)
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if c.ID == "" || c.Prompt == "" || seen[c.ID] {
			t.Errorf("bad or duplicate case %+v", c)
		}
		seen[c.ID] = true
	}
	only, _ := loadCorpus("", "kitchen, parking-lot")
	if len(only) != 2 {
		t.Fatalf("-only selected %d cases", len(only))
	}
}

func TestReadSSE(t *testing.T) {
	stream := "event: text\ndata: {\"type\":\"text\",\"text\":\"Building\"}\n\n" +
		"event: tool_call\ndata: {\"tool\":\"project_write_file\"}\n\n" +
		"event: project_changed\ndata: {\"path\":\"world.sdf\",\"rev\":\"abc\"}\n\n" +
		"event: usage\ndata: {\"usage\":{\"input_tokens\":10,\"cache_read_tokens\":90,\"output_tokens\":7,\"thinking_tokens\":5}}\n\n" +
		"event: usage\ndata: {\"usage\":{\"input_tokens\":1,\"output_tokens\":2}}\n\n" +
		"event: done\ndata: {}\n\n" +
		"event: text\ndata: {\"text\":\"after done\"}\n\n"
	tl := tally{}
	if err := readSSE(strings.NewReader(stream), tl.add); err != nil {
		t.Fatal(err)
	}
	if tl.usage != (Usage{Calls: 2, Input: 11, CacheRead: 90, Output: 9, Thinking: 5}) {
		t.Errorf("usage %+v", tl.usage)
	}
	if tl.writes != 1 || tl.toolCalls != 1 || tl.reply.String() != "Building" {
		t.Fatalf("tally %+v reply %q", tl, tl.reply.String())
	}
}

func TestScore(t *testing.T) {
	world := `<sdf version="1.9"><world name="w">
	  <include><uri>https://fuel.gazebosim.org/1.0/OpenRobotics/models/Table</uri><name>t</name></include>
	  <model name="box"><link name="l"/></model>
	</world></sdf>`
	res := Result{Writes: 2}
	score(Case{MinEntities: 2, WantInclude: true}, world, nil, &res)
	if !res.Passed || res.Entities != 2 || res.Includes != 1 {
		t.Fatalf("want pass: %+v", res)
	}

	res = Result{Writes: 0}
	score(Case{MinEntities: 3, WantJoints: 1}, `<sdf version="1.9"><world name="w"><model name="m"><static>maybe</static></model></world></sdf>`, nil, &res)
	got := strings.Join(res.Failures, "|")
	for _, want := range []string{"validation errors", "no edits", "entities", "joints"} {
		if !strings.Contains(got, want) {
			t.Errorf("failures %q missing %q", got, want)
		}
	}
}

func TestScoreProjectModels(t *testing.T) {
	world := `<sdf version="1.9"><world name="w">
	  <include><uri>model://robot</uri><name>r1</name></include>
	  <include><uri>model://robot</uri><name>r2</name></include>
	  <include><uri>model://loop</uri></include>
	</world></sdf>`
	models := map[string]string{
		"robot": `<sdf version="1.9"><model name="robot">
		  <link name="a"/><link name="b"/><link name="c"/>
		  <joint name="j1" type="revolute"><parent>a</parent><child>b</child></joint>
		  <joint name="j2" type="revolute"><parent>a</parent><child>c</child></joint>
		  <include><uri>https://fuel.gazebosim.org/1.0/OpenRobotics/models/Camera</uri></include>
		</model></sdf>`,
		"loop": `<sdf version="1.9"><model name="loop"><include><uri>model://loop</uri></include></model></sdf>`,
	}
	load := func(name string) (string, bool) { src, ok := models[name]; return src, ok }
	res := Result{Writes: 1}
	score(Case{MinEntities: 3, WantJoints: 4, WantInclude: true}, world, load, &res)
	if !res.Passed || res.Entities != 3 || res.Joints != 4 || res.Includes != 2 || res.Models != 4 {
		t.Fatalf("want pass with 3 entities, 4 joints, 2 Fuel includes, 4 model includes: %+v", res)
	}
}

func TestTallyCountsCallsPerTool(t *testing.T) {
	stream := "event: tool_call\ndata: {\"tool\":\"model_generate\"}\n\n" +
		"event: tool_call\ndata: {\"tool\":\"mesh_api_reference\"}\n\n" +
		"event: tool_call\ndata: {\"tool\":\"model_generate\"}\n\n" +
		"event: done\ndata: {}\n\n"
	tl := tally{calls: map[string]int{}}
	if err := readSSE(strings.NewReader(stream), tl.add); err != nil {
		t.Fatal(err)
	}
	if tl.calls["model_generate"] != 2 || tl.calls["mesh_api_reference"] != 1 || tl.toolCalls != 3 {
		t.Fatalf("%+v", tl.calls)
	}
}

func TestJudge(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "k" {
			w.WriteHeader(401)
			return
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"content":[{"type":"text","text":"Here: {\"score\": 4, \"reason\": \"a mug, handle slightly thick\"}"}]}`))
	}))
	defer srv.Close()
	j := &judge{key: "k", model: "m", base: srv.URL, http: srv.Client()}
	v, err := j.score(context.Background(), "a coffee mug", "mug", []byte("PNG"))
	if err != nil || v.Score != 4 || v.Model != "mug" || !strings.Contains(v.Reason, "handle") {
		t.Fatalf("%+v %v", v, err)
	}
	content := got["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if content[0].(map[string]any)["type"] != "image" || !strings.Contains(content[1].(map[string]any)["text"].(string), `"a coffee mug"`) {
		t.Fatalf("request %v", got)
	}
	j.key = "wrong"
	if _, err := j.score(context.Background(), "x", "y", nil); err == nil {
		t.Fatal("want an error for a rejected key")
	}
}

func TestFixtures(t *testing.T) {
	for name, fixture := range fixtures {
		b, err := fixture()
		if err != nil || len(b) == 0 {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.HasSuffix(name, ".png") {
			if _, err := png.Decode(bytes.NewReader(b)); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
	// The chair is 900 mm tall along Y.
	obj, _ := chairOBJ()
	top := 0.0
	for _, line := range strings.Split(string(obj), "\n") {
		if v := floats(strings.TrimPrefix(line, "v ")); strings.HasPrefix(line, "v ") && len(v) == 3 {
			top = math.Max(top, v[1])
		}
	}
	if top != 900 {
		t.Errorf("chair top %g mm", top)
	}
	// The map's walls are where its comment says: the cell just inside
	// (-5, -3) is a wall, and so is the one just inside (15, 9).
	data, _ := warehouseMap()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(zr.File) != 2 {
		t.Fatalf("map zip: %v", err)
	}
	f, _ := zr.File[1].Open()
	pgm, _ := io.ReadAll(f)
	pix := pgm[len("P5\n440 280\n255\n"):]
	cell := func(x, y float64) byte { // the cell holding (x, y)
		c, r := int((x+6)/0.05), 280-1-int((y+4)/0.05)
		return pix[r*440+c]
	}
	if cell(-4.99, -2.99) != 0 || cell(14.99, 8.99) != 0 || cell(-5.01, 0) != 205 || cell(0, 1.5) != 254 ||
		cell(5, 0) != 0 || cell(-1, -2.9) != 254 {
		t.Errorf("map cells: %d %d %d %d %d %d", cell(-4.99, -2.99), cell(14.99, 8.99), cell(-5.01, 0),
			cell(0, 1.5), cell(5, 0), cell(-1, -2.9))
	}
}

func TestFootprint(t *testing.T) {
	world := `<sdf version="1.9"><world name="w">
	  <model name="ground"><static>true</static><link name="l"><visual name="v">
	    <geometry><plane><normal>0 0 1</normal><size>100 100</size></plane></geometry></visual>
	    <visual name="floor"><pose>0 0 -0.05 0 0 0</pose><geometry><box><size>50 50 0.1</size></box></geometry></visual>
	  </link></model>
	  <model name="walls"><pose>1 2 0 0 0 0</pose><link name="l">
	    <visual name="a"><pose>3 0 1.25 0 0 0</pose><geometry><box><size>6 0.2 2.5</size></box></geometry></visual>
	    <visual name="b"><pose>0 2 1.25 0 0 1.5707963</pose><geometry><box><size>4 0.2 2.5</size></box></geometry></visual>
	  </link></model>
	  <include><uri>model://rack</uri><pose>5 5 0 0 0 0</pose></include>
	</world></sdf>`
	rack := `<sdf version="1.9"><model name="rack"><link name="l"><visual name="v"><pose>0 0 1 0 0 0</pose>
	  <geometry><cylinder><radius>0.5</radius><length>2</length></cylinder></geometry></visual></link></model></sdf>`
	files := func(p string) ([]byte, bool) { return []byte(rack), p == "models/rack/model.sdf" }
	f := worldFootprint(world, files)
	// Wall a: x 1..7, y 1.9..2.1; wall b (turned): x 0.9..1.1, y 2..6;
	// the rack: x 4.5..5.5, y 4.5..5.5. The ground and floor are too low.
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
	if f.shapes != 3 || !near(f.min[0], 0.9) || !near(f.min[1], 1.9) || !near(f.max[0], 7) || !near(f.max[1], 6) {
		t.Fatalf("footprint %+v", f)
	}

	res := Result{}
	checkFootprint(Case{WantSize: []float64{4, 6}, WantBounds: []float64{1, 2, 7, 6}}, f, &res)
	if !res.Passed {
		t.Errorf("want pass: %v", res.Failures)
	}
	res = Result{}
	checkFootprint(Case{WantCovered: [][2]float64{{5, 5}, {4, 2}, {3, 3}}}, f, &res)
	if res.Passed || len(res.Failures) != 1 || !strings.Contains(res.Failures[0], "[[3 3]]") {
		t.Errorf("want (3, 3) bare: %v", res.Failures)
	}
	res = Result{}
	checkFootprint(Case{WantSize: []float64{6, 4}, WantBounds: []float64{0, 0, 7, 6}}, footprint{}, &res)
	if res.Passed || !strings.Contains(res.Failures[0], "no walls") {
		t.Errorf("want fail: %v", res.Failures)
	}
	res = Result{}
	checkFootprint(Case{WantSize: []float64{3, 3}, WantBounds: []float64{-2, 2, 7, 6}}, f, &res)
	if len(res.Failures) != 2 {
		t.Errorf("want two failures: %v", res.Failures)
	}
}

func TestNoteImport(t *testing.T) {
	tl := tally{imported: map[string][]float64{}}
	tl.add("tool_result", []byte(`{"tool":"mesh_job_status","result":[{"type":"text","text":`+
		`"{\n  \"bounding_box_m\": [0.45, 0.45, 0.9],\n  \"import\": {\"up_axis\": \"y\"},\n  \"model\": \"chair\"\n}"},`+
		`{"type":"image","data":"..."}]}`))
	tl.add("tool_result", []byte(`{"tool":"model_generate","result":"{\"bounding_box_m\": [1, 1, 1], \"model\": \"mug\"}"}`))
	// As the agent streams it: the result is an object.
	tl.add("tool_result", []byte(`{"tool":"model_import","result":{"bounding_box_m":[0.4,0.5,1.2],`+
		`"import":{"up_axis":"y"},"model":"stool"}}`))
	if got := tl.imported["stool"]; len(got) != 3 || got[2] != 1.2 {
		t.Errorf("stool %v", tl.imported)
	}
	if got := tl.imported["chair"]; len(got) != 3 || got[2] != 0.9 || len(tl.imported) != 2 {
		t.Fatalf("imported %v", tl.imported)
	}
}

// testGLB is a GLB with one mesh whose positions span lo..hi, under a
// node moved by move.
func testGLB(lo, hi, move [3]float64) []byte {
	doc, _ := json.Marshal(map[string]any{
		"scene": 0, "scenes": []any{map[string]any{"nodes": []int{0}}},
		"nodes":     []any{map[string]any{"mesh": 0, "translation": move}},
		"meshes":    []any{map[string]any{"primitives": []any{map[string]any{"attributes": map[string]int{"NORMAL": 0, "POSITION": 1}}}}},
		"accessors": []any{map[string]any{"min": []int{-1, -1, -1}, "max": []int{1, 1, 1}}, map[string]any{"min": lo, "max": hi}},
	})
	for len(doc)%4 != 0 {
		doc = append(doc, ' ')
	}
	out := []byte("glTF")
	out = binary.LittleEndian.AppendUint32(out, 2)
	out = binary.LittleEndian.AppendUint32(out, uint32(20+len(doc)))
	out = binary.LittleEndian.AppendUint32(out, uint32(len(doc)))
	return append(append(out, "JSON"...), doc...)
}

func TestFootprintGLB(t *testing.T) {
	// A generated rack bay: its mesh is 2.6 × 1 × 2.8 m, centered in x and
	// y with its base at z = 0, and its node is moved 1 m along x. The
	// SDF scales it by 2 in y.
	rack := `<sdf version="1.9"><model name="bay"><link name="link"><visual name="v"><geometry>
	  <mesh><uri>meshes/bay.glb</uri><scale>1 2 1</scale></mesh></geometry></visual></link></model></sdf>`
	glb := testGLB([3]float64{-1.3, -0.5, 0}, [3]float64{1.3, 0.5, 2.8}, [3]float64{1, 0, 0})
	files := func(p string) ([]byte, bool) {
		switch p {
		case "models/bay/model.sdf":
			return []byte(rack), true
		case "models/bay/meshes/bay.glb":
			return glb, true
		}
		return nil, false
	}
	world := `<sdf version="1.9"><world name="w"><include><uri>model://bay</uri><pose>5 3 0 0 0 0</pose></include></world></sdf>`
	f := worldFootprint(world, files)
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-6 }
	if f.shapes != 1 || !near(f.min[0], 4.7) || !near(f.max[0], 7.3) || !near(f.min[1], 2) || !near(f.max[1], 4) {
		t.Fatalf("footprint %+v", f)
	}
	if !f.covers(5, 3) || f.covers(4, 3) {
		t.Error("covers")
	}
	if _, _, ok := glbBounds([]byte("not a glb")); ok {
		t.Error("garbage read as a GLB")
	}
}
