"""Import an uploaded mesh as a model: scaled, Z up, origin at the bottom
center, simplified to a triangle budget, with collision and inertia.

    python -m gzmesh.import_mesh JOB.json OUT_DIR

JOB.json: {"name", "path" (the mesh, relative to the job folder), and
optionally "units" (auto, mm, cm, m, in, ft), "scale" or "height" (meters,
after orienting), "up_axis" (auto, x, y, z), "static", "mass" or
"density", "collision" (auto, primitive, convex), "tri_budget",
"sdf_version", "description", "source_name" (the project file)}.

OUT_DIR gets the same layout as gzmesh.run: models/<name>/ (model.sdf,
model.config, meshes/<name>.glb, collision meshes, source/import.json),
preview.png and report.json. The visual keeps the upload's parts and
materials (textures as PNG or JPEG), so a textured mesh stays textured.
Exit status 0 means the model was built; 2 means the mesh or the job is
the problem, and report.json says why; 1 is an internal error.

Uploads are untrusted input to the mesh parsers: this runs in the same
per-job sandbox as gzmesh.run.
"""

import json
import os
import re
import sys
import time
import traceback

import numpy as np
import trimesh

from PIL import Image

from . import collision, export, inspect, preview
from .inertia import Inertial
from .simplify import flat_material, main_color, simplify

VERSION = "1"
NAME_RE = re.compile(r"^[a-z][a-z0-9_]{0,63}$")
UNITS = {"mm": 0.001, "cm": 0.01, "m": 1.0, "in": 0.0254, "ft": 0.3048}
GUESSED_UNITS = {"millimeters": "mm", "centimeters": "cm", "meters": "m"}
TRI_BUDGET = (500, 200_000)
MAX_TEXTURE = 2048
MERGE_PARTS = 64
DEFAULTS = {"units": "auto", "up_axis": "auto", "static": False, "collision": "auto", "tri_budget": 50_000,
            "sdf_version": "1.9", "description": "", "source_name": ""}
# Without a mass or density: water, about right for furniture and props
# made of wood and plastic, and better than nothing for anything else.
DEFAULT_DENSITY = 1000.0


class ImportError_(Exception):
    """The mesh or the job can't be imported; the message says why."""


def check_job(job: dict) -> dict:
    j = {**DEFAULTS, **job}
    if not isinstance(j.get("name"), str) or not NAME_RE.match(j["name"]):
        raise ImportError_("name must be lowercase letters, digits and underscores, starting with a letter")
    if j["units"] != "auto" and j["units"] not in UNITS:
        raise ImportError_(f"units must be auto or one of {', '.join(UNITS)}")
    if j["up_axis"] not in ("auto", "x", "y", "z"):
        raise ImportError_("up_axis must be auto, x, y or z")
    if j["collision"] not in collision.MODES:
        raise ImportError_(f"collision must be one of {', '.join(collision.MODES)}")
    lo, hi = TRI_BUDGET
    if not isinstance(j["tri_budget"], int) or not lo <= j["tri_budget"] <= hi:
        raise ImportError_(f"tri_budget must be an integer from {lo} to {hi}")
    for k in ("scale", "height", "mass", "density"):
        if j.get(k) is not None and not (isinstance(j[k], (int, float)) and j[k] > 0):
            raise ImportError_(f"{k} must be a positive number")
    if j.get("scale") is not None and j.get("height") is not None:
        raise ImportError_("give scale or height, not both")
    if j.get("mass") is not None and j.get("density") is not None:
        raise ImportError_("give mass or density, not both")
    return j


def gltf_ready(m: trimesh.Trimesh) -> trimesh.Trimesh:
    """m with a PBR material Gazebo renders as the upload looked: a color
    material when it has only vertex or face colors (Gazebo ignores those),
    not metal unless a map says so (glTF's default is fully metal), RGB or
    RGBA textures at most MAX_TEXTURE pixels, and cutouts (alpha) as MASK,
    the only alpha mode Gazebo draws double-sided."""
    vis = m.visual
    if not isinstance(vis, trimesh.visual.TextureVisuals):
        m.visual = trimesh.visual.TextureVisuals(material=flat_material(main_color(m)))
        return m
    mat = vis.material.to_pbr() if hasattr(vis.material, "to_pbr") else vis.material
    if getattr(mat, "metallicRoughnessTexture", None) is None and mat.metallicFactor is None:
        mat.metallicFactor = 0.0
    tex = getattr(mat, "baseColorTexture", None)
    if tex is not None:
        if tex.mode not in ("RGB", "RGBA"):
            tex = tex.convert("RGBA" if "A" in tex.mode or tex.mode == "P" else "RGB")
        if max(tex.size) > MAX_TEXTURE:
            tex = tex.copy()
            tex.thumbnail((MAX_TEXTURE, MAX_TEXTURE), Image.LANCZOS)
        if tex.mode == "RGBA" and tex.getextrema()[3][0] < 255:
            mat.alphaMode, mat.alphaCutoff, mat.doubleSided = "MASK", 0.5, True
        elif tex.mode == "RGBA":
            tex = tex.convert("RGB")
        mat.baseColorTexture = tex
    m.visual = trimesh.visual.TextureVisuals(uv=vis.uv, material=mat)
    return m


def merge_parts(meshes: list) -> list:
    """Many small parts (a scan split into pieces, a model of hundreds of
    bolts) as one mesh per material, so simplifying can reach the budget
    and the GLB isn't hundreds of meshes."""
    groups = {}
    for m in meshes:
        vis = m.visual
        if isinstance(vis, trimesh.visual.TextureVisuals) and vis.uv is not None:
            key = ("texture", id(vis.material))
        else:
            key = ("color", tuple(int(c) for c in main_color(m)))
        groups.setdefault(key, []).append(m)
    out = []
    for (kind, _), ms in groups.items():
        if len(ms) == 1:
            out.append(ms[0])
            continue
        merged = trimesh.util.concatenate(ms)
        if kind == "color":
            merged.visual = trimesh.visual.TextureVisuals(material=flat_material(main_color(ms[0])))
        out.append(merged)
    return out


def mass_properties(solid: trimesh.Trimesh, density: float) -> tuple[float, np.ndarray, np.ndarray, float]:
    """Mass, center of mass and inertia tensor (about it) of a closed mesh."""
    solid = solid.copy()
    solid.density = density
    return float(solid.mass), np.asarray(solid.center_mass), np.asarray(solid.moment_inertia), float(solid.volume)


def build(job: dict, base: str, out: str) -> dict:
    t0 = time.monotonic()
    j = check_job(job)
    name = j["name"]
    path = os.path.normpath(os.path.join(base, j.get("path", "")))
    if not path.startswith(os.path.normpath(base) + os.sep) or not os.path.isfile(path):
        raise ImportError_("the mesh to import is missing")
    try:
        scene = inspect.load_model_mesh(path, os.path.normpath(base))
    except inspect.InspectError as e:
        raise ImportError_(str(e))
    meshes = [g.copy() for g in scene.dump() if isinstance(g, trimesh.Trimesh) and len(g.faces)]
    if not meshes:
        raise ImportError_("the mesh has no triangles")
    warnings = []

    # Orient: Z up.
    up = inspect.up_axis(path) if j["up_axis"] == "auto" else j["up_axis"]
    R = inspect.to_z_up(up)
    for m in meshes:
        m.apply_transform(R)
    lo = np.min([m.bounds[0] for m in meshes], axis=0)
    hi = np.max([m.bounds[1] for m in meshes], axis=0)
    size = hi - lo

    # Scale to meters.
    if j.get("scale") is not None:
        s, how = float(j["scale"]), f"scale {j['scale']}"
    elif j.get("height") is not None:
        if size[2] <= 0:
            raise ImportError_("the mesh is flat, so it has no height to scale to")
        s, how = float(j["height"]) / float(size[2]), f"height {j['height']} m"
    else:
        ext = os.path.splitext(path)[1].lower()
        units = j["units"] if j["units"] != "auto" else GUESSED_UNITS[inspect.units_guess(size, ext)]
        s, how = UNITS[units], f"units {units}" + (" (guessed)" if j["units"] == "auto" else "")
        if j["units"] == "auto":
            warnings.append(f"units guessed as {units}, so the model is {np.max(size) * UNITS[units]:.3g} m across; "
                            "give height (or units) if that's wrong")
    for m in meshes:
        m.apply_scale(s)
    size = size * s
    if np.max(size) > 200 or np.max(size) < 0.005:
        warnings.append(f"the model is {np.max(size):.4g} m across after {how}; check the units or give height")

    # Simplify to the budget.
    before = sum(len(m.faces) for m in meshes)
    if len(meshes) > MERGE_PARTS:
        meshes = merge_parts(meshes)
    if before > j["tri_budget"]:
        ratio = 1 - j["tri_budget"] / before
        meshes = [simplify(m, ratio) if len(m.faces) > 50 else m for m in meshes]
        after = sum(len(m.faces) for m in meshes)
        warnings.append(f"simplified from {before} to {after} triangles to fit tri_budget; textures may show seams")
        if after > j["tri_budget"] * 1.2:
            warnings.append(f"still {after} triangles, over tri_budget {j['tri_budget']}: the mesh has many small parts")

    # Origin at the bottom center.
    lo = np.min([m.bounds[0] for m in meshes], axis=0)
    hi = np.max([m.bounds[1] for m in meshes], axis=0)
    offset = -np.array([(lo[0] + hi[0]) / 2, (lo[1] + hi[1]) / 2, lo[2]])
    for m in meshes:
        m.apply_translation(offset)
    meshes = [gltf_ready(m) for m in meshes]

    # Collision and inertia from the shape: the mesh itself when it is
    # closed, else its convex hull.
    whole = trimesh.util.concatenate([trimesh.Trimesh(m.vertices, m.faces, process=False) for m in meshes])
    whole.merge_vertices()
    if whole.is_watertight and whole.volume < 0:
        whole.invert()  # inside out
    closed = bool(whole.is_watertight and whole.is_winding_consistent and whole.volume > 1e-12)
    try:
        hull = collision.limit_vertices(trimesh.convex.convex_hull(whole.vertices))
        if not hull.volume > 1e-12:
            raise ValueError("no volume")
    except Exception:  # noqa: BLE001 - qhull fails on coplanar points
        raise ImportError_("the mesh is flat (all in one plane), so it has no volume for collision or mass; "
                           "give it some thickness, or build it from a thin box instead")
    solid = whole if closed else hull
    if closed:
        shapes, coll = collision.build(solid, float(solid.volume), j["collision"])
    elif j["collision"] == "primitive":
        shapes, coll = collision.build(hull, float(hull.volume), "primitive")
    else:
        # An open mesh's own surface decomposes (CoACD takes non-manifold
        # input), so a table keeps its legs apart; its hull would be a block.
        shapes, coll = collision.decompose(whole), {"mode": j["collision"], "open_mesh": True}
        coll["hulls"] = len(shapes)
        warnings.append("the mesh isn't closed, so inertia uses its convex hull and collision is a convex "
                        "decomposition of its surface")

    volume = float(solid.volume)
    if j.get("mass") is not None:
        density = float(j["mass"]) / volume
    elif j.get("density") is not None:
        density = float(j["density"])
    else:
        density = DEFAULT_DENSITY
        if not j["static"]:
            warnings.append(f"no mass given: assumed {DEFAULT_DENSITY:g} kg/m³, so "
                            f"{DEFAULT_DENSITY * volume:.4g} kg; pass mass for a real one")
    mass, center, inertia, _ = mass_properties(solid, density)
    inert = Inertial(mass=mass, center=center, inertia=inertia, volume=volume)
    try:
        inert.check()
    except ValueError as e:
        raise ImportError_(f"the mesh's inertia isn't valid ({e}); try collision: convex or a closed mesh")

    visual = trimesh.Scene(meshes)
    link = export.LinkOut("link", visual, inert, shapes, name)
    settings = {k: j[k] for k in ("units", "up_axis", "static", "collision", "tri_budget")}
    for k in ("scale", "height", "mass", "density"):
        if j.get(k) is not None:
            settings[k] = j[k]
    source = json.dumps({"imported_from": j["source_name"], "settings": settings,
                         "applied": {"up_axis": up, "scale": s, "offset_m": [round(float(x), 6) for x in offset]}},
                        indent=2) + "\n"
    note = (f"Imported by the AI Builder from the project file {j['source_name'] or 'upload'}.\n"
            "     Import it again (model_import) to change it; don't edit the meshes.")
    root = os.path.join(out, "models", name)
    files = export.write_model(root, name, links=[link], joints=[], offset=offset, static=j["static"],
                               sdf_version=j["sdf_version"], description=j["description"], source=source,
                               source_path="source/import.json", note=note)
    preview.sheet(inspect.preview_meshes(visual, np.eye(4))).save(os.path.join(out, "preview.png"))

    lo = np.min([m.bounds[0] for m in meshes], axis=0)
    hi = np.max([m.bounds[1] for m in meshes], axis=0)
    return {
        "ok": True,
        "name": name,
        "uri": f"model://{name}",
        "files": [f"models/{name}/{f}" for f in files],
        "bounding_box_m": [round(float(x), 4) for x in hi - lo],
        "triangles": int(sum(len(m.faces) for m in meshes)),
        "import": {"up_axis": up, "scale": round(s, 8), "how": how, "parts": len(meshes), "closed": closed,
                   "triangles_before": int(before)},
        "static": j["static"],
        "collision": {**coll, "shapes": [
            {"kind": sh.kind, **({"vertices": int(len(sh.mesh.vertices))} if sh.kind == "mesh" else {})}
            for sh in shapes]},
        "inertial": {"mass_kg": round(mass, 6), "density_kg_m3": round(density, 3), "volume_m3": round(volume, 8),
                     "center_of_mass_m": [round(float(x), 5) for x in center],
                     "principal_moments": [float(f"{x:.6g}") for x in np.linalg.eigvalsh(inertia)]},
        "warnings": warnings,
        "timings": {"total_s": round(time.monotonic() - t0, 3)},
        "version": VERSION,
    }


def main(argv=None) -> int:
    argv = argv or sys.argv[1:]
    if len(argv) != 2:
        print(__doc__, file=sys.stderr)
        return 1
    job_path, out = argv
    os.makedirs(out, exist_ok=True)
    code = 0
    try:
        with open(job_path) as f:
            job = json.load(f)
        report = build(job, os.path.dirname(os.path.abspath(job_path)), out)
    except ImportError_ as e:
        report, code = {"ok": False, "error": str(e)}, 2
    except Exception as e:  # noqa: BLE001 - report the failure, then exit 1
        report = {"ok": False, "error": f"internal error: {type(e).__name__}: {e}",
                  "traceback": traceback.format_exc(limit=8)}
        code = 1
    with open(os.path.join(out, "report.json"), "w") as f:
        json.dump(report, f, indent=2)
    return code


if __name__ == "__main__":
    sys.exit(main())
