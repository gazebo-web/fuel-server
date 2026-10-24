package sdf

import (
	"strings"
	"testing"
)

func TestLightingWarnings(t *testing.T) {
	light := func(typ, name, body string) string {
		return `<light type="` + typ + `" name="` + name + `">` + body + `</light>`
	}
	bright := light("point", "room", `<diffuse>1.2 1 1 1</diffuse><intensity>3</intensity>`+
		`<attenuation><range>10</range><constant>0.4</constant></attenuation>`)
	var rooms strings.Builder
	for i := 0; i < 5; i++ {
		rooms.WriteString(light("point", "lamp"+string(rune('a'+i)), `<attenuation><range>5</range><constant>1</constant></attenuation>`))
	}
	src := `<sdf version="1.12"><world name="w"><scene><ambient>0.55 0.55 0.58 1</ambient></scene>` +
		light("directional", "sun", `<direction>-0.5 0.1 -0.9</direction>`) +
		light("directional", "sun2", `<direction>0.5 0.1 -0.9</direction>`) + bright + rooms.String() +
		`</world></sdf>`
	r := Validate(src)
	all := messages(r.Warnings)
	for _, want := range []string{"ambient 0.55 0.55 0.58 1 washes", "2 directional lights", "6 point and spot lights",
		"diffuse 1.2 1 1 1 is over 1", "intensity 3 overexposes", "constant 0.4 makes it 2.5 times"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing warning %q in:\n%s", want, all)
		}
	}

	// Gazebo's default lighting: no warnings.
	src = `<sdf version="1.12"><world name="w"><scene><ambient>0.4 0.4 0.4 1</ambient></scene>` +
		light("directional", "sun", `<diffuse>0.8 0.8 0.8 1</diffuse><attenuation><range>1000</range>`+
			`<constant>0.9</constant></attenuation><direction>-0.5 0.1 -0.9</direction>`) +
		light("point", "lamp", `<attenuation><range>5</range><constant>1</constant></attenuation>`) + `</world></sdf>`
	if w := messages(Validate(src).Warnings); strings.Contains(w, "light") || strings.Contains(w, "ambient") {
		t.Errorf("default lighting warned: %s", w)
	}
}

func TestDimCeilingLights(t *testing.T) {
	// A covered floor lit by a grid of dim lights is fine; too many warn.
	grid := func(n int) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteString(`<light type="point" name="ceiling_` + strings.Repeat("x", i+1) + `">` +
				`<diffuse>0.3 0.3 0.3 1</diffuse><cast_shadows>false</cast_shadows>` +
				`<attenuation><range>12</range><constant>1</constant><linear>0.05</linear></attenuation></light>`)
		}
		return `<sdf version="1.12"><world name="w">` + b.String() + `</world></sdf>`
	}
	if w := messages(Validate(grid(16)).Warnings); strings.Contains(w, "light") {
		t.Errorf("16 dim lights warned: %s", w)
	}
	if w := messages(Validate(grid(33)).Warnings); !strings.Contains(w, "33 dim point and spot lights") {
		t.Errorf("33 dim lights: %s", w)
	}
}
