"""One UV atlas and one PBR material per model (plan §5.3).

xatlas unwraps the combined mesh; every texel is then colored by its
part's material from its 3D position and normal, so procedural materials
bake without seams. The result is a base color texture (sRGB) and a packed
ORM texture (occlusion R, roughness G, metallic B: glTF's channels, which
Gazebo's loader splits).
"""

import numpy as np
import trimesh
import xatlas
from PIL import Image

from .materials import Material

PADDING_PX = 8


def unwrap(mesh: trimesh.Trimesh, size: int):
    """xatlas charts packed for a size x size texture."""
    atlas = xatlas.Atlas()
    atlas.add_mesh(mesh.vertices, mesh.faces)
    pack = xatlas.PackOptions()
    pack.resolution = size
    pack.padding = PADDING_PX // 2
    pack.bilinear = True
    atlas.generate(pack_options=pack)
    vmap, faces, uvs = atlas[0]
    return vmap, faces, uvs


def bake(mesh: trimesh.Trimesh, face_part: np.ndarray, materials: list[Material], size: int):
    """Returns the textured visual mesh and atlas utilization (0-1)."""
    vmap, faces, uvs = unwrap(mesh, size)
    pos = mesh.vertices[vmap]
    nrm = mesh.vertex_normals[vmap]
    color = np.zeros((size, size, 3))
    orm = np.zeros((size, size, 3))
    filled = np.zeros((size, size), bool)
    # Pixel centers. Image row 0 is v = 1: trimesh flips v on glTF export.
    px = np.stack([uvs[:, 0] * size - 0.5, (1 - uvs[:, 1]) * size - 0.5], 1)

    # Group texels by material so each material's color function runs once
    # on all of its texels.
    texels = {i: ([], [], []) for i in range(len(materials))}
    for fi, tri in enumerate(faces):
        g, w = _cover(px[tri], size)
        if g is None:
            continue
        ps, ns, gs = texels[face_part[fi]]
        ps.append(w @ pos[tri])
        ns.append(w @ nrm[tri])
        gs.append(g)
    for i, (ps, ns, gs) in texels.items():
        if not gs:
            continue
        m = materials[i]
        p, n, g = np.concatenate(ps), np.concatenate(ns), np.concatenate(gs)
        color[g[:, 1], g[:, 0]] = np.clip(m.color(p, n), 0, 1)
        orm[g[:, 1], g[:, 0]] = (1.0, m.roughness, m.metalness)
        filled[g[:, 1], g[:, 0]] = True
    utilization = float(filled.mean())
    dilate([color, orm], filled, PADDING_PX)

    to_img = lambda a: Image.fromarray((np.clip(a, 0, 1) * 255 + 0.5).astype(np.uint8))
    visual = trimesh.Trimesh(
        pos, faces, vertex_normals=nrm, process=False,
        visual=trimesh.visual.TextureVisuals(
            uv=uvs,
            material=trimesh.visual.material.PBRMaterial(
                baseColorTexture=to_img(color), metallicRoughnessTexture=to_img(orm),
                metallicFactor=1.0, roughnessFactor=1.0, doubleSided=False)))
    return visual, utilization


def _cover(tri_px: np.ndarray, size: int):
    """Texels covered by a triangle in pixel space, and their barycentric
    weights. Slightly conservative so chart edges have no gaps. A triangle
    smaller than a texel may contain no texel center; it still gets the
    texel under its centroid, or a chart of only such triangles would stay
    unbaked (black) with nothing nearby for dilation to copy."""
    a, b, c = tri_px
    centroid = np.clip(np.rint((a + b + c) / 3), 0, size - 1).astype(int)[None, :]
    third = np.full((1, 3), 1 / 3)
    x0, y0 = np.floor(np.minimum(np.minimum(a, b), c)).astype(int)
    x1, y1 = np.ceil(np.maximum(np.maximum(a, b), c)).astype(int)
    x0, y0, x1, y1 = max(x0, 0), max(y0, 0), min(x1, size - 1), min(y1, size - 1)
    if x1 < x0 or y1 < y0:
        return centroid, third
    d = (b[1] - c[1]) * (a[0] - c[0]) + (c[0] - b[0]) * (a[1] - c[1])
    if abs(d) < 1e-12:
        return centroid, third
    gx, gy = np.meshgrid(np.arange(x0, x1 + 1), np.arange(y0, y1 + 1))
    g = np.stack([gx.ravel(), gy.ravel()], 1)
    w0 = ((b[1] - c[1]) * (g[:, 0] - c[0]) + (c[0] - b[0]) * (g[:, 1] - c[1])) / d
    w1 = ((c[1] - a[1]) * (g[:, 0] - c[0]) + (a[0] - c[0]) * (g[:, 1] - c[1])) / d
    w = np.stack([w0, w1, 1 - w0 - w1], 1)
    inside = (w >= -0.02).all(1)
    if not inside.any():
        return centroid, third
    w = np.clip(w[inside], 0, None)
    return g[inside], w / w.sum(1, keepdims=True)


def dilate(images: list[np.ndarray], filled: np.ndarray, steps: int):
    """Grow charts outwards so bilinear and mip sampling at seams don't pull
    in the empty background."""
    filled = filled.copy()
    for _ in range(steps):
        acc = [np.zeros_like(im) for im in images]
        cnt = np.zeros(filled.shape)
        for dy, dx in ((-1, 0), (1, 0), (0, -1), (0, 1)):
            sf = np.roll(filled, (dy, dx), (0, 1))
            for a, im in zip(acc, images):
                a += np.roll(im, (dy, dx), (0, 1)) * sf[..., None]
            cnt += sf
        grow = ~filled & (cnt > 0)
        if not grow.any():
            break
        for a, im in zip(acc, images):
            im[grow] = a[grow] / cnt[grow][:, None]
        filled |= grow
