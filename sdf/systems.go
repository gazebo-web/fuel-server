package sdf

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// World systems. Gazebo simulates a sensor only when the world loads the
// system for its type: a camera or lidar in a world without the Sensors
// system publishes nothing, and nothing says why. And a world that lists
// any system gets none of Gazebo's defaults (Physics, UserCommands,
// SceneBroadcaster), so adding the Sensors system alone stops physics and
// blanks the web viewer. These checks catch both.

type worldSystem struct {
	key, plugin string
}

var (
	sensorsSystem  = worldSystem{"sensors", `<plugin filename="gz-sim-sensors-system" name="gz::sim::systems::Sensors"><render_engine>ogre2</render_engine></plugin>`}
	defaultSystems = []worldSystem{
		{"physics", `<plugin filename="gz-sim-physics-system" name="gz::sim::systems::Physics"/>`},
		{"usercommands", `<plugin filename="gz-sim-user-commands-system" name="gz::sim::systems::UserCommands"/>`},
		{"scenebroadcaster", `<plugin filename="gz-sim-scene-broadcaster-system" name="gz::sim::systems::SceneBroadcaster"/>`},
	}
	// sensorSystems maps each sensor type to the world system that runs it.
	sensorSystems = map[string]worldSystem{
		"imu":          {"imu", `<plugin filename="gz-sim-imu-system" name="gz::sim::systems::Imu"/>`},
		"contact":      {"contact", `<plugin filename="gz-sim-contact-system" name="gz::sim::systems::Contact"/>`},
		"navsat":       {"navsat", `<plugin filename="gz-sim-navsat-system" name="gz::sim::systems::NavSat"/>`},
		"gps":          {"navsat", `<plugin filename="gz-sim-navsat-system" name="gz::sim::systems::NavSat"/>`},
		"altimeter":    {"altimeter", `<plugin filename="gz-sim-altimeter-system" name="gz::sim::systems::Altimeter"/>`},
		"magnetometer": {"magnetometer", `<plugin filename="gz-sim-magnetometer-system" name="gz::sim::systems::Magnetometer"/>`},
		"air_pressure": {"airpressure", `<plugin filename="gz-sim-air-pressure-system" name="gz::sim::systems::AirPressure"/>`},
		"air_speed":    {"airspeed", `<plugin filename="gz-sim-air-speed-system" name="gz::sim::systems::AirSpeed"/>`},
		"force_torque": {"forcetorque", `<plugin filename="gz-sim-forcetorque-system" name="gz::sim::systems::ForceTorque"/>`},
	}
	// cpuLidar types parse but Gazebo has no CPU ray sensor.
	cpuLidar = map[string]bool{"lidar": true, "ray": true}
)

func init() {
	for _, t := range []string{"camera", "depth", "depth_camera", "rgbd", "rgbd_camera", "gpu_lidar", "gpu_ray",
		"thermal", "thermal_camera", "segmentation", "segmentation_camera", "boundingbox", "boundingbox_camera",
		"wideanglecamera", "wide_angle_camera", "multicamera", "logical_camera"} {
		sensorSystems[t] = sensorsSystem
	}
	sensorSystems["logical_camera"] = worldSystem{"logicalcamera",
		`<plugin filename="gz-sim-logical-camera-system" name="gz::sim::systems::LogicalCamera"/>`}
}

// systemKeys returns the keys a <plugin> provides, from its name
// ("gz::sim::systems::UserCommands" -> usercommands) and its filename
// ("libgz-sim8-user-commands-system.so" -> usercommands).
func systemKeys(p *node) []string {
	var keys []string
	if n := p.attr("name"); n != "" {
		if i := strings.LastIndex(n, "::"); i >= 0 {
			n = n[i+2:]
		}
		keys = append(keys, strings.ToLower(n))
	}
	f := strings.ToLower(path.Base(p.attr("filename")))
	f = strings.TrimSuffix(strings.TrimPrefix(f, "lib"), ".so")
	for _, prefix := range []string{"gz-sim", "ignition-gazebo"} {
		if strings.HasPrefix(f, prefix) {
			f = strings.TrimLeft(strings.TrimPrefix(f, prefix), "0123456789")
			f = strings.TrimSuffix(strings.TrimPrefix(f, "-"), "-system")
			keys = append(keys, strings.ReplaceAll(f, "-", ""))
		}
	}
	return keys
}

type sensorUse struct {
	types map[string]bool
	where map[string]bool
}

func (v *validator) checkSystems(root *node) {
	for _, w := range root.children {
		if w.name == "world" {
			v.checkWorldSystems(w)
		}
	}
}

func (v *validator) checkWorldSystems(w *node) {
	have := map[string]bool{}
	explicit := false
	for _, c := range w.children {
		if c.name == "plugin" {
			explicit = true
			for _, k := range systemKeys(c) {
				have[k] = true
			}
		}
	}
	if explicit {
		var missing []string
		for _, s := range defaultSystems {
			if !have[s.key] {
				missing = append(missing, s.plugin)
			}
		}
		if len(missing) > 0 {
			v.warnAt(w, 0, "this world lists systems, so Gazebo loads none of its defaults; without these, "+
				"nothing moves and the viewer shows nothing. Add: %s", strings.Join(missing, " "))
		}
	}

	need := map[worldSystem]*sensorUse{}
	note := func(typ, where string, at *node) {
		if cpuLidar[typ] {
			v.warnAt(at, 0, "%s: Gazebo doesn't simulate type %q; use type \"gpu_lidar\"", where, typ)
			typ = "gpu_lidar"
		}
		s, ok := sensorSystems[typ]
		if !ok || have[s.key] {
			return
		}
		u := need[s]
		if u == nil {
			u = &sensorUse{types: map[string]bool{}, where: map[string]bool{}}
			need[s] = u
		}
		u.types[typ] = true
		u.where[where] = true
	}
	walk(w, func(n *node) bool {
		switch n.name {
		case "sensor":
			note(n.attr("type"), "sensor "+n.attr("name"), n)
		case "include":
			uri := n.child("uri")
			if uri == nil || v.opts.IncludedModel == nil {
				return false
			}
			doc, ok := v.opts.IncludedModel(strings.TrimSpace(uri.text))
			if !ok {
				return false
			}
			inc, err := parseTree(doc)
			if err != nil {
				return false
			}
			name := strings.TrimSpace(uri.text)
			if c := n.child("name"); c != nil {
				name = strings.TrimSpace(c.text)
			}
			walk(inc, func(m *node) bool {
				if m.name == "sensor" {
					note(m.attr("type"), "model "+name, n)
				}
				return true
			})
			return false
		}
		return true
	})
	keys := make([]worldSystem, 0, len(need))
	for s := range need {
		keys = append(keys, s)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i].key < keys[j].key })
	for _, s := range keys {
		u := need[s]
		v.errorAt(w, 0, "%s sensors (%s) publish nothing without the world system for them; add %s to the world",
			strings.Join(sortedKeys(u.types), ", "), strings.Join(sortedKeys(u.where), ", "), s.plugin)
	}
}

// walk visits n's subtree depth first; f returns false to skip a node's
// children.
func walk(n *node, f func(*node) bool) {
	for _, c := range n.children {
		if f(c) {
			walk(c, f)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// MissingWorldSystems reports what the world in worldDoc lacks for
// sensors in modelDoc, a model the world includes: the same check
// ValidateWithOptions makes on a world, run when the model changes.
func MissingWorldSystems(worldDoc, modelDoc string) []string {
	w, err := parseTree(worldDoc)
	if err != nil {
		return nil
	}
	m, err := parseTree(modelDoc)
	if err != nil {
		return nil
	}
	have := map[string]bool{}
	for _, world := range w.children {
		if world.name != "world" {
			continue
		}
		for _, c := range world.children {
			if c.name == "plugin" {
				for _, k := range systemKeys(c) {
					have[k] = true
				}
			}
		}
	}
	seen := map[string]bool{}
	var out []string
	walk(m, func(n *node) bool {
		if n.name == "sensor" {
			typ := n.attr("type")
			if cpuLidar[typ] {
				typ = "gpu_lidar"
			}
			if s, ok := sensorSystems[typ]; ok && !have[s.key] && !seen[s.key] {
				seen[s.key] = true
				out = append(out, fmt.Sprintf("%s (for %s)", s.plugin, typ))
			}
		}
		return true
	})
	return out
}
