package sdf

import (
	"strconv"
	"strings"
)

// Lighting. Every light adds to every surface it reaches, and ambient
// light adds to all of them, so a world with a sun, a light per room and
// a raised ambient renders washed out: white walls, no shading. These
// checks warn about the usual causes, against Gazebo's defaults (ambient
// 0.4, one sun, attenuation 1/(constant + linear·d + quadratic·d²) with
// constant 1).

const (
	maxAmbient = 0.5 // per channel; Gazebo's default is 0.4
	// maxLocalLights is how many point and spot lights a world may have
	// before the warning: lamps the user asked for, not one per room.
	maxLocalLights = 4
	// minConstant: below it a light is brighter than full strength next to
	// it (0.4 is 2.5 times).
	minConstant = 0.5
	maxDiffuse  = 1.0
	maxStrength = 1.5 // <intensity>
)

func (v *validator) checkLighting(root *node) {
	for _, w := range root.children {
		if w.name == "world" {
			v.checkWorldLighting(w)
		}
	}
}

func (v *validator) checkWorldLighting(w *node) {
	if scene := w.child("scene"); scene != nil {
		if a := scene.child("ambient"); a != nil {
			if c, ok := floats(a.text); ok && len(c) >= 3 && maxOf(c[:3]) > maxAmbient {
				v.warnAt(a, 0, "ambient %s washes the scene out: it lights every surface evenly and hides "+
					"shading; keep it at Gazebo's default 0.4 0.4 0.4 1 or lower and let the sun do the lighting",
					strings.TrimSpace(a.text))
			}
		}
	}
	var suns, local []*node
	walk(w, func(n *node) bool {
		if n.name != "light" {
			return true
		}
		switch n.attr("type") {
		case "directional":
			suns = append(suns, n)
		case "point", "spot":
			local = append(local, n)
		}
		v.checkLight(n)
		return false
	})
	if len(suns) > 1 {
		v.warnAt(suns[1], 0, "%d directional lights: each lights the whole world, so they add up; one sun is "+
			"enough (remove the others, or make them dim fill lights with diffuse 0.2 or less)", len(suns))
	}
	if len(local) > maxLocalLights {
		v.warnAt(local[maxLocalLights], 0, "%d point and spot lights: together with the sun and ambient light "+
			"they overexpose the scene; keep only the lamps the user asked for (at most %d), or none",
			len(local), maxLocalLights)
	}
}

func (v *validator) checkLight(n *node) {
	name := n.attr("name")
	if d := n.child("diffuse"); d != nil {
		if c, ok := floats(d.text); ok && len(c) >= 3 && maxOf(c[:3]) > maxDiffuse {
			v.warnAt(d, 0, "light %s: diffuse %s is over 1, brighter than white; colors are 0 to 1",
				name, strings.TrimSpace(d.text))
		}
	}
	if i := n.child("intensity"); i != nil {
		if x, err := strconv.ParseFloat(strings.TrimSpace(i.text), 64); err == nil && x > maxStrength {
			v.warnAt(i, 0, "light %s: intensity %g overexposes what it lights; use 1 or less", name, x)
		}
	}
	if n.attr("type") == "directional" {
		return
	}
	if a := n.child("attenuation"); a != nil {
		if c := a.child("constant"); c != nil {
			if x, err := strconv.ParseFloat(strings.TrimSpace(c.text), 64); err == nil && x < minConstant {
				v.warnAt(c, 0, "light %s: attenuation constant %g makes it %.1f times full strength near it; "+
					"use 0.9 to 1", name, x, 1/maxFloat(x, 0.01))
			}
		}
	}
}

func floats(s string) ([]float64, bool) {
	var out []float64
	for _, f := range strings.Fields(s) {
		x, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil, false
		}
		out = append(out, x)
	}
	return out, true
}

func maxOf(xs []float64) float64 {
	m := xs[0]
	for _, x := range xs[1:] {
		m = maxFloat(m, x)
	}
	return m
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
