"""Measure a Fuel model: its bounding box in the model frame, as Gazebo shows it.

    python -m gzmesh.measure MODEL_DIR OUT_DIR

MODEL_DIR is an unpacked Fuel model (model.config and its SDF files, maybe
one folder down). OUT_DIR gets report.json:

    {"ok": true, "version": ..., "sdf": "model.sdf",
     "bbox": {"min": [x, y, z], "max": [x, y, z]}, "size": [x, y, z],
     "visuals": 3, "measured": 3, "warnings": [...]}

The box covers every visual (collisions when a model has no visuals) with
link, visual and nested-model poses and mesh scales applied. Meshes load
the way Gazebo loads them: Collada with its <unit> scale and no up-axis
rotation; OBJ, STL, PLY and glTF as stored. Visuals that can't be measured
(a mesh from another model, a missing file) are left out and named in the
warnings. Exit status 0 means a box was measured; 2 means nothing could
be; 1 is an internal error.

Model files are untrusted: this runs in the same per-job sandbox as
gzmesh.run.
"""

import json
import math
import os
import re
import sys
import traceback
import xml.etree.ElementTree as ET

import numpy as np

VERSION = "1"
MESH_EXTS = {".dae", ".obj", ".stl", ".ply", ".glb", ".gltf"}
MAX_WARNINGS = 20


class MeasureError(Exception):
    """The model can't be measured at all."""


# -- poses ---------------------------------------------------------------------

def pose_matrix(el, warnings, where):
    """The 4x4 transform of an element's <pose> child (identity without one)."""
    m = np.eye(4)
    p = el.find("pose") if el is not None else None
    if p is None or not (p.text or "").strip():
        return m
    try:
        v = [float(x) for x in p.text.split()]
    except ValueError:
        warn(warnings, f"{where}: unreadable pose {p.text.strip()!r}; using zero")
        return m
    if p.get("relative_to"):
        warn(warnings, f"{where}: pose relative_to={p.get('relative_to')!r} is treated as relative to its parent")
    if p.get("rotation_format") == "quat_xyzw" and len(v) == 7:
        m[:3, :3] = quat_matrix(*v[3:])
    elif len(v) == 6:
        r, pi, y = v[3:]
        if p.get("degrees") == "true":
            r, pi, y = (math.radians(a) for a in (r, pi, y))
        m[:3, :3] = rpy_matrix(r, pi, y)
    else:
        warn(warnings, f"{where}: pose needs 6 numbers, got {len(v)}; using zero")
        return m
    m[:3, 3] = v[:3]
    return m


def rpy_matrix(r, p, y):
    cr, sr, cp, sp, cy, sy = math.cos(r), math.sin(r), math.cos(p), math.sin(p), math.cos(y), math.sin(y)
    return np.array([[cy * cp, cy * sp * sr - sy * cr, cy * sp * cr + sy * sr],
                     [sy * cp, sy * sp * sr + cy * cr, sy * sp * cr - cy * sr],
                     [-sp, cp * sr, cp * cr]])


def quat_matrix(x, y, z, w):
    n = math.sqrt(x * x + y * y + z * z + w * w) or 1.0
    x, y, z, w = x / n, y / n, z / n, w / n
    return np.array([[1 - 2 * (y * y + z * z), 2 * (x * y - z * w), 2 * (x * z + y * w)],
                     [2 * (x * y + z * w), 1 - 2 * (x * x + z * z), 2 * (y * z - x * w)],
                     [2 * (x * z - y * w), 2 * (y * z + x * w), 1 - 2 * (x * x + y * y)]])


def warn(warnings, msg):
    if len(warnings) < MAX_WARNINGS:
        warnings.append(msg)


def floats(text, n, default):
    try:
        v = [float(x) for x in (text or "").split()]
    except ValueError:
        return default
    return v if len(v) == n else default


# -- files ---------------------------------------------------------------------

def model_root(top):
    """The folder holding model.config (or model.sdf), at most two levels down."""
    for depth in range(3):
        for dirpath, dirnames, filenames in os.walk(top):
            if dirpath[len(top):].count(os.sep) != depth:
                continue
            if "model.config" in filenames or "model.sdf" in filenames:
                return dirpath
    raise MeasureError("no model.config or model.sdf in the model")


def sdf_file(root):
    """The SDF file Gazebo loads: the newest version model.config lists."""
    best, best_v = None, (-1,)
    cfg = os.path.join(root, "model.config")
    if os.path.isfile(cfg):
        try:
            for el in ET.parse(cfg).getroot().iter("sdf"):
                name = (el.text or "").strip()
                v = tuple(int(x) for x in re.findall(r"\d+", el.get("version", "0")))
                if name and os.path.isfile(os.path.join(root, name)) and v > best_v:
                    best, best_v = name, v
        except ET.ParseError:
            pass
    if best:
        return best
    if os.path.isfile(os.path.join(root, "model.sdf")):
        return "model.sdf"
    sdfs = sorted(f for f in os.listdir(root) if f.endswith(".sdf"))
    if sdfs:
        return sdfs[0]
    raise MeasureError("model.config names no SDF file that exists")


def resolve_mesh(root, uri):
    """A mesh URI's file inside this model, or None.

    Fuel models refer to their own files as model://<name>/<path>, as a
    Fuel URL ending in .../files/<path>, or relative to the model folder.
    """
    uri = uri.strip()
    if "/files/" in uri and uri.startswith(("http://", "https://")):
        rel = uri.split("/files/", 1)[1]
    elif uri.startswith("model://"):
        parts = uri[len("model://"):].split("/", 1)
        rel = parts[1] if len(parts) == 2 else ""
    elif uri.startswith("file://"):
        rel = uri[len("file://"):]
    elif "://" in uri:
        return None
    else:
        rel = uri
    rel = rel.lstrip("/")
    if not rel or ".." in rel.split("/"):
        return None
    path = os.path.join(root, rel)
    if os.path.isfile(path):
        return path
    # Fuel's file names are case-sensitive but uploads aren't always.
    want = rel.lower()
    for dirpath, _, filenames in os.walk(root):
        for f in filenames:
            full = os.path.join(dirpath, f)
            if os.path.relpath(full, root).replace(os.sep, "/").lower() == want:
                return full
    return None


# -- geometry ------------------------------------------------------------------

def unit_sphere():
    # Directions sampling a sphere finely enough that the box of a rotated,
    # scaled one is within about 1% of exact.
    pts = []
    for i in range(13):
        theta = math.pi * i / 12
        for j in range(24):
            phi = 2 * math.pi * j / 24
            pts.append((math.sin(theta) * math.cos(phi), math.sin(theta) * math.sin(phi), math.cos(theta)))
    return np.array(pts)


SPHERE = unit_sphere()
CIRCLE = np.array([(math.cos(a), math.sin(a)) for a in np.linspace(0, 2 * math.pi, 48, endpoint=False)])


def shape_points(geom, root, mesh_cache, warnings, where):
    """Points whose box is the geometry's box, in the geometry's frame."""
    for child in geom:
        tag = child.tag
        if tag == "box":
            sx, sy, sz = floats(child.findtext("size"), 3, [1, 1, 1])
            return np.array([(x, y, z) for x in (-sx / 2, sx / 2) for y in (-sy / 2, sy / 2)
                             for z in (-sz / 2, sz / 2)])
        if tag == "cylinder":
            r = floats(child.findtext("radius"), 1, [0.5])[0]
            h = floats(child.findtext("length"), 1, [1])[0]
            ring = CIRCLE * r
            return np.vstack([np.c_[ring, np.full(len(ring), z)] for z in (-h / 2, h / 2)])
        if tag == "sphere":
            return SPHERE * floats(child.findtext("radius"), 1, [0.5])[0]
        if tag == "capsule":
            r = floats(child.findtext("radius"), 1, [0.5])[0]
            h = floats(child.findtext("length"), 1, [1])[0]
            return np.vstack([SPHERE * r + (0, 0, -h / 2), SPHERE * r + (0, 0, h / 2)])
        if tag == "ellipsoid":
            return SPHERE * floats(child.findtext("radii"), 3, [1, 1, 1])
        if tag == "plane":
            sx, sy = floats(child.findtext("size"), 2, [1, 1])
            n = np.array(floats(child.findtext("normal"), 3, [0, 0, 1]), dtype=float)
            corners = np.array([(x, y, 0) for x in (-sx / 2, sx / 2) for y in (-sy / 2, sy / 2)])
            return corners @ align_z(n).T
        if tag == "mesh":
            uri = child.findtext("uri") or ""
            path = resolve_mesh(root, uri)
            if path is None:
                warn(warnings, f"{where}: mesh {uri.strip()!r} isn't in this model; left out")
                return None
            ext = os.path.splitext(path)[1].lower()
            if ext not in MESH_EXTS:
                warn(warnings, f"{where}: mesh format {ext} isn't supported; left out")
                return None
            if path not in mesh_cache:
                try:
                    mesh_cache[path] = mesh_vertices(path)
                except Exception as e:  # noqa: BLE001 - any loader failure is a warning
                    warn(warnings, f"{where}: couldn't read {os.path.relpath(path, root)}: {e}")
                    mesh_cache[path] = None
            pts = mesh_cache[path]
            if pts is None or not len(pts):
                return None
            if child.find("submesh") is not None:
                warn(warnings, f"{where}: uses one submesh of {os.path.relpath(path, root)}; measured the whole mesh")
            return pts * np.array(floats(child.findtext("scale"), 3, [1, 1, 1]))
        if tag in ("heightmap", "polyline", "image"):
            warn(warnings, f"{where}: {tag} geometry isn't measured; left out")
            return None
    return None


def align_z(n):
    """A rotation taking +Z to the unit vector along n."""
    n = n / (np.linalg.norm(n) or 1.0)
    z = np.array([0.0, 0.0, 1.0])
    v, c = np.cross(z, n), float(np.dot(z, n))
    if np.linalg.norm(v) < 1e-9:
        return np.eye(3) if c > 0 else np.diag([1.0, -1.0, -1.0])
    k = np.array([[0, -v[2], v[1]], [v[2], 0, -v[0]], [-v[1], v[0], 0]])
    return np.eye(3) + k + k @ k / (1 + c)


def mesh_vertices(path):
    """A mesh file's vertices in meters, in the frame Gazebo uses for it."""
    if path.lower().endswith(".dae"):
        return collada_vertices(path)
    import trimesh

    scene = trimesh.load(path, force="scene", process=False)
    pts = []
    for node in scene.graph.nodes_geometry:
        transform, name = scene.graph[node]
        geom = scene.geometry[name]
        v = np.asarray(getattr(geom, "vertices", []), dtype=float)
        if len(v):
            pts.append(v @ transform[:3, :3].T + transform[:3, 3])
    return np.vstack(pts) if pts else np.zeros((0, 3))


def collada_vertices(path):
    """Vertices of every geometry instanced in the scene, node transforms and
    the <unit meter> scale applied. Gazebo's Collada loader ignores
    <up_axis>, so this does too."""
    import collada
    from collada.common import DaeBrokenRefError, DaeIncompleteError, DaeMalformedError, DaeUnsupportedError

    dae = collada.Collada(path, ignore=[DaeUnsupportedError, DaeBrokenRefError, DaeIncompleteError,
                                        DaeMalformedError])
    meter = 1.0
    if dae.assetInfo is not None and dae.assetInfo.unitmeter:
        meter = float(dae.assetInfo.unitmeter)
    pts = []
    if dae.scene is not None:
        for bound in dae.scene.objects("geometry"):
            for prim in bound.primitives():
                v = getattr(prim, "vertex", None)
                if v is not None and len(v):
                    pts.append(np.asarray(v, dtype=float).reshape(-1, 3))
    if not pts:
        # No scene: Gazebo would show nothing, but the geometry is still
        # the best guess at the object's size.
        for geom in dae.geometries:
            for prim in geom.primitives:
                v = getattr(prim, "vertex", None)
                if v is not None and len(v):
                    pts.append(np.asarray(v, dtype=float).reshape(-1, 3))
    return np.vstack(pts) * meter if pts else np.zeros((0, 3))


# -- the model tree ------------------------------------------------------------

def measure(top):
    root = model_root(top)
    name = sdf_file(root)
    try:
        doc = ET.parse(os.path.join(root, name)).getroot()
    except ET.ParseError as e:
        raise MeasureError(f"{name}: {e}") from e
    model = doc.find("model") if doc.tag == "sdf" else (doc if doc.tag == "model" else None)
    if model is None:
        raise MeasureError(f"{name} has no <model>")

    warnings, mesh_cache = [], {}
    counts = {"visual": [0, 0], "collision": [0, 0]}
    found = {}
    for kind in ("visual", "collision"):
        lo, hi = np.full(3, np.inf), np.full(3, -np.inf)
        # A nested model's own pose places it in its parent; the root
        # model's pose is where a world puts it, so it isn't applied.
        stack = [(model, np.eye(4), model.get("name", "model"))]
        while stack:
            m, frame, path = stack.pop()
            for nested in m.findall("model"):
                stack.append((nested, frame @ pose_matrix(nested, warnings, path), path + "/" + nested.get("name", "")))
            for inc in m.findall("include"):
                warn(warnings, f"{path}: included model {inc.findtext('uri', '').strip()!r} isn't measured")
            for link in m.findall("link"):
                lf = frame @ pose_matrix(link, warnings, path + "/" + link.get("name", ""))
                for el in link.findall(kind):
                    where = f"{path}/{link.get('name', '')}/{el.get('name', '')}"
                    counts[kind][0] += 1
                    geom = el.find("geometry")
                    if geom is None:
                        continue
                    pts = shape_points(geom, root, mesh_cache, warnings if kind == "visual" else [], where)
                    if pts is None or not len(pts):
                        continue
                    t = lf @ pose_matrix(el, warnings if kind == "visual" else [], where)
                    world = pts @ t[:3, :3].T + t[:3, 3]
                    lo, hi = np.minimum(lo, world.min(axis=0)), np.maximum(hi, world.max(axis=0))
                    counts[kind][1] += 1
        if counts[kind][1]:
            found[kind] = (lo, hi)
        if kind == "visual" and found:
            break

    if not found:
        raise MeasureError("no visual or collision geometry could be measured" +
                           (": " + "; ".join(warnings[:3]) if warnings else ""))
    kind = "visual" if "visual" in found else "collision"
    lo, hi = found[kind]
    if kind == "collision":
        warn(warnings, "no visual could be measured; the box is from collisions")
    r = lambda v: [round(float(x), 4) + 0.0 for x in v]  # noqa: E731
    return {
        "ok": True, "version": VERSION, "sdf": name, "from": kind,
        "bbox": {"min": r(lo), "max": r(hi)}, "size": r(hi - lo),
        "visuals": counts["visual"][0], "measured": counts[kind][1], "warnings": warnings,
    }


def main(argv):
    if len(argv) != 3:
        print(__doc__.split("\n\n")[1], file=sys.stderr)
        return 1
    model_dir, out = argv[1], argv[2]
    os.makedirs(out, exist_ok=True)
    code = 0
    try:
        report = measure(model_dir)
    except MeasureError as e:
        report, code = {"ok": False, "version": VERSION, "error": str(e)}, 2
    except Exception:  # noqa: BLE001 - report the failure, then exit 1
        report, code = {"ok": False, "version": VERSION, "error": "internal error",
                        "detail": traceback.format_exc(limit=5)}, 1
    with open(os.path.join(out, "report.json"), "w") as f:
        json.dump(report, f, indent=1)
    return code


if __name__ == "__main__":
    sys.exit(main(sys.argv))
