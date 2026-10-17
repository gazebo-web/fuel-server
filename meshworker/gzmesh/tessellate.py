"""B-rep to triangles within a triangle budget (plan §5.4).

OCCT tessellates each B-rep face separately, so vertices are shared within
a face (smooth shading across a cylinder's side) and split between faces
(hard edges at a box's corners). The tolerance is loosened until the model
fits the budget; only if that isn't enough are parts decimated, after
which hard edges are recovered by angle.
"""

import math

import fast_simplification
import numpy as np
import trimesh

from .parts import Part

MAX_ATTEMPTS = 8


def tessellate_shape(shape, tolerance: float, angular_tolerance: float) -> trimesh.Trimesh:
    verts, tris = shape.tessellate(tolerance, angular_tolerance)
    v = np.array([(p.X, p.Y, p.Z) for p in verts], float).reshape(-1, 3)
    f = np.array(tris, np.int64).reshape(-1, 3)
    return trimesh.Trimesh(v, f, process=False)


def extent(parts: list[Part]) -> float:
    lo = np.full(3, np.inf)
    hi = np.full(3, -np.inf)
    for p in parts:
        bb = p.shape.bounding_box()
        lo = np.minimum(lo, [bb.min.X, bb.min.Y, bb.min.Z])
        hi = np.maximum(hi, [bb.max.X, bb.max.Y, bb.max.Z])
    return float((hi - lo).max())


def tessellate_parts(parts: list[Part], budget: int) -> tuple[list[trimesh.Trimesh], dict]:
    """Meshes for each part, with a combined triangle count <= budget, and
    a report of how that was reached."""
    size = extent(parts)
    tol, ang = max(size * 5e-4, 1e-5), 0.2
    for attempt in range(MAX_ATTEMPTS):
        meshes = [tessellate_shape(p.shape, tol, ang) for p in parts]
        tris = sum(len(m.faces) for m in meshes)
        if tris <= budget:
            return meshes, {"tolerance_m": tol, "angular_tolerance": ang, "triangles": tris, "decimated": False}
        tol, ang = tol * 2, min(ang * 1.5, 0.8)

    # Still over budget: decimate each part in proportion.
    ratio = budget / tris
    out = []
    for m in meshes:
        merged = m.copy()
        merged.merge_vertices()
        target = max(int(len(merged.faces) * ratio), 4)
        v, f = fast_simplification.simplify(merged.vertices, merged.faces, 1 - target / len(merged.faces))
        dm = trimesh.Trimesh(v, f, process=False)
        out.append(trimesh.graph.smooth_shade(dm, angle=math.radians(35)))
    return out, {"tolerance_m": tol, "angular_tolerance": ang, "triangles": sum(len(m.faces) for m in out),
                 "decimated": True, "before_decimation": tris}


def combine(meshes: list[trimesh.Trimesh]) -> tuple[trimesh.Trimesh, np.ndarray]:
    """One mesh plus the index of the part each face came from."""
    ids = np.concatenate([np.full(len(m.faces), i) for i, m in enumerate(meshes)])
    return trimesh.util.concatenate(meshes), ids


def bottom_center_offset(mesh: trimesh.Trimesh) -> np.ndarray:
    """The translation that puts the bounding box's bottom center at the
    origin (plan §5.4)."""
    lo, hi = mesh.bounds
    return -np.array([(lo[0] + hi[0]) / 2, (lo[1] + hi[1]) / 2, lo[2]])
