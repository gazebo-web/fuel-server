"""The contract between agent-written code and the pipeline (plan §5.2).

The code defines build(), which returns a build123d shape, a Part from
part(), a list of those (one rigid model), or a model() of links connected
by joints (an articulated one: a cabinet with a hinged door, a drawer, a
wheeled robot), with optional sensors and controllers. Everything else
(tessellation, UVs, materials, collision, inertia, files) is the
pipeline's job.

Everything is written in the model frame as built: parts where they sit
when the model is assembled, joint origins and axes in the same frame.
"""

import math
import re
from dataclasses import dataclass, field
from typing import Optional, Sequence

import numpy as np

from .materials import Material, plastic

MAX_LINKS = 16
MAX_SENSORS = 16
NAME_RE = re.compile(r"^[a-z][a-z0-9_]{0,31}$")
JOINT_KINDS = ("revolute", "prismatic", "continuous", "fixed")


@dataclass
class Part:
    shape: object  # a build123d Shape (Solid, Part, Compound)
    name: str
    material: Material


@dataclass
class Link:
    name: str
    parts: list
    friction: Optional[float] = None  # Coulomb mu on every collision; None keeps Gazebo's default (1)


@dataclass
class Joint:
    name: str
    kind: str
    parent: str
    child: str
    origin: np.ndarray = field(default_factory=lambda: np.zeros(3))  # model frame, m
    axis: np.ndarray = field(default_factory=lambda: np.array([0.0, 0.0, 1.0]))
    lower: Optional[float] = None  # rad (revolute) or m (prismatic)
    upper: Optional[float] = None
    effort: float = 100.0
    velocity: float = 10.0
    damping: float = 0.0
    friction: float = 0.0


@dataclass
class Sensor:
    name: str
    kind: str  # camera, depth_camera, rgbd_camera, gpu_lidar, imu
    link: str
    pos: np.ndarray  # model frame, m
    rpy: np.ndarray  # rad; the sensor looks along its +X
    rate: float  # Hz
    params: dict = field(default_factory=dict)


@dataclass
class Plugin:
    kind: str  # diff_drive, joint_states, position_controller, velocity_controller
    params: dict = field(default_factory=dict)


@dataclass
class Assembly:
    links: list
    joints: list
    sensors: list = field(default_factory=list)
    plugins: list = field(default_factory=list)


def part(shape, name: str = "part", material: Optional[Material] = None) -> Part:
    """A named piece of the model with its material (default: grey plastic)."""
    if not hasattr(shape, "wrapped"):
        raise TypeError(f"part {name!r}: want a build123d shape, got {type(shape).__name__}")
    if material is not None and not isinstance(material, Material):
        raise TypeError(f"part {name!r}: material must come from gzmesh (e.g. gz.plastic('red')), "
                        f"got {type(material).__name__}")
    return Part(shape, name, material or plastic("grey"))


def link(name: str, *parts, friction: Optional[float] = None) -> Link:
    """A rigid body of the model: parts (or shapes, or lists of them) that
    move together. friction sets the Coulomb coefficient of its collisions
    (0 for a caster ball that slides, about 1 for tires)."""
    flat = []
    for p in parts:
        flat.extend(p if isinstance(p, (list, tuple)) else [p])
    if friction is not None and not (isinstance(friction, (int, float)) and 0 <= friction <= 10):
        raise ValueError(f"link {name!r}: friction must be a number from 0 to 10")
    return Link(name, flat, None if friction is None else float(friction))


def _vec(v, what) -> np.ndarray:
    a = np.asarray(v, float)
    if a.shape != (3,) or not np.isfinite(a).all():
        raise ValueError(f"{what} must be three numbers, got {v!r}")
    return a


def _joint(kind, name, parent, child, origin, axis, lower=None, upper=None, **kw) -> Joint:
    a = _vec(axis, f"joint {name!r} axis")
    if np.linalg.norm(a) < 1e-9:
        raise ValueError(f"joint {name!r} axis must not be zero")
    return Joint(name, kind, parent, child, _vec(origin, f"joint {name!r} origin"), a / np.linalg.norm(a),
                 lower, upper, **kw)


def revolute(name: str, parent: str, child: str, origin, axis, lower: float, upper: float,
             damping: float = 0.5, friction: float = 0.0, effort: float = 100.0, velocity: float = 3.0) -> Joint:
    """A hinge: child rotates about `axis` through `origin` (model frame),
    from `lower` to `upper` radians. 0 is the pose as built."""
    return _joint("revolute", name, parent, child, origin, axis, float(lower), float(upper),
                  damping=damping, friction=friction, effort=effort, velocity=velocity)


def prismatic(name: str, parent: str, child: str, origin, axis, lower: float, upper: float,
              damping: float = 2.0, friction: float = 0.0, effort: float = 100.0, velocity: float = 1.0) -> Joint:
    """A slider (a drawer): child moves along `axis` from `lower` to `upper`
    meters. 0 is the pose as built."""
    return _joint("prismatic", name, parent, child, origin, axis, float(lower), float(upper),
                  damping=damping, friction=friction, effort=effort, velocity=velocity)


def continuous(name: str, parent: str, child: str, origin, axis, damping: float = 0.0,
               friction: float = 0.0, effort: float = 100.0, velocity: float = 50.0) -> Joint:
    """An unlimited rotation (a wheel) about `axis` through `origin`."""
    return _joint("continuous", name, parent, child, origin, axis,
                  damping=damping, friction=friction, effort=effort, velocity=velocity)


def fixed(name: str, parent: str, child: str) -> Joint:
    """Two links welded together (rarely needed: put the parts in one link)."""
    return Joint(name, "fixed", parent, child)


def model(links: Sequence[Link], joints: Sequence[Joint] = (), sensors: Sequence[Sensor] = (),
          plugins: Sequence[Plugin] = ()) -> Assembly:
    """A model of links connected into a tree by joints, with sensors on
    its links and plugins (controllers) that drive it."""
    return Assembly(list(links), list(joints), list(sensors), list(plugins))


# -- sensors -------------------------------------------------------------------
#
# Positions are in the model frame as built, like joint origins; rpy turns
# the sensor (radians, roll about X, then pitch about Y, then yaw about Z).
# Cameras and lidars look along the sensor's +X, so rpy=(0, 0, 0) looks
# toward the model's front.

def _num(v, what, lo, hi, integer=False):
    if isinstance(v, bool) or not isinstance(v, (int, float)) or not math.isfinite(v) or not lo <= v <= hi:
        raise ValueError(f"{what} must be a number from {lo} to {hi}, got {v!r}")
    if integer and int(v) != v:
        raise ValueError(f"{what} must be a whole number, got {v!r}")
    return int(v) if integer else float(v)


def _sensor(kind, name, link, pos, rpy, rate, **params) -> Sensor:
    return Sensor(name, kind, link, _vec(pos, f"sensor {name!r} pos"), _vec(rpy, f"sensor {name!r} rpy"),
                  _num(rate, f"sensor {name!r} rate", 0.1, 1000), params)


def _camera(kind, name, link, pos, rpy, width, height, fov, rate, near, far) -> Sensor:
    w = f"sensor {name!r}"
    near, far = _num(near, f"{w} near", 0.001, 10), _num(far, f"{w} far", 0.01, 10_000)
    if near >= far:
        raise ValueError(f"{w}: near ({near}) must be less than far ({far})")
    return _sensor(kind, name, link, pos, rpy, rate, width=_num(width, f"{w} width", 16, 4096, True),
                   height=_num(height, f"{w} height", 16, 4096, True), fov=_num(fov, f"{w} fov", 0.05, 3.1),
                   near=near, far=far)


def camera(name: str, link: str, pos, rpy=(0, 0, 0), width: int = 640, height: int = 480, fov: float = 1.047,
           rate: float = 30, near: float = 0.05, far: float = 100) -> Sensor:
    """A color camera. fov is the horizontal field of view in radians."""
    return _camera("camera", name, link, pos, rpy, width, height, fov, rate, near, far)


def depth_camera(name: str, link: str, pos, rpy=(0, 0, 0), width: int = 640, height: int = 480,
                 fov: float = 1.047, rate: float = 30, near: float = 0.1, far: float = 10) -> Sensor:
    """A depth camera: an image of distances in meters."""
    return _camera("depth_camera", name, link, pos, rpy, width, height, fov, rate, near, far)


def rgbd_camera(name: str, link: str, pos, rpy=(0, 0, 0), width: int = 640, height: int = 480,
                fov: float = 1.047, rate: float = 30, near: float = 0.1, far: float = 10) -> Sensor:
    """A color camera and a depth camera in one (like a RealSense)."""
    return _camera("rgbd_camera", name, link, pos, rpy, width, height, fov, rate, near, far)


def lidar(name: str, link: str, pos, rpy=(0, 0, 0), samples: int = 360, fov: float = 2 * math.pi,
          vertical_samples: int = 1, vertical_fov: float = 0.0, min_range: float = 0.1, max_range: float = 30,
          rate: float = 10) -> Sensor:
    """A scanning lidar. fov is the horizontal sweep in radians, centered
    on +X; vertical_samples > 1 makes a 3D lidar spanning vertical_fov."""
    w = f"sensor {name!r}"
    lo, hi = _num(min_range, f"{w} min_range", 0.001, 100), _num(max_range, f"{w} max_range", 0.01, 1000)
    if lo >= hi:
        raise ValueError(f"{w}: min_range ({lo}) must be less than max_range ({hi})")
    vs = _num(vertical_samples, f"{w} vertical_samples", 1, 256, True)
    vf = _num(vertical_fov, f"{w} vertical_fov", 0, math.pi)
    if vs > 1 and vf <= 0:
        raise ValueError(f"{w}: a lidar with vertical_samples > 1 needs a vertical_fov")
    return _sensor("gpu_lidar", name, link, pos, rpy, rate, samples=_num(samples, f"{w} samples", 1, 8192, True),
                   fov=_num(fov, f"{w} fov", 0.01, 2 * math.pi), vertical_samples=vs, vertical_fov=vf,
                   min_range=lo, max_range=hi)


def imu(name: str, link: str, pos=(0, 0, 0), rpy=(0, 0, 0), rate: float = 100) -> Sensor:
    """An IMU: orientation, angular velocity and linear acceleration."""
    return _sensor("imu", name, link, pos, rpy, rate)


# -- plugins -------------------------------------------------------------------

def _joints(v, what) -> list:
    names = [v] if isinstance(v, str) else list(v)
    if not names or not all(isinstance(n, str) for n in names):
        raise ValueError(f"{what} must be a joint name or a list of them")
    return names


def diff_drive(left, right, wheel_separation: Optional[float] = None, wheel_radius: Optional[float] = None,
               max_speed: float = 1.0) -> Plugin:
    """Differential drive: steers by turning the left and right wheel joints
    (names, or lists for several wheels a side) at different speeds, from
    velocity commands on /model/<instance>/cmd_vel; publishes odometry.
    The front is +X and the left side +Y. wheel_separation and
    wheel_radius default to what the model's geometry says."""
    p = {"left": _joints(left, "diff_drive left"), "right": _joints(right, "diff_drive right"),
         "max_speed": _num(max_speed, "diff_drive max_speed", 0.01, 100)}
    if wheel_separation is not None:
        p["wheel_separation"] = _num(wheel_separation, "diff_drive wheel_separation", 0.001, 100)
    if wheel_radius is not None:
        p["wheel_radius"] = _num(wheel_radius, "diff_drive wheel_radius", 0.001, 10)
    return Plugin("diff_drive", p)


def joint_states() -> Plugin:
    """Publishes every joint's position and velocity on
    /world/<world>/model/<instance>/joint_state."""
    return Plugin("joint_states", {})


def position_controller(joint: str, p_gain: float = 10.0, d_gain: float = 0.5, max_effort: Optional[float] = None) -> Plugin:
    """Holds a joint at the position (rad or m) sent on
    /model/<instance>/joint/<joint>/0/cmd_pos, for arms and grippers."""
    out = {"joint": _joints(joint, "position_controller joint")[0],
           "p_gain": _num(p_gain, "position_controller p_gain", 0, 1e6),
           "d_gain": _num(d_gain, "position_controller d_gain", 0, 1e6)}
    if max_effort is not None:
        out["max_effort"] = _num(max_effort, "position_controller max_effort", 0, 1e6)
    return Plugin("position_controller", out)


def velocity_controller(joint: str) -> Plugin:
    """Turns a joint at the speed (rad/s or m/s) sent on
    /model/<instance>/joint/<joint>/cmd_vel, for conveyors and fans."""
    return Plugin("velocity_controller", {"joint": _joints(joint, "velocity_controller joint")[0]})


def _as_parts(items, prefix) -> list:
    out = []
    for i, it in enumerate(items):
        out.append(it if isinstance(it, Part) else part(it, f"{prefix}_{i}"))
    return out


def normalize(result) -> Assembly:
    """Turn whatever build() returned into a checked Assembly. A plain part
    or list of parts is one link named "link" with no joints."""
    if isinstance(result, Assembly):
        asm = result
        for lk in asm.links:
            if not isinstance(lk, Link):
                raise TypeError(f"model() links must come from gz.link(), got {type(lk).__name__}")
            lk.parts = _as_parts(lk.parts, lk.name)
    else:
        items = result if isinstance(result, (list, tuple)) else [result]
        if any(isinstance(i, (Link, Joint, Sensor, Plugin)) for i in items):
            raise TypeError("return gz.model(links=[...], joints=[...], sensors=[...], plugins=[...]) "
                            "for links, joints, sensors and plugins")
        asm = Assembly([Link("link", _as_parts(items, "part"))], [])
    _check(asm)
    return asm


def _check(asm: Assembly):
    if not asm.links:
        raise ValueError("build() returned no parts")
    if len(asm.links) > MAX_LINKS:
        raise ValueError(f"at most {MAX_LINKS} links; merge parts that move together into one link")
    names = [lk.name for lk in asm.links]
    for n in names:
        if not NAME_RE.match(n):
            raise ValueError(f"link name {n!r}: lowercase letters, digits and '_', starting with a letter")
    if len(set(names)) != len(names):
        raise ValueError(f"link names must be unique: {names}")
    seen = {}
    for lk in asm.links:
        if not lk.parts:
            raise ValueError(f"link {lk.name!r} has no parts")
        for p in lk.parts:
            n = seen.get(p.name, 0)
            seen[p.name] = n + 1
            if n:
                p.name = f"{p.name}_{n}"
            vol = getattr(p.shape, "volume", 0) or 0
            if vol <= 0:
                raise ValueError(f"part {p.name!r} has no volume: build() must return solids "
                                 f"(extrude sketches, or use Box/Cylinder/...)")

    jnames = [j.name for j in asm.joints]
    if len(set(jnames)) != len(jnames):
        raise ValueError(f"joint names must be unique: {jnames}")
    parent_of = {}
    for j in asm.joints:
        if not isinstance(j, Joint):
            raise TypeError(f"model() joints must come from gz.revolute/prismatic/continuous/fixed, "
                            f"got {type(j).__name__}")
        if not NAME_RE.match(j.name):
            raise ValueError(f"joint name {j.name!r}: lowercase letters, digits and '_', starting with a letter")
        for end in (j.parent, j.child):
            if end not in names:
                raise ValueError(f"joint {j.name!r}: no link named {end!r} (links: {', '.join(names)})")
        if j.parent == j.child:
            raise ValueError(f"joint {j.name!r} connects {j.parent!r} to itself")
        if j.child in parent_of:
            raise ValueError(f"link {j.child!r} is the child of two joints; each link has one parent")
        parent_of[j.child] = j.parent
        if j.kind in ("revolute", "prismatic") and not (j.lower < j.upper):
            raise ValueError(f"joint {j.name!r}: lower ({j.lower}) must be less than upper ({j.upper})")
        if j.kind == "revolute" and (j.upper - j.lower) > 2 * math.pi + 1e-6:
            raise ValueError(f"joint {j.name!r}: a revolute range over 2π; use gz.continuous for a wheel")
    _check_sensors(asm, names)
    _check_plugins(asm)
    roots = [n for n in names if n not in parent_of]
    if len(roots) != 1:
        raise ValueError(f"the links must form one tree joined by joints; unconnected roots: {roots}")
    for n in names:  # every chain of parents must reach the root without a loop
        seen_chain = set()
        while n in parent_of:
            if n in seen_chain:
                raise ValueError(f"joints form a loop through {n!r}")
            seen_chain.add(n)
            n = parent_of[n]


def root(asm: Assembly) -> str:
    children = {j.child for j in asm.joints}
    return next(lk.name for lk in asm.links if lk.name not in children)


def _check_sensors(asm: Assembly, links: list):
    if len(asm.sensors) > MAX_SENSORS:
        raise ValueError(f"at most {MAX_SENSORS} sensors")
    seen = set()
    for s in asm.sensors:
        if not isinstance(s, Sensor):
            raise TypeError(f"model() sensors must come from gz.camera/depth_camera/rgbd_camera/lidar/imu, "
                            f"got {type(s).__name__}")
        if not NAME_RE.match(s.name):
            raise ValueError(f"sensor name {s.name!r}: lowercase letters, digits and '_', starting with a letter")
        if s.name in seen:
            raise ValueError(f"sensor names must be unique: {s.name!r}")
        seen.add(s.name)
        if s.link not in links:
            raise ValueError(f"sensor {s.name!r}: no link named {s.link!r} (links: {', '.join(links)})")


def _check_plugins(asm: Assembly):
    joints = {j.name: j for j in asm.joints}
    kinds = [p.kind for p in asm.plugins]
    for k in ("diff_drive", "joint_states"):
        if kinds.count(k) > 1:
            raise ValueError(f"at most one gz.{k}()")
    controlled = set()
    for p in asm.plugins:
        if not isinstance(p, Plugin):
            raise TypeError(f"model() plugins must come from gz.diff_drive/joint_states/position_controller/"
                            f"velocity_controller, got {type(p).__name__}")
        if p.kind == "diff_drive":
            for side in ("left", "right"):
                for n in p.params[side]:
                    j = joints.get(n)
                    if j is None:
                        raise ValueError(f"diff_drive {side}: no joint named {n!r} (joints: {', '.join(joints) or 'none'})")
                    if j.kind not in ("continuous", "revolute"):
                        raise ValueError(f"diff_drive {side}: joint {n!r} is {j.kind}; wheels need gz.continuous")
            if set(p.params["left"]) & set(p.params["right"]):
                raise ValueError("diff_drive: a joint can't be both a left and a right wheel")
        elif p.kind in ("position_controller", "velocity_controller"):
            n = p.params["joint"]
            j = joints.get(n)
            if j is None:
                raise ValueError(f"{p.kind}: no joint named {n!r} (joints: {', '.join(joints) or 'none'})")
            if j.kind == "fixed":
                raise ValueError(f"{p.kind}: joint {n!r} is fixed and can't move")
            if n in controlled:
                raise ValueError(f"joint {n!r} has two controllers")
            controlled.add(n)
