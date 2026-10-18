"""The contract between agent-written code and the pipeline (plan §5.2).

The code defines build(), which returns a build123d shape, a Part from
part(), a list of those (one rigid model), or a model() of links connected
by joints (an articulated one: a cabinet with a hinged door, a drawer, a
wheeled cart). Everything else (tessellation, UVs, materials, collision,
inertia, files) is the pipeline's job.

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
class Assembly:
    links: list
    joints: list


def part(shape, name: str = "part", material: Optional[Material] = None) -> Part:
    """A named piece of the model with its material (default: grey plastic)."""
    if not hasattr(shape, "wrapped"):
        raise TypeError(f"part {name!r}: want a build123d shape, got {type(shape).__name__}")
    if material is not None and not isinstance(material, Material):
        raise TypeError(f"part {name!r}: material must come from gzmesh (e.g. gz.plastic('red')), "
                        f"got {type(material).__name__}")
    return Part(shape, name, material or plastic("grey"))


def link(name: str, *parts) -> Link:
    """A rigid body of the model: parts (or shapes, or lists of them) that
    move together."""
    flat = []
    for p in parts:
        flat.extend(p if isinstance(p, (list, tuple)) else [p])
    return Link(name, flat)


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


def model(links: Sequence[Link], joints: Sequence[Joint] = ()) -> Assembly:
    """An articulated model: links connected into a tree by joints."""
    return Assembly(list(links), list(joints))


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
        if any(isinstance(i, (Link, Joint)) for i in items):
            raise TypeError("return gz.model(links=[...], joints=[...]) for links and joints")
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
