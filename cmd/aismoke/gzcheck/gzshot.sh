#!/usr/bin/env bash
# Renders an overview image of a project's world in Gazebo (headless): a
# camera placed above and to the side of the models' poses, looking at
# their center.
#
#   gzshot.sh PROJECT_DIR OUT.png
set -euo pipefail
dir="$(cd "$1" && pwd)"
out="$(cd "$(dirname "$2")" && pwd)/$(basename "$2")"
work="$(mktemp -d -p "${GZSHOT_TMP:-$HOME/.cache}" gzshot.XXXX)"
python3 - "$dir/world.sdf" "$work/world.sdf" <<'PY'
import math, re, sys, xml.etree.ElementTree as ET
src, dst = sys.argv[1:]
tree = ET.parse(src)
world = tree.getroot().find("world")
def xyz(el):
    p = el.find("pose")
    return [float(v) for v in p.text.split()[:3]] if p is not None and p.text else [0.0, 0.0, 0.0]

# Frame what's in the world: top-level models and includes, and each
# model's links, ignoring rotations. The ground plane doesn't count.
pts = []
for el in list(world.findall("model")) + list(world.findall("include")):
    if el.find(".//plane") is not None:
        continue
    base = xyz(el)
    links = el.findall("link")
    if not links:
        pts.append(base)
    for lk in links:
        lp = [a + b for a, b in zip(base, xyz(lk))]
        pts.append(lp)
        for vis in lk.findall("visual"):
            vp = [a + b for a, b in zip(lp, xyz(vis))]
            pts.append(vp)
            for size in vis.iter("size"):
                v = [float(x) for x in size.text.split()]
                if len(v) == 3:
                    for sx in (-0.5, 0.5):
                        for sy in (-0.5, 0.5):
                            pts.append([vp[0] + sx * v[0], vp[1] + sy * v[1], vp[2]])
if not pts:
    pts = [[0, 0, 0]]
lo = [min(p[i] for p in pts) for i in range(3)]
hi = [max(p[i] for p in pts) for i in range(3)]
c = [(a + b) / 2 for a, b in zip(lo, hi)]
c[2] = lo[2]
r = max(3.0, math.hypot(hi[0] - lo[0], hi[1] - lo[1]) * 0.8)
# Looking down about 55 degrees from the -X -Y side, so walls don't hide
# the inside of rooms.
eye = [c[0] - r * 0.45, c[1] - r * 0.65, c[2] + r * 1.15]
d = [c[i] - eye[i] for i in range(3)]
yaw = math.atan2(d[1], d[0])
pitch = math.atan2(-d[2], math.hypot(d[0], d[1]))
cam = ET.fromstring(f"""<model name="gzshot_camera"><static>true</static>
<pose>{eye[0]} {eye[1]} {eye[2]} 0 {pitch} {yaw}</pose><link name="link">
<sensor name="shot" type="camera"><always_on>true</always_on><update_rate>2</update_rate><topic>gzshot</topic>
<camera><horizontal_fov>1.2</horizontal_fov><image><width>1280</width><height>800</height></image>
<clip><near>0.1</near><far>500</far></clip><save enabled="true"><path>/work/frames</path></save></camera>
</sensor></link></model>""")
world.append(cam)
names = {(p.get("filename") or "").split("/")[-1] for p in world.findall("plugin")}
if not any("sensors" in n for n in names):
    world.append(ET.fromstring('<plugin filename="gz-sim-sensors-system" name="gz::sim::systems::Sensors"><render_engine>ogre2</render_engine></plugin>'))
    if not any("physics" in n for n in names):
        for f, n in (("physics", "Physics"), ("user-commands", "UserCommands"), ("scene-broadcaster", "SceneBroadcaster")):
            world.append(ET.fromstring(f'<plugin filename="gz-sim-{f}-system" name="gz::sim::systems::{n}"/>'))
tree.write(dst)
PY
gpu=()
if command -v nvidia-smi >/dev/null 2>&1; then gpu=(--gpus all); fi
docker run --rm "${gpu[@]}" --user "$(id -u):$(id -g)" -e HOME=/tmp -e GZ_PARTITION="gzshot$$" \
  -e GZ_SIM_RESOURCE_PATH=/project/models -v "$dir:/project:ro" -v "$work:/work" --entrypoint bash \
  gz-websocket:latest -c '
cd /tmp; mkdir -p /work/frames
gz sim -s -r --headless-rendering -v 1 /work/world.sdf >/tmp/sim.log 2>&1 &
for i in $(seq 1 120); do gz topic -l 2>/dev/null | grep -q "/stats$" && break; sleep 1; done
timeout 12 gz topic -e -t /gzshot >/dev/null 2>&1 || true
kill %1 2>/dev/null || true'
last=$(ls "$work"/frames/*.png 2>/dev/null | sort | tail -1)
test -n "$last" && cp "$last" "$out" && echo "$out"
