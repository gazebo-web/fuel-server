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
