"""C1 format spike (plan §7): export test models with the library stack the
mesh worker will use (build123d, xatlas, trimesh; D-9), so they can be
checked in Gazebo and gzweb before anything is built on GLB.

    python export.py OUT_DIR

Writes OUT_DIR/models/<name>/ for each test model: model.config,
model.sdf, meshes/<name>.glb (one PBR material: base color and packed ORM
atlases), meshes/<name>_collision_0.glb (convex hull, no material) and
mesh_report.json, plus OUT_DIR/previews/<name>.png when a GL context is
available (pyrender; PYOPENGL_PLATFORM=egl or osmesa).

Test models:
- spike_axes: an orientation marker. A red arrow points along +X, a green
  bar along +Y and a blue post up +Z, on a grey 0.4 m plate. If Gazebo or
  gzweb shows the red arrow anywhere but +X, the axis convention is wrong.
- spike_cone: a 0.6 m traffic cone on a 0.36 m rubber base. Its white band
  (z 0.33-0.45 m) and noisy wear are computed per texel from 3D position
  and only show up if the UV atlas and its orientation are right. Its
  collision is the convex hull, so it should come to rest on its base.

Axis convention: vertices are written in Gazebo's frame (Z up, +X forward)
with no root-node rotation. Both Gazebo's assimp loader and gzweb's
three.js GLTFLoader use glTF coordinates as-is, so this is what makes the
model upright in both, even though glTF's own convention is Y up.
"""

import json
import math
import os
import sys

import numpy as np
import trimesh
import xatlas
from build123d import Box, Cone, Cylinder, Pos, Rot
from PIL import Image

OUT = sys.argv[1]
TEXTURE_SIZE = 1024
TOLERANCE, ANGULAR_TOLERANCE = 0.0005, 0.2  # m, rad


# -- materials: color(positions, normals) -> RGB in [0,1], plus roughness and
# metalness. The worker's library will add CC0 texture sets projected the
# same way (triplanar on position and normal).

def flat(rgb, roughness=0.6, metalness=0.0):
    def color(p, n):
        return np.broadcast_to(np.array(rgb, float), p.shape).copy()
    return {"color": color, "roughness": roughness, "metalness": metalness}


def value_noise(p, scale):
    """Cheap smooth-ish 3D noise in [0,1] from summed sines."""
    q = p * scale
    v = np.sin(q[:, 0] * 1.7 + np.sin(q[:, 1] * 2.3)) + np.sin(q[:, 1] * 1.3 + np.sin(q[:, 2] * 2.9)) \
        + np.sin(q[:, 2] * 1.9 + np.sin(q[:, 0] * 3.1))
    return (v / 3 + 1) / 2


def cone_bands():
    orange, white = np.array([0.95, 0.35, 0.03]), np.array([0.95, 0.95, 0.92])

    def color(p, n):
        band = ((p[:, 2] > 0.33) & (p[:, 2] < 0.45))[:, None]
        base = np.where(band, white, orange)
        wear = 0.8 + 0.2 * value_noise(p, 60)[:, None]
        return base * wear
    return {"color": color, "roughness": 0.5, "metalness": 0.0}


# -- geometry

def tessellate(shape):
    verts, tris = shape.tessellate(TOLERANCE, ANGULAR_TOLERANCE)
    v = np.array([(p.X, p.Y, p.Z) for p in verts], float)
    f = np.array(tris, np.int64)
    return trimesh.Trimesh(v, f, process=False)


def combine(parts):
    """Concatenate (mesh, material) parts; returns the mesh, a per-face
    material index and the materials."""
    meshes, ids, mats = [], [], []
    for i, (mesh, mat) in enumerate(parts):
        meshes.append(mesh)
        ids.append(np.full(len(mesh.faces), i))
        mats.append(mat)
    return trimesh.util.concatenate(meshes), np.concatenate(ids), mats


def bottom_center(mesh):
    """Plan §5.4: origin at the bottom center of the bounding box."""
    lo, hi = mesh.bounds
    mesh.apply_translation(-np.array([(lo[0] + hi[0]) / 2, (lo[1] + hi[1]) / 2, lo[2]]))
    return mesh.extents


# -- atlas

def rasterize(mesh, face_mat, mats, size):
    """UV-unwrap with xatlas and bake every material into one base color
    and one packed ORM (occlusion R, roughness G, metallic B) atlas."""
    vmap, faces, uvs = xatlas.parametrize(mesh.vertices, mesh.faces)
    pos = mesh.vertices[vmap]
    nrm = mesh.vertex_normals[vmap]
    color = np.zeros((size, size, 3))
    orm = np.zeros((size, size, 3))
    filled = np.zeros((size, size), bool)
    # Pixel centers; image row 0 is v = 1 (trimesh flips v on glTF export).
    px = np.stack([uvs[:, 0] * size - 0.5, (1 - uvs[:, 1]) * size - 0.5], 1)
    for fi, tri in enumerate(faces):
        a, b, c = px[tri]
        x0, y0 = np.floor(np.minimum(np.minimum(a, b), c)).astype(int)
        x1, y1 = np.ceil(np.maximum(np.maximum(a, b), c)).astype(int)
        x0, y0, x1, y1 = max(x0, 0), max(y0, 0), min(x1, size - 1), min(y1, size - 1)
        if x1 < x0 or y1 < y0:
            continue
        gx, gy = np.meshgrid(np.arange(x0, x1 + 1), np.arange(y0, y1 + 1))
        g = np.stack([gx.ravel(), gy.ravel()], 1).astype(float)
        d = (b[1] - c[1]) * (a[0] - c[0]) + (c[0] - b[0]) * (a[1] - c[1])
        if abs(d) < 1e-12:
            continue
        w0 = ((b[1] - c[1]) * (g[:, 0] - c[0]) + (c[0] - b[0]) * (g[:, 1] - c[1])) / d
        w1 = ((c[1] - a[1]) * (g[:, 0] - c[0]) + (a[0] - c[0]) * (g[:, 1] - c[1])) / d
        w = np.stack([w0, w1, 1 - w0 - w1], 1)
        inside = (w >= -0.02).all(1)  # a little conservative, to cover edges
        if not inside.any():
            continue
        w, g = w[inside], g[inside].astype(int)
        p = w @ pos[tri]
        n = w @ nrm[tri]
        m = mats[face_mat[fi]]
        color[g[:, 1], g[:, 0]] = m["color"](p, n)
        orm[g[:, 1], g[:, 0]] = (1.0, m["roughness"], m["metalness"])
        filled[g[:, 1], g[:, 0]] = True
    dilate([color, orm], filled, 8)
    to_img = lambda a: Image.fromarray((np.clip(a, 0, 1) * 255).astype(np.uint8))
    return vmap, faces, uvs, to_img(color), to_img(orm), filled.mean()


def dilate(images, filled, steps):
    """Grow chart edges outwards so bilinear and mip sampling don't pull in
    the empty background at seams."""
    for _ in range(steps):
        acc = [np.zeros_like(im) for im in images]
        cnt = np.zeros(filled.shape)
        for dy, dx in ((-1, 0), (1, 0), (0, -1), (0, 1)):
            sf = np.roll(filled, (dy, dx), (0, 1))
            for a, im in zip(acc, images):
                a += np.roll(im, (dy, dx), (0, 1)) * sf[..., None]
            cnt += sf
        grow = ~filled & (cnt > 0)
        for a, im in zip(acc, images):
            im[grow] = a[grow] / cnt[grow][:, None]
        filled |= grow


# -- output

def write_model(name, parts, mass):
    mesh, face_mat, mats = combine(parts)
    size = bottom_center(mesh)
    vmap, faces, uvs, color, orm, utilization = rasterize(mesh, face_mat, mats, TEXTURE_SIZE)
    visual = trimesh.Trimesh(
        mesh.vertices[vmap], faces, process=False,
        visual=trimesh.visual.TextureVisuals(
            uv=uvs,
            material=trimesh.visual.material.PBRMaterial(
                name=name, baseColorTexture=color, metallicRoughnessTexture=orm,
                metallicFactor=1.0, roughnessFactor=1.0, doubleSided=False)))

    # Collision and inertia from the convex hull (the worker will use the
    # exact B-rep, primitive fits or CoACD).
    hull = trimesh.convex.convex_hull(mesh.vertices)
    hull.density = mass / hull.volume
    com, inertia = hull.center_mass, hull.moment_inertia

    root = os.path.join(OUT, "models", name)
    os.makedirs(os.path.join(root, "meshes"), exist_ok=True)
    visual.export(os.path.join(root, "meshes", name + ".glb"))
    hull.export(os.path.join(root, "meshes", name + "_collision_0.glb"))
    color.save(os.path.join(root, "albedo_debug.png"))

    g = lambda x: f"{x:.6g}"
    with open(os.path.join(root, "model.sdf"), "w") as f:
        f.write(f"""<?xml version="1.0"?>
<sdf version="1.9">
  <model name="{name}">
    <link name="link">
      <inertial>
        <pose>{g(com[0])} {g(com[1])} {g(com[2])} 0 0 0</pose>
        <mass>{g(mass)}</mass>
        <inertia>
          <ixx>{g(inertia[0, 0])}</ixx><ixy>{g(inertia[0, 1])}</ixy><ixz>{g(inertia[0, 2])}</ixz>
          <iyy>{g(inertia[1, 1])}</iyy><iyz>{g(inertia[1, 2])}</iyz>
          <izz>{g(inertia[2, 2])}</izz>
        </inertia>
      </inertial>
      <visual name="visual">
        <geometry><mesh><uri>meshes/{name}.glb</uri></mesh></geometry>
      </visual>
      <collision name="collision">
        <geometry><mesh><uri>meshes/{name}_collision_0.glb</uri></mesh></geometry>
      </collision>
    </link>
  </model>
</sdf>
""")
    with open(os.path.join(root, "model.config"), "w") as f:
        f.write(f"""<?xml version="1.0"?>
<model>
  <name>{name}</name>
  <version>1.0</version>
  <sdf version="1.9">model.sdf</sdf>
  <author><name>AI Builder format spike</name></author>
  <description>C1 format spike test model.</description>
</model>
""")
    report = {
        "triangles": int(len(faces)),
        "vertices": int(len(vmap)),
        "bounding_box": [round(float(v), 4) for v in size],
        "collision": {"hulls": 1, "triangles": int(len(hull.faces)), "volume_m3": round(float(hull.volume), 6)},
        "inertial": {"mass": mass, "center_of_mass": [round(float(v), 4) for v in com]},
        "texture_size": TEXTURE_SIZE,
        "atlas_utilization": round(float(utilization), 3),
    }
    with open(os.path.join(root, "mesh_report.json"), "w") as f:
        json.dump(report, f, indent=2)
    print(name, json.dumps(report))
    return visual


def preview(name, mesh, size):
    """Three-quarter view from +X/-Y with pyrender, if a GL context exists."""
    try:
        import pyrender
        scene = pyrender.Scene(ambient_light=[0.35, 0.35, 0.35], bg_color=[0.8, 0.82, 0.85, 1])
        scene.add(pyrender.Mesh.from_trimesh(mesh))
        d = float(max(size)) * 2.4
        eye = np.array([d * 0.9, -d, d * 0.7])
        target = np.array([0, 0, size[2] / 2])
        z = eye - target
        z /= np.linalg.norm(z)
        x = np.cross([0, 0, 1], z)
        x /= np.linalg.norm(x)
        pose = np.eye(4)
        pose[:3, :3] = np.stack([x, np.cross(z, x), z], 1)
        pose[:3, 3] = eye
        scene.add(pyrender.PerspectiveCamera(yfov=0.6), pose=pose)
        scene.add(pyrender.DirectionalLight(intensity=3.0), pose=pose)
        r = pyrender.OffscreenRenderer(512, 512)
        img, _ = r.render(scene)
        r.delete()
        os.makedirs(os.path.join(OUT, "previews"), exist_ok=True)
        Image.fromarray(img).save(os.path.join(OUT, "previews", name + ".png"))
    except Exception as e:  # previews are optional in the spike
        print(f"preview {name}: {e}", file=sys.stderr)


def axes():
    grey, red = flat((0.5, 0.5, 0.5)), flat((0.8, 0.05, 0.05))
    green, blue = flat((0.05, 0.6, 0.1)), flat((0.05, 0.15, 0.8))
    parts = [
        (Pos(0, 0, 0.01) * Box(0.4, 0.4, 0.02), grey),
        (Pos(0.13, 0, 0.04) * Box(0.22, 0.04, 0.04), red),
        (Pos(0.28, 0, 0.04) * Rot(0, 90, 0) * Cone(0.05, 0, 0.08), red),  # apex at +X
        (Pos(0, 0.1, 0.04) * Box(0.03, 0.16, 0.03), green),
        (Pos(0, 0, 0.145) * Cylinder(0.02, 0.25), blue),
    ]
    return [(tessellate(s), m) for s, m in parts]


def cone():
    parts = [
        (Pos(0, 0, 0.015) * Box(0.36, 0.36, 0.03), flat((0.04, 0.04, 0.04), roughness=0.9)),
        (Pos(0, 0, 0.315) * Cone(0.14, 0.025, 0.57), cone_bands()),
    ]
    return [(tessellate(s), m) for s, m in parts]


for name, parts, mass in (("spike_axes", axes(), 1.0), ("spike_cone", cone(), 2.5)):
    visual = write_model(name, parts, mass)
    preview(name, visual, visual.extents)
