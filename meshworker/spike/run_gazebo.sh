#!/usr/bin/env bash
# C1 format spike: load the exported models in Gazebo, headless, and save
# camera images plus the cone's resting pose.
#
#   ./run_gazebo.sh OUT_DIR [GZ_SIM_MAJOR]     (default 8 = Harmonic)
#
# Needs OUT_DIR/models from export.py. Writes OUT_DIR/gz<major>/: images
# from both cameras, cone_pose.txt (the last published pose) and sim.log.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
out="$(cd "$1" && pwd)"
major="${2:-8}"
dir="$out/gz$major"
mkdir -p "$dir/three_quarter" "$dir/side"

export GZ_SIM_RESOURCE_PATH="$out/models"
sed "s|\$SPIKE_IMAGES|$dir|g" "$here/world.sdf" >"$dir/world.sdf"

# 10 s of sim time (the cameras need a few seconds to initialize, then
# save one frame per second).
gz sim --force-version "$major" -s -r --headless-rendering --iterations 10000 -v 3 \
  "$dir/world.sdf" >"$dir/sim.log" 2>&1 &
sim=$!
# Cameras only render while something subscribes, so <save> writes nothing
# without these.
for cam in three_quarter side; do
  timeout 20 gz topic -e -t "/world/spike/model/cameras/link/link/sensor/$cam/image" >/dev/null 2>&1 &
done
timeout 20 gz topic -e -t /model/spike_cone/pose >"$dir/cone_poses.txt" 2>/dev/null || true
wait "$sim" || echo "gz sim exited with $?" >&2
# The last message is the resting pose.
awk '/^position/{p=""} {p=p $0 "\n"} END{printf "%s", p}' "$dir/cone_poses.txt" >"$dir/cone_pose.txt" || true
echo "images: $(find "$dir" -name '*.png' | wc -l) in $dir"
grep -iE 'error|warn' "$dir/sim.log" | grep -v 'Unable to find file with URI \[file://media' | head -20 || true
