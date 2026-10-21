"""Simplifying uploaded meshes, keeping how they look.

fast_simplification works on positions only; its collapse mapping carries
each vertex's UV (or color) to the vertex it collapses into, so textures
stay on (seams may show). A mesh without a texture keeps its color as a
glTF material, since Gazebo ignores vertex colors.
"""

import numpy as np
import trimesh
from trimesh.visual.material import PBRMaterial
from trimesh.visual.texture import TextureVisuals


def main_color(m: trimesh.Trimesh) -> np.ndarray:
    """A mesh's color as RGBA 0-255: its material's, or its vertex or face
    colors' average, else grey."""
    vis = m.visual
    try:
        if vis.kind in ("vertex", "face"):
            return np.asarray(vis.main_color, float)
        mat = vis.material
        for attr in ("baseColorFactor", "diffuse", "main_color"):
            c = getattr(mat, attr, None)
            if c is not None:
                c = np.asarray(c, float)
                if len(c) == 3:
                    c = np.append(c, 1.0 if c.max() <= 1.0 else 255)
                return c * 255 if c.max() <= 1.0 else c
    except Exception:  # noqa: BLE001 - fall back to grey
        pass
    return np.array([180, 180, 180, 255], float)


def flat_material(color) -> PBRMaterial:
    """A plain, non-metal glTF material of an RGBA 0-255 color."""
    c = np.asarray(color, float)
    return PBRMaterial(baseColorFactor=np.clip(c, 0, 255).astype(np.uint8), metallicFactor=0.0,
                       roughnessFactor=0.7)


def simplify(m: trimesh.Trimesh, ratio: float) -> trimesh.Trimesh:
    """m with about ratio of its faces removed, its look kept."""
    import fast_simplification

    _, _, collapses = fast_simplification.simplify(m.vertices, m.faces, ratio, return_collapses=True)
    v, f, mapping = fast_simplification.replay_simplification(m.vertices, m.faces, collapses)
    out = trimesh.Trimesh(v, f, process=False)
    vis = m.visual
    if isinstance(vis, TextureVisuals) and vis.uv is not None and len(vis.uv) == len(m.vertices):
        uv = np.zeros((len(v), 2))
        uv[mapping] = vis.uv  # mapping: original vertex -> its vertex in the result
        out.visual = TextureVisuals(uv=uv, material=vis.material)
    elif isinstance(vis, TextureVisuals):
        out.visual = TextureVisuals(material=vis.material)
    else:
        out.visual = TextureVisuals(material=flat_material(main_color(m)))
    return out
