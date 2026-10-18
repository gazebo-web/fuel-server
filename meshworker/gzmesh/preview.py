"""A 2x2 preview sheet for the agent to check its own work (plan §5.4).

A small software rasterizer: orthographic views, a z-buffer, the baked
base color texture and Lambert shading. It needs no GL context (pyrender
failed under numpy 2.5 in the format spike) and approximates, not
reproduces, Gazebo's and gzweb's PBR; aismoke checks those.
"""

import numpy as np
import trimesh
from PIL import Image, ImageDraw

BACKGROUND = np.array([0.86, 0.88, 0.9])
AMBIENT = 0.35

# name, view direction (camera looks along -d, i.e. sits at +d), up. The
# two three-quarter views between them face every horizontal direction, so
# no side of the model is hidden (a desk's drawers on +Y were, with a
# front and a -Y side view).
VIEWS = (
    ("three-quarter (+X -Y)", np.array([0.8, -1.0, 0.7]), np.array([0, 0, 1.0])),
    ("back three-quarter (-X +Y)", np.array([-0.8, 1.0, 0.7]), np.array([0, 0, 1.0])),
    ("front (+X)", np.array([1.0, 0, 0]), np.array([0, 0, 1.0])),
    ("top (+Z)", np.array([0, 0, 1.0]), np.array([1.0, 0, 0])),
)


def sheet(visuals, tile: int = 384, moved=None) -> Image.Image:
    """Four views of one textured mesh or a list of them (the links of an
    articulated model), drawn into a shared depth buffer. moved, for an
    articulated model, is the links with their joints moved: it replaces
    the front view, so doors, drawers and wheels show how they move."""
    if isinstance(visuals, trimesh.Trimesh):
        visuals = [visuals]
    meshes = _prepared(visuals)
    allv = np.concatenate([m[0] for m in meshes])
    views = list(VIEWS)
    if moved is not None:
        views[2] = ("three-quarter, joints moved", VIEWS[0][1], VIEWS[0][2])
        moved_meshes = _prepared(moved)
        moved_v = np.concatenate([m[0] for m in moved_meshes])
    out = Image.new("RGB", (tile * 2, tile * 2))
    for i, (label, d, up) in enumerate(views):
        if moved is not None and i == 2:
            img = render(moved_meshes, moved_v, d, up, tile)
        else:
            img = render(meshes, allv, d, up, tile)
        im = Image.fromarray((img * 255).astype(np.uint8))
        ImageDraw.Draw(im).text((6, 4), label, fill=(40, 40, 40))
        out.paste(im, ((i % 2) * tile, (i // 2) * tile))
    return out


def _prepared(visuals):
    return [(m.vertices, m.faces, m.vertex_normals, m.visual.uv,
             np.asarray(m.visual.material.baseColorTexture.convert("RGB"), float) / 255) for m in visuals]


def render(meshes, allv, d, up, size):
    """meshes: (vertices, faces, normals, uv, texture) tuples; allv frames
    the view."""
    d = d / np.linalg.norm(d)
    right = np.cross(up, d)
    right /= np.linalg.norm(right)
    cam_up = np.cross(d, right)
    center = (allv.min(0) + allv.max(0)) / 2
    q = allv - center
    half = max(np.abs(q @ right).max(), np.abs(q @ cam_up).max()) * 1.12 or 1.0
    # Light each view from above and to the right of its camera, so every
    # view, including the back one, shows its faces lit.
    light = d * 0.7 + cam_up * 0.6 + right * 0.4
    light /= np.linalg.norm(light)
    img = np.tile(BACKGROUND, (size, size, 1))
    depth = np.full((size, size), -np.inf)
    for v, f, n, uv, tex in meshes:
        _draw(img, depth, v - center, f, n, uv, tex, d, right, cam_up, half, light, size)
    return np.clip(img, 0, 1)


def _draw(img, depth, q, f, n, uv, tex, d, right, cam_up, half, light, size):
    x, y, z = q @ right, q @ cam_up, q @ d  # z: toward the camera
    sx = (x / half * 0.5 + 0.5) * (size - 1)
    sy = (0.5 - y / half * 0.5) * (size - 1)
    shade = AMBIENT + (1 - AMBIENT) * np.clip(n @ light, 0, 1)
    th, tw = tex.shape[:2]
    # Back-face culling: keep triangles facing the camera.
    fn = np.cross(q[f[:, 1]] - q[f[:, 0]], q[f[:, 2]] - q[f[:, 0]])
    for tri in f[(fn @ d) > 0]:
        ax, ay = sx[tri], sy[tri]
        x0, x1 = int(max(np.floor(ax.min()), 0)), int(min(np.ceil(ax.max()), size - 1))
        y0, y1 = int(max(np.floor(ay.min()), 0)), int(min(np.ceil(ay.max()), size - 1))
        if x1 < x0 or y1 < y0:
            continue
        den = (ay[1] - ay[2]) * (ax[0] - ax[2]) + (ax[2] - ax[1]) * (ay[0] - ay[2])
        if abs(den) < 1e-12:
            continue
        gx, gy = np.meshgrid(np.arange(x0, x1 + 1), np.arange(y0, y1 + 1))
        gx, gy = gx.ravel(), gy.ravel()
        w0 = ((ay[1] - ay[2]) * (gx - ax[2]) + (ax[2] - ax[1]) * (gy - ay[2])) / den
        w1 = ((ay[2] - ay[0]) * (gx - ax[2]) + (ax[0] - ax[2]) * (gy - ay[2])) / den
        w = np.stack([w0, w1, 1 - w0 - w1], 1)
        inside = (w >= -1e-6).all(1)
        if not inside.any():
            continue
        w, gx, gy = w[inside], gx[inside], gy[inside]
        zz = w @ z[tri]
        closer = zz > depth[gy, gx]
        if not closer.any():
            continue
        w, gx, gy, zz = w[closer], gx[closer], gy[closer], zz[closer]
        t = w @ uv[tri]
        tx = np.clip((t[:, 0] * tw).astype(int), 0, tw - 1)
        ty = np.clip(((1 - t[:, 1]) * th).astype(int), 0, th - 1)
        img[gy, gx] = tex[ty, tx] * (w @ shade[tri])[:, None]
        depth[gy, gx] = zz
