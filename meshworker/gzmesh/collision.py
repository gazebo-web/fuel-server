"""Collision geometry (plan §5.4).

auto: the best-fitting primitive when the solid fills at least
PRIMITIVE_FILL of it; else one convex hull when the solid fills at least
HULL_FILL of that; else a convex decomposition (CoACD). convex forces the
decomposition. primitive fits a primitive to each solid piece the code
made (each leg and rail of a table), not one around the whole link, and
decomposes a piece that's hollow (a tray with a rim) rather than filling
it. Every hull has at most MAX_HULL_VERTICES vertices, and a
decomposition at most MAX_HULLS hulls.
"""

import math
from dataclasses import dataclass, field
from typing import Optional

import coacd
import numpy as np
import trimesh
from trimesh.transformations import euler_from_matrix

PRIMITIVE_FILL = 0.9
HULL_FILL = 0.85
# primitive mode: a piece filling less of its best primitive is hollow, and
# is decomposed; more pieces than MAX_PIECES are fitted as one solid.
HOLLOW_FILL = 0.5
MAX_PIECES = 64
MAX_HULLS = 16
MAX_HULL_VERTICES = 255
MODES = ("auto", "primitive", "convex")

coacd.set_log_level("error")


@dataclass
class Shape:
    """One collision element. kind is box, cylinder, sphere or mesh; pose is
    x y z roll pitch yaw in the model frame; mesh is set for kind mesh."""
    kind: str
    pose: list = field(default_factory=lambda: [0.0] * 6)
    size: Optional[list] = None      # box
    radius: Optional[float] = None   # cylinder, sphere
    length: Optional[float] = None   # cylinder
    mesh: Optional[trimesh.Trimesh] = None

    def volume(self) -> float:
        if self.kind == "box":
            return float(np.prod(self.size))
        if self.kind == "cylinder":
            return math.pi * self.radius ** 2 * self.length
        if self.kind == "sphere":
            return 4 / 3 * math.pi * self.radius ** 3
        return float(self.mesh.volume)


def _pose(T: np.ndarray) -> list:
    return [float(x) for x in (*T[:3, 3], *euler_from_matrix(T, "sxyz"))]


def primitives(mesh: trimesh.Trimesh) -> list[Shape]:
    """Enclosing box (axis-aligned or oriented), cylinder and sphere."""
    out = []
    lo, hi = mesh.bounds
    aabb = Shape("box", [*((lo + hi) / 2), 0, 0, 0], size=list(hi - lo))
    obb = mesh.bounding_box_oriented
    obb_shape = Shape("box", _pose(obb.primitive.transform), size=list(obb.primitive.extents))
    # Prefer the axis-aligned box unless the oriented one is clearly tighter.
    out.append(obb_shape if obb_shape.volume() < aabb.volume() * 0.97 else aabb)
    try:
        cyl = trimesh.bounds.minimum_cylinder(mesh)
        out.append(Shape("cylinder", _pose(cyl["transform"]), radius=float(cyl["radius"]),
                         length=float(cyl["height"])))
    except Exception:
        pass
    sph = mesh.bounding_sphere.primitive
    out.append(Shape("sphere", [*sph.center, 0, 0, 0], radius=float(sph.radius)))
    for s in out:
        s.size = [float(x) for x in s.size] if s.size else None
        s.pose = [float(x) for x in s.pose]
    return out


def limit_vertices(hull: trimesh.Trimesh) -> trimesh.Trimesh:
    """Re-hull a sample of the vertices until there are at most
    MAX_HULL_VERTICES."""
    v = hull.vertices
    while len(v) > MAX_HULL_VERTICES:
        rng = np.random.default_rng(0)
        v = v[rng.choice(len(v), MAX_HULL_VERTICES - 8, replace=False)]
        v = trimesh.convex.convex_hull(v).vertices
    return trimesh.convex.convex_hull(v) if len(v) != len(hull.vertices) else hull


def decompose(mesh: trimesh.Trimesh) -> list[Shape]:
    parts = coacd.run_coacd(coacd.Mesh(mesh.vertices, mesh.faces), threshold=0.05,
                            max_convex_hull=MAX_HULLS, decimate=True, max_ch_vertex=MAX_HULL_VERTICES,
                            real_metric=True, seed=0)
    hulls = [limit_vertices(trimesh.convex.convex_hull(np.asarray(v))) for v, _ in parts]
    return [Shape("mesh", mesh=h) for h in hulls]


def per_piece(pieces: list[tuple[trimesh.Trimesh, float]]) -> tuple[list[Shape], dict]:
    """primitive mode: the best primitive for each solid piece (its mesh and
    exact volume), or a decomposition of a hollow one."""
    shapes, hollow = [], 0
    for mesh, volume in pieces:
        best = max(primitives(mesh), key=lambda s: volume / s.volume())
        if volume / best.volume() >= HOLLOW_FILL:
            shapes.append(best)
        else:
            shapes += decompose(mesh)
            hollow += 1
    report = {"mode": "primitive", "pieces": len(pieces)}
    if hollow:
        report["hollow_pieces_decomposed"] = hollow
    return shapes, report


def build(solid: trimesh.Trimesh, solid_volume: float, mode: str = "auto") -> tuple[list[Shape], dict]:
    """solid: a watertight mesh of the whole model; solid_volume: its exact
    volume (from the B-rep)."""
    if mode not in MODES:
        raise ValueError(f"collision {mode!r}: want one of {', '.join(MODES)}")
    report = {"mode": mode}
    if mode in ("auto", "primitive"):
        prims = primitives(solid)
        best = max(prims, key=lambda s: solid_volume / s.volume())
        fill = solid_volume / best.volume()
        report.update(primitive=best.kind, primitive_fill=round(fill, 3))
        if mode == "primitive" or fill >= PRIMITIVE_FILL:
            return [best], report
    hull = limit_vertices(trimesh.convex.convex_hull(solid.vertices))
    hull_fill = solid_volume / hull.volume
    report["hull_fill"] = round(hull_fill, 3)
    if mode == "auto" and hull_fill >= HULL_FILL:
        return [Shape("mesh", mesh=hull)], report
    shapes = decompose(solid)
    report["hulls"] = len(shapes)
    return shapes, report
