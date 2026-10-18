#!/usr/bin/env bash
# Runs a project's world in Gazebo (headless) and reports which sensor and
# odometry topics publish data.
#
#   gzcheck.sh PROJECT_DIR [SECONDS]
#
# PROJECT_DIR holds world.sdf and models/. Uses the gz-websocket image
# (Gazebo Harmonic); the local install isn't needed. Prints one line per
# sensor topic, by the kind of data it carries: "ok <kind> <topic>" or
# "silent <kind> <topic>", then the sim's errors. Kinds: image, scan,
# points, imu, odometry, camera_info, navsat, contacts, joint_state.
set -euo pipefail
dir="$(cd "$1" && pwd)"
secs="${2:-8}"
image="${GZCHECK_IMAGE:-gz-websocket:latest}"
gpu=()
if command -v nvidia-smi >/dev/null 2>&1; then gpu=(--gpus all); fi
docker run --rm "${gpu[@]}" --user "$(id -u):$(id -g)" -e HOME=/tmp -e GZ_PARTITION="gzcheck$$" \
  -e GZ_SIM_RESOURCE_PATH=/project/models -v "$dir:/project:ro" --entrypoint bash "$image" -c '
cd /tmp
gz sim -s -r --headless-rendering -v 2 /project/world.sdf >/tmp/sim.log 2>&1 &
# Wait for the world to load (Fuel includes download first), then let
# sensors start.
for i in $(seq 1 120); do gz topic -l 2>/dev/null | grep -q "/stats$" && break; sleep 1; done
sleep '"$secs"'
# Classify each published topic by message type, so custom topic names
# count too, then require one message on it.
for t in $(gz topic -l | sort); do
  type=$(gz topic -i -t "$t" 2>/dev/null | awk "/Publishers/{p=1;next} /Subscribers/{p=0} p && /gz.msgs/{print \$NF; exit}")
  case "$type" in
    gz.msgs.Image) kind=image ;;
    gz.msgs.LaserScan) kind=scan ;;
    gz.msgs.PointCloudPacked) kind=points ;;
    gz.msgs.IMU) kind=imu ;;
    gz.msgs.Odometry) kind=odometry ;;
    gz.msgs.CameraInfo) kind=camera_info ;;
    gz.msgs.NavSat) kind=navsat ;;
    gz.msgs.Contacts) kind=contacts ;;
    *) case "$t" in */joint_state) kind=joint_state ;; *) continue ;; esac ;;
  esac
  if timeout 5 gz topic -e -n 1 -t "$t" >/dev/null 2>&1; then echo "ok $kind $t"; else echo "silent $kind $t"; fi
done
echo "--- sim errors"
grep -iE "\[(err|error)\]|\[Err\]" /tmp/sim.log | sort | uniq -c | head -20 || true
kill %1 2>/dev/null || true'
