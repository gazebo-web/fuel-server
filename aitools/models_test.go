package aitools

import (
	"strings"
	"testing"

	"github.com/gazebo-web/fuel-server/sdf"
)

var testFiles = []string{
	"world.sdf",
	"models/crate/model.config", "models/crate/model.sdf", "models/crate/meshes/visual.glb",
	"models/cart/model.sdf",
	"models/notamodel/readme.txt",
}

func TestResolveInclude(t *testing.T) {
	world := newProjectFiles("world.sdf", testFiles)
	if got := strings.Join(world.models(), ","); got != "cart,crate" {
		t.Errorf("models %s", got)
	}
	for uri, wantErr := range map[string]string{
		"model://crate":     "",
		"model://cart/":     "",
		"models/crate":      "",
		"model://sun":       `no model "sun" in this project (project models: cart, crate)`,
		"model://notamodel": "no model",
		"/home/me/models/x": "only works on one machine",
		"file:///tmp/x":     "only works on one machine",
		"models/missing":    "no model folder models/missing",
		"https://fuel.gazebosim.org/1.0/OpenRobotics/models/Table": "-", // not handled here
	} {
		handled, err := world.resolveInclude(uri)
		switch {
		case wantErr == "-":
			if handled {
				t.Errorf("%s: should be left to the catalog check", uri)
			}
		case wantErr == "":
			if !handled || err != nil {
				t.Errorf("%s: %v", uri, err)
			}
		case err == nil || !strings.Contains(err.Error(), wantErr):
			t.Errorf("%s: got %v, want %q", uri, err, wantErr)
		}
	}
}

func TestResolveFile(t *testing.T) {
	model := newProjectFiles("models/crate/model.sdf", testFiles)
	for uri, ok := range map[string]bool{
		"meshes/visual.glb":               true,
		"model://crate/meshes/visual.glb": true,
		"meshes/collision.glb":            false,
		"model://cart/meshes/visual.glb":  false,
		"/abs/visual.glb":                 false,
		"https://fuel.gazebosim.org/1.0/o/models/m/tip/files/meshes/a.dae": true,
	} {
		if err := model.resolveFile(uri); (err == nil) != ok {
			t.Errorf("%s: %v", uri, err)
		}
	}
}

func TestCheckModelConfig(t *testing.T) {
	pf := newProjectFiles("models/crate/model.config", testFiles)
	good := `<model><name>crate</name><sdf version="1.9">model.sdf</sdf></model>`
	if r := checkModelConfig(good, pf); !r.Valid || len(r.Warnings) != 0 {
		t.Errorf("good: %+v", r)
	}
	if r := checkModelConfig(`<model><name>crate</name><sdf version="1.9">crate.sdf</sdf></model>`, pf); len(r.Warnings) != 1 {
		t.Errorf("missing sdf file should warn: %+v", r)
	}
	for _, bad := range []string{`<model><sdf>model.sdf</sdf></model>`, `<model><name>x</name></model>`, `<model>`} {
		if r := checkModelConfig(bad, pf); r.Valid {
			t.Errorf("%s: want invalid", bad)
		}
	}
}

func TestGeometryXML(t *testing.T) {
	for _, c := range []struct {
		s    sdf.Shape
		want string
	}{
		{sdf.Shape{Kind: "box", Size: [3]float64{1, 0.5, 2}}, "<box><size>1 0.5 2</size></box>"},
		{sdf.Shape{Kind: "cylinder", Radius: 0.1, Length: 1}, "<cylinder><radius>0.1</radius><length>1</length></cylinder>"},
		{sdf.Shape{Kind: "sphere", Radius: 2}, "<sphere><radius>2</radius></sphere>"},
	} {
		got, err := geometryXML(c.s)
		if err != nil || got != c.want {
			t.Errorf("%+v: %s %v", c.s, got, err)
		}
	}
	if _, err := geometryXML(sdf.Shape{Kind: "box"}); err == nil {
		t.Error("zero box accepted")
	}
}
