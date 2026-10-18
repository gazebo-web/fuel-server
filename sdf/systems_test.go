package sdf

import (
	"strings"
	"testing"
)

const defaults = `<plugin filename="gz-sim-physics-system" name="gz::sim::systems::Physics"/>
<plugin filename="gz-sim-user-commands-system" name="gz::sim::systems::UserCommands"/>
<plugin filename="gz-sim-scene-broadcaster-system" name="gz::sim::systems::SceneBroadcaster"/>`

const rover = `<?xml version="1.0"?><sdf version="1.9"><model name="rover"><link name="base">
<sensor name="cam" type="camera"/><sensor name="imu" type="imu"/></link></model></sdf>`

func world(body string) string {
	return `<?xml version="1.0"?><sdf version="1.9"><world name="w">` + body + `</world></sdf>`
}

func TestWorldSystems(t *testing.T) {
	inc := Options{IncludedModel: func(uri string) (string, bool) { return rover, uri == "model://rover" }}
	for name, c := range map[string]struct {
		doc       string
		want, not []string
	}{
		"no plugins, no sensors": {doc: world(""), not: []string{"systems"}},
		"sensor needs Sensors": {
			doc:  world(`<model name="m"><link name="l"><sensor name="s" type="gpu_lidar"/></link></model>`),
			want: []string{"gpu_lidar sensors (sensor s) publish nothing", "gz-sim-sensors-system"},
			not:  []string{"defaults"},
		},
		"only Sensors listed": {
			doc:  world(`<plugin filename="gz-sim-sensors-system" name="gz::sim::systems::Sensors"/>`),
			want: []string{"loads none of its defaults", "gz-sim-physics-system", "gz-sim-scene-broadcaster-system"},
		},
		"included model's sensors": {
			doc: world(defaults + `<include><uri>model://rover</uri><name>r1</name></include>`),
			want: []string{"camera sensors (model r1)", "gz-sim-sensors-system", "imu sensors (model r1)",
				"gz-sim-imu-system"},
		},
		"all present, other spellings": {
			doc: world(defaults + `<plugin filename="libgz-sim8-sensors-system.so" name="x"/>` +
				`<plugin filename="ignition-gazebo-imu-system" name="y"/>` +
				`<include><uri>model://rover</uri></include>`),
			not: []string{"publish nothing", "defaults"},
		},
		"cpu lidar": {
			doc:  world(`<model name="m"><link name="l"><sensor name="s" type="ray"/></link></model>`),
			want: []string{`use type "gpu_lidar"`},
		},
	} {
		r := ValidateWithOptions(c.doc, inc)
		got := r.Summary("")
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: missing %q in\n%s", name, w, got)
			}
		}
		for _, w := range c.not {
			if strings.Contains(got, w) {
				t.Errorf("%s: unexpected %q in\n%s", name, w, got)
			}
		}
	}
}

func TestMissingWorldSystems(t *testing.T) {
	got := strings.Join(MissingWorldSystems(world(defaults), rover), " ")
	if !strings.Contains(got, "gz-sim-sensors-system") || !strings.Contains(got, "gz-sim-imu-system") {
		t.Fatalf("got %q", got)
	}
	if got := MissingWorldSystems(world(defaults+`<plugin filename="gz-sim-sensors-system" name="gz::sim::systems::Sensors"/><plugin filename="gz-sim-imu-system" name="gz::sim::systems::Imu"/>`), rover); len(got) != 0 {
		t.Fatalf("got %q", got)
	}
}
