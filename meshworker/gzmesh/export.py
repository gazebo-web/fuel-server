"""Model folder output (plan §5.4): GLB visuals, collision GLBs, model.sdf,
model.config.

Vertices are written in Gazebo's frame (Z up, +X forward) with no root
node rotation: Gazebo's assimp loader and gzweb's GLTFLoader both use glTF
coordinates as-is (meshworker/spike/README.md).

Every link's frame is the model frame (no link poses), so meshes, inertia,
joint origins and axes all share the frame the code was written in. A
one-link model keeps the simple names: link "link", meshes/<name>.glb and
meshes/<name>_collision_<i>.glb. Links of an articulated model get
meshes/<name>_<link>.glb and meshes/<name>_<link>_collision_<i>.glb.
"""

import os
from dataclasses import dataclass, field
from typing import Optional
from xml.sax.saxutils import escape

import numpy as np
import trimesh

from .collision import Shape
from .inertia import Inertial
from .parts import Joint, Sensor

# Revolute limits that mean "unlimited" to Gazebo (as in its diff_drive
# example), for continuous joints.
UNLIMITED = 1.79769e308


@dataclass
class LinkOut:
    name: str
    visual: trimesh.Trimesh
    inertial: Inertial
    shapes: list
    stem: str  # file stem: <model> for a one-link model, else <model>_<link>
    friction: Optional[float] = None
    sensors: list = field(default_factory=list)  # parts.Sensor, poses already offset


def num(x: float) -> str:
    x = float(x)
    return "0" if abs(x) < 1e-12 else f"{x:.6g}"


def pose(p) -> str:
    return " ".join(num(v) for v in p)


def write_glb(mesh, path: str):
    """mesh: one Trimesh, or a Scene (an imported mesh keeps its parts and
    materials)."""
    os.makedirs(os.path.dirname(path), exist_ok=True)
    scene = mesh if isinstance(mesh, trimesh.Scene) else trimesh.Scene(mesh)
    with open(path, "wb") as f:
        f.write(trimesh.exchange.gltf.export_glb(scene, include_normals=True))


def collision_xml(i: int, s: Shape, stem: str, friction: Optional[float] = None) -> str:
    if s.kind == "box":
        geom = f"<box><size>{pose(s.size)}</size></box>"
    elif s.kind == "cylinder":
        geom = f"<cylinder><radius>{num(s.radius)}</radius><length>{num(s.length)}</length></cylinder>"
    elif s.kind == "sphere":
        geom = f"<sphere><radius>{num(s.radius)}</radius></sphere>"
    else:
        geom = f"<mesh><uri>meshes/{stem}_collision_{i}.glb</uri></mesh>"
    p = f"\n        <pose>{pose(s.pose)}</pose>" if any(abs(v) > 1e-12 for v in s.pose) else ""
    surface = "" if friction is None else (
        f"\n        <surface><friction><ode><mu>{num(friction)}</mu><mu2>{num(friction)}</mu2></ode></friction></surface>")
    return f"""      <collision name="collision_{i}">{p}
        <geometry>{geom}</geometry>{surface}
      </collision>"""


def sensor_xml(s: Sensor) -> str:
    """One <sensor>; s.pos is in the link frame (the model frame, offset)."""
    k = s.params
    if s.kind in ("camera", "depth_camera", "rgbd_camera"):
        fmt = "<format>R_FLOAT32</format>" if s.kind == "depth_camera" else ""
        body = f"""
        <camera>
          <horizontal_fov>{num(k["fov"])}</horizontal_fov>
          <image><width>{k["width"]}</width><height>{k["height"]}</height>{fmt}</image>
          <clip><near>{num(k["near"])}</near><far>{num(k["far"])}</far></clip>
        </camera>"""
    elif s.kind == "gpu_lidar":
        vertical = "" if k["vertical_samples"] <= 1 else f"""
            <vertical><samples>{k["vertical_samples"]}</samples><resolution>1</resolution><min_angle>{num(-k["vertical_fov"] / 2)}</min_angle><max_angle>{num(k["vertical_fov"] / 2)}</max_angle></vertical>"""
        body = f"""
        <lidar>
          <scan>
            <horizontal><samples>{k["samples"]}</samples><resolution>1</resolution><min_angle>{num(-k["fov"] / 2)}</min_angle><max_angle>{num(k["fov"] / 2)}</max_angle></horizontal>{vertical}
          </scan>
          <range><min>{num(k["min_range"])}</min><max>{num(k["max_range"])}</max><resolution>0.01</resolution></range>
        </lidar>"""
    else:
        body = ""
    return f"""      <sensor name="{s.name}" type="{s.kind}">
        <pose>{pose([*s.pos, *s.rpy])}</pose>
        <always_on>true</always_on>
        <update_rate>{num(s.rate)}</update_rate>
        <visualize>true</visualize>{body}
      </sensor>"""


def plugin_xml(kind: str, k: dict) -> str:
    """One model <plugin>; k holds resolved parameters (wheel geometry
    filled in)."""
    if kind == "diff_drive":
        joints = "".join(f"\n      <left_joint>{n}</left_joint>" for n in k["left"]) + \
                 "".join(f"\n      <right_joint>{n}</right_joint>" for n in k["right"])
        return f"""    <plugin filename="gz-sim-diff-drive-system" name="gz::sim::systems::DiffDrive">{joints}
      <wheel_separation>{num(k["wheel_separation"])}</wheel_separation>
      <wheel_radius>{num(k["wheel_radius"])}</wheel_radius>
      <max_linear_velocity>{num(k["max_speed"])}</max_linear_velocity>
      <min_linear_velocity>{num(-k["max_speed"])}</min_linear_velocity>
      <odom_publish_frequency>20</odom_publish_frequency>
    </plugin>"""
    if kind == "joint_states":
        return """    <plugin filename="gz-sim-joint-state-publisher-system" name="gz::sim::systems::JointStatePublisher"/>"""
    if kind == "position_controller":
        limit = "" if "max_effort" not in k else (
            f"\n      <cmd_max>{num(k['max_effort'])}</cmd_max><cmd_min>{num(-k['max_effort'])}</cmd_min>")
        return f"""    <plugin filename="gz-sim-joint-position-controller-system" name="gz::sim::systems::JointPositionController">
      <joint_name>{k["joint"]}</joint_name>
      <p_gain>{num(k["p_gain"])}</p_gain>
      <d_gain>{num(k["d_gain"])}</d_gain>{limit}
    </plugin>"""
    if kind == "velocity_controller":
        return f"""    <plugin filename="gz-sim-joint-controller-system" name="gz::sim::systems::JointController">
      <joint_name>{k["joint"]}</joint_name>
    </plugin>"""
    raise ValueError(f"unknown plugin {kind!r}")


def link_xml(lk: LinkOut) -> str:
    I, c = lk.inertial.inertia, lk.inertial.center
    collisions = "\n".join(collision_xml(i, s, lk.stem, lk.friction) for i, s in enumerate(lk.shapes))
    sensors = "".join("\n" + sensor_xml(s) for s in lk.sensors)
    return f"""    <link name="{lk.name}">
      <inertial>
        <pose>{pose([*c, 0, 0, 0])}</pose>
        <mass>{num(lk.inertial.mass)}</mass>
        <inertia>
          <ixx>{num(I[0, 0])}</ixx><ixy>{num(I[0, 1])}</ixy><ixz>{num(I[0, 2])}</ixz>
          <iyy>{num(I[1, 1])}</iyy><iyz>{num(I[1, 2])}</iyz>
          <izz>{num(I[2, 2])}</izz>
        </inertia>
      </inertial>
      <visual name="visual">
        <geometry><mesh><uri>meshes/{lk.stem}.glb</uri></mesh></geometry>
      </visual>
{collisions}{sensors}
    </link>"""


def joint_xml(j: Joint, offset: np.ndarray) -> str:
    """The joint frame sits at the origin with the model's axes; the child
    link's frame is the model frame, so the pose is the origin itself."""
    if j.kind == "fixed":
        return f"""    <joint name="{j.name}" type="fixed">
      <parent>{j.parent}</parent>
      <child>{j.child}</child>
    </joint>"""
    lower, upper = (j.lower, j.upper) if j.kind != "continuous" else (-UNLIMITED, UNLIMITED)
    kind = "revolute" if j.kind == "continuous" else j.kind
    return f"""    <joint name="{j.name}" type="{kind}">
      <parent>{j.parent}</parent>
      <child>{j.child}</child>
      <pose>{pose([*(j.origin + offset), 0, 0, 0])}</pose>
      <axis>
        <xyz>{pose(j.axis)}</xyz>
        <limit>
          <lower>{num(lower)}</lower>
          <upper>{num(upper)}</upper>
          <effort>{num(j.effort)}</effort>
          <velocity>{num(j.velocity)}</velocity>
        </limit>
        <dynamics>
          <damping>{num(j.damping)}</damping>
          <friction>{num(j.friction)}</friction>
        </dynamics>
      </axis>
    </joint>"""


GENERATED_NOTE = ("Generated by the AI Builder mesh worker from source/model.py.\n"
                  "     Edit the source and regenerate; don't edit the meshes.")


def model_sdf(name: str, sdf_version: str, static: bool, links: list, joints: list, offset: np.ndarray,
              plugins: tuple = (), note: str = GENERATED_NOTE) -> str:
    """plugins: (kind, resolved params) pairs."""
    body = "\n".join([link_xml(lk) for lk in links] + [joint_xml(j, offset) for j in joints] +
                     [plugin_xml(kind, k) for kind, k in plugins])
    return f"""<?xml version="1.0"?>
<!-- {note} -->
<sdf version="{sdf_version}">
  <model name="{name}">
    <static>{"true" if static else "false"}</static>
{body}
  </model>
</sdf>
"""


def model_config(name: str, sdf_version: str, description: str) -> str:
    return f"""<?xml version="1.0"?>
<model>
  <name>{escape(name)}</name>
  <version>1.0</version>
  <sdf version="{sdf_version}">model.sdf</sdf>
  <author><name>AI Builder</name></author>
  <description>{escape(description)}</description>
</model>
"""


def write_model(root: str, name: str, *, links: list, joints: list, offset: np.ndarray, static: bool,
                sdf_version: str, description: str, source: str, plugins: tuple = (),
                source_path: str = "source/model.py", note: str = GENERATED_NOTE) -> list[str]:
    """Writes the model folder under root; returns the paths written,
    relative to root. source is saved as source_path: the code a model was
    generated from, or an imported model's import settings."""
    files = []
    for lk in links:
        write_glb(lk.visual, os.path.join(root, "meshes", f"{lk.stem}.glb"))
        files.append(f"meshes/{lk.stem}.glb")
        for i, s in enumerate(lk.shapes):
            if s.kind == "mesh":
                rel = f"meshes/{lk.stem}_collision_{i}.glb"
                write_glb(s.mesh, os.path.join(root, rel))
                files.append(rel)
    texts = {
        "model.sdf": model_sdf(name, sdf_version, static, links, joints, offset, plugins, note),
        "model.config": model_config(name, sdf_version, description),
        source_path: source,
    }
    for rel, text in texts.items():
        path = os.path.join(root, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w") as f:
            f.write(text)
        files.append(rel)
    return sorted(files)


def translate_shapes(shapes: list[Shape], offset: np.ndarray):
    for s in shapes:
        if s.kind == "mesh":
            s.mesh.apply_translation(offset)
        else:
            s.pose[:3] = [float(a + b) for a, b in zip(s.pose[:3], offset)]
