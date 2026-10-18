package sdf

import (
	"errors"
	"strings"
	"testing"
)

const goodWorld = `<?xml version="1.0"?>
<sdf version="1.9">
  <world name="demo">
    <gravity>0 0 -9.8</gravity>
    <plugin filename="gz-sim-physics-system" name="gz::sim::systems::Physics">
      <engine><filename>gz-physics-dartsim-plugin</filename></engine>
    </plugin>
    <plugin filename="gz-sim-user-commands-system" name="gz::sim::systems::UserCommands"/>
    <plugin filename="gz-sim-scene-broadcaster-system" name="gz::sim::systems::SceneBroadcaster"/>
    <include>
      <uri>https://fuel.gazebosim.org/1.0/OpenRobotics/models/Table</uri>
      <name>table</name>
      <pose>1 0 0 0 0 0</pose>
    </include>
    <model name="cart">
      <link name="base">
        <inertial>
          <mass>20.0</mass>
          <inertia><ixx>3.33333</ixx><iyy>3.33333</iyy><izz>3.33333</izz></inertia>
        </inertial>
        <collision name="c"><geometry><box><size>1 1 1</size></box></geometry></collision>
        <visual name="v"><geometry><box><size>1 1 1</size></box></geometry></visual>
      </link>
      <link name="wheel"/>
      <joint name="axle" type="revolute">
        <parent>base</parent>
        <child>wheel</child>
      </joint>
    </model>
  </world>
</sdf>`

func messages(items []ValidationError) string {
	var parts []string
	for _, e := range items {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, " | ")
}

func TestValidateAcceptsRealisticWorld(t *testing.T) {
	r := Validate(goodWorld)
	if !r.Valid || len(r.Warnings) != 0 {
		t.Fatalf("want clean; errors=%s warnings=%s", messages(r.Errors), messages(r.Warnings))
	}
}

func TestValidateAcceptsMinimalWorld(t *testing.T) {
	r := Validate(`<sdf version="1.9"><world name="w"/></sdf>`)
	if !r.Valid {
		t.Errorf("expected valid; errors=%s", messages(r.Errors))
	}
}

func TestValidateStructure(t *testing.T) {
	cases := map[string]struct{ sdf, want string }{
		"non-sdf root":    {`<foo/>`, "root element must be <sdf>"},
		"no version":      {`<sdf><model name="m"/></sdf>`, "must have a version"},
		"malformed":       {"<sdf version=\"1.9\">\n<world name=\"w\">\n</sdf>", "malformed XML"},
		"no top element":  {`<sdf version="1.9"></sdf>`, "at least one of"},
		"missing attr":    {`<sdf version="1.9"><world/></sdf>`, `missing required attribute "name"`},
		"bad bool":        {`<sdf version="1.9"><model name="m"><static>maybe</static></model></sdf>`, "expected true or false"},
		"bad pose":        {`<sdf version="1.9"><model name="m"><pose>1 2 3</pose></model></sdf>`, "expected 6 numbers"},
		"bad number":      {`<sdf version="1.9"><model name="m"><link name="l"><inertial><mass>heavy</mass></inertial></link></model></sdf>`, "is not a number"},
		"include no uri":  {`<sdf version="1.9"><world name="w"><include><name>x</name></include></world></sdf>`, "missing required <uri>"},
		"duplicate model": {`<sdf version="1.9"><world name="w"><model name="a"/><model name="a"/></world></sdf>`, `duplicate name "a"`},
		"bad joint child": {`<sdf version="1.9"><model name="m"><link name="a"/><joint name="j" type="fixed"><parent>a</parent><child>nope</child></joint></model></sdf>`, `child "nope" does not name`},
		"zero mass":       {`<sdf version="1.9"><model name="m"><link name="l"><inertial><mass>0</mass></inertial></link></model></sdf>`, "mass must be positive"},
		"bad inertia":     {`<sdf version="1.9"><model name="m"><link name="l"><inertial><mass>1</mass><inertia><ixx>0.1</ixx><iyy>0.1</iyy><izz>1</izz></inertia></inertial></link></model></sdf>`, "ixx+iyy >= izz"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := Validate(c.sdf)
			if r.Valid {
				t.Fatalf("expected invalid")
			}
			if got := messages(r.Errors); !strings.Contains(got, c.want) {
				t.Fatalf("errors %q do not mention %q", got, c.want)
			}
		})
	}
}

func TestValidateWarnsOnUnknownElementsAndAttributes(t *testing.T) {
	r := Validate(`<sdf version="1.9"><model name="m" colour="red"><lnk name="l"/></model></sdf>`)
	if !r.Valid {
		t.Fatalf("unknown names should warn, not error: %s", messages(r.Errors))
	}
	got := messages(r.Warnings)
	for _, want := range []string{"unknown element <lnk>", `unknown attribute "colour"`} {
		if !strings.Contains(got, want) {
			t.Errorf("warnings %q do not mention %q", got, want)
		}
	}
}

func TestValidateSkipsNamespacedElements(t *testing.T) {
	r := Validate(`<sdf version="1.9" xmlns:gz="http://gazebosim.org/schema"><model name="m"><gz:custom>x</gz:custom></model></sdf>`)
	if !r.Valid || len(r.Warnings) != 0 {
		t.Fatalf("namespaced element should be ignored; errors=%s warnings=%s", messages(r.Errors), messages(r.Warnings))
	}
}

func TestValidateUnknownVersionWarns(t *testing.T) {
	r := Validate(`<sdf version="9.9"><model name="m"><bogus/></model></sdf>`)
	if !r.Valid {
		t.Fatalf("unknown version should not error: %s", messages(r.Errors))
	}
	if !strings.Contains(messages(r.Warnings), "not bundled") {
		t.Fatalf("expected a not-bundled warning, got %s", messages(r.Warnings))
	}
}

func TestValidateResolveURI(t *testing.T) {
	var asked []string
	r := ValidateWithOptions(goodWorld, Options{ResolveURI: func(uri string) error {
		asked = append(asked, uri)
		return errors.New("model not found")
	}})
	if r.Valid || !strings.Contains(messages(r.Errors), "model not found") {
		t.Fatalf("expected resolver error, got %s", messages(r.Errors))
	}
	if len(asked) != 1 || !strings.HasSuffix(asked[0], "/models/Table") {
		t.Fatalf("resolver called with %v", asked)
	}
}

func TestValidateReportsLineAndPath(t *testing.T) {
	src := "<sdf version=\"1.9\">\n<world name=\"w\">\n<model name=\"m\">\n<static>maybe</static>\n</model>\n</world>\n</sdf>"
	r := Validate(src)
	if len(r.Errors) != 1 {
		t.Fatalf("want 1 error, got %s", messages(r.Errors))
	}
	e := r.Errors[0]
	if e.Line != 4 || e.Path != "/sdf/world[w]/model[m]/static" {
		t.Fatalf("got line %d path %q", e.Line, e.Path)
	}
	if s := r.Summary("world.sdf"); !strings.Contains(s, "error world.sdf:4 /sdf/world[w]/model[m]/static: ") {
		t.Fatalf("summary %q", s)
	}
}

func TestValidate18(t *testing.T) {
	r := Validate(strings.Replace(goodWorld, `version="1.9"`, `version="1.8"`, 1))
	if !r.Valid {
		t.Fatalf("1.8 world should validate: %s", messages(r.Errors))
	}
}

func TestValidateToleratesTinyXMLConstructs(t *testing.T) {
	src := "<sdf version=\"1.9\">\n<!-- a -- comment -->\n<world name=\"w\">\n" +
		"<plugin filename=\"f\" name=\"n\"><gz::system_priority>10</gz::system_priority></plugin>\n" +
		"<model name=\"m\"><static>maybe</static></model>\n</world>\n</sdf>"
	r := Validate(src)
	if len(r.Errors) != 1 || !strings.Contains(r.Errors[0].Message, "expected true or false") {
		t.Fatalf("want only the static error, got %s", messages(r.Errors))
	}
	if r.Errors[0].Line != 5 {
		t.Fatalf("line numbers must survive the rewrite: got %d, want 5", r.Errors[0].Line)
	}
}

func TestValidateNestedModels(t *testing.T) {
	src := `<sdf version="1.9"><model name="outer"><link name="a"/>
	<model name="inner"><link name="b"/><joint name="j" type="fixed"><parent>b</parent><child>c</child></joint><link name="c"/></model>
	</model></sdf>`
	r := Validate(src)
	if !r.Valid || len(r.Warnings) != 0 {
		t.Fatalf("nested model should be clean; errors=%s warnings=%s", messages(r.Errors), messages(r.Warnings))
	}
}

func TestValidateAcceptsHexIntegers(t *testing.T) {
	src := `<sdf version="1.9"><model name="m"><link name="l"><collision name="c"><geometry><box><size>1 1 1</size></box></geometry>
	<surface><contact><collide_bitmask>0xAB</collide_bitmask></contact></surface></collision></link></model></sdf>`
	if r := Validate(src); !r.Valid {
		t.Fatalf("hex bitmask should be valid: %s", messages(r.Errors))
	}
}

func TestResolveFileHook(t *testing.T) {
	doc := `<sdf version="1.9"><model name="m"><static>true</static><link name="l"><visual name="v">
	  <geometry><mesh><uri>meshes/missing.glb</uri></mesh></geometry>
	  <material><pbr><metal><albedo_map>materials/textures/a.png</albedo_map></metal></pbr></material>
	</visual></link></model></sdf>`
	var seen []string
	r := ValidateWithOptions(doc, Options{ResolveFile: func(uri string) error {
		seen = append(seen, uri)
		if uri == "meshes/missing.glb" {
			return errors.New("no file")
		}
		return nil
	}})
	if len(seen) != 2 || len(r.Errors) != 1 || !strings.Contains(r.Errors[0].Message, `mesh "meshes/missing.glb": no file`) {
		t.Fatalf("seen %v errors %v", seen, r.Errors)
	}
}

func TestResolveURIWarning(t *testing.T) {
	doc := `<sdf version="1.9"><world name="w"><include><uri>https://x/1.0/o/models/m</uri></include></world></sdf>`
	r := ValidateWithOptions(doc, Options{ResolveURI: func(string) error { return Warning("private model") }})
	if !r.Valid || len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0].Message, "private model") {
		t.Fatalf("errors %v warnings %v", r.Errors, r.Warnings)
	}
}
