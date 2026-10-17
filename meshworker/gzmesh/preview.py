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


def sheet(visual: trimesh.Trimesh, tile: int = 384) -> Image.Image:
    tex = np.asarray(visual.visual.material.baseColorTexture.convert("RGB"), float) / 255
    uv = visual.visual.uv
    out = Image.new("RGB", (tile * 2, tile * 2))
    for i, (label, d, up) in enumerate(VIEWS):
        img = render(visual.vertices, visual.faces, visual.vertex_normals, uv, tex, d, up, tile)
        im = Image.fromarray((img * 255).astype(np.uint8))
        ImageDraw.Draw(im).text((6, 4), label, fill=(40, 40, 40))
        out.paste(im, ((i % 2) * tile, (i // 2) * tile))
    return out


def render(v, f, n, uv, tex, d, up, size):
    d = d / np.linalg.norm(d)
    right = np.cross(up, d)
    right /= np.linalg.norm(right)
    cam_up = np.cross(d, right)
    center = (v.min(0) + v.max(0)) / 2
    q = v - center
    x, y, z = q @ right, q @ cam_up, q @ d  # z: toward the camera
    half = max(np.abs(x).max(), np.abs(y).max()) * 1.12 or 1.0
    sx = (x / half * 0.5 + 0.5) * (size - 1)
    sy = (0.5 - y / half * 0.5) * (size - 1)
    # Light each view from above and to the right of its camera, so every
    # view, including the back one, shows its faces lit.
    light = d * 0.7 + cam_up * 0.6 + right * 0.4
    light /= np.linalg.norm(light)
    shade = AMBIENT + (1 - AMBIENT) * np.clip(n @ light, 0, 1)

    img = np.tile(BACKGROUND, (size, size, 1))
    depth = np.full((size, size), -np.inf)
    th, tw = tex.shape[:2]
    # Back-face culling: keep triangles facing the camera.
    fn = np.cross(v[f[:, 1]] - v[f[:, 0]], v[f[:, 2]] - v[f[:, 0]])
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
    return np.clip(img, 0, 1)
