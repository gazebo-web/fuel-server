"""Run one mesh job: agent-written build123d code to a model folder.

    python -m gzmesh.run JOB.json OUT_DIR

JOB.json: {"name", "code", and optionally "mass" or "density", "static",
"tri_budget", "texture_size", "collision", "sdf_version", "description"}.

OUT_DIR gets models/<name>/ (model.sdf, model.config, meshes/*.glb,
source/model.py), preview.png and report.json. Exit status 0 means the
model was built; 2 means the job or its code failed, and report.json says
why with line numbers in model.py; 1 is an internal error.

This process runs inside the per-job sandbox (nsjail, no network; plan
§5.7). The import allow-list below is a second line of defence that also
gives the agent a clear error, not the security boundary.
"""

import ast
import builtins
import json
import math
import os
import re
import sys
import time
import traceback

import numpy as np
import trimesh

from . import atlas, collision, export, inertia, parts, preview, tessellate

VERSION = "0.3.0"
NAME_RE = re.compile(r"^[a-z][a-z0-9_]{0,63}$")
ALLOWED_IMPORTS = {"build123d", "gzmesh", "math", "numpy", "random", "itertools", "functools", "typing",
                   "dataclasses", "collections", "copy"}
FORBIDDEN_NAMES = {"open", "exec", "eval", "compile", "__import__", "globals", "locals", "vars", "input",
                   "breakpoint", "exit", "quit", "help", "memoryview"}
LIMITS = {"tri_budget": (100, 100_000), "texture_size": (256, 2048)}
DEFAULTS = {"tri_budget": 15_000, "texture_size": 1024, "collision": "auto", "static": False,
            "sdf_version": "1.9", "description": ""}


class JobError(Exception):
    """A problem with the job or the agent's code, reported back to it."""

    def __init__(self, message, line=None, detail=None):
        super().__init__(message)
        self.line, self.detail = line, detail


def check_job(job: dict) -> dict:
    j = {**DEFAULTS, **job}
    if not isinstance(j.get("name"), str) or not NAME_RE.match(j["name"]):
        raise JobError("name must be lowercase letters, digits and underscores, starting with a letter")
    if not isinstance(j.get("code"), str) or not j["code"].strip():
        raise JobError("code is required: build123d Python that defines build()")
    for k, (lo, hi) in LIMITS.items():
        if not isinstance(j[k], int) or not lo <= j[k] <= hi:
            raise JobError(f"{k} must be an integer from {lo} to {hi}")
    if j["texture_size"] & (j["texture_size"] - 1):
        raise JobError("texture_size must be a power of two")
    for k in ("mass", "density"):
        if j.get(k) is not None and not (isinstance(j[k], (int, float)) and j[k] > 0):
            raise JobError(f"{k} must be a positive number")
    return j


def check_code(code: str) -> ast.Module:
    try:
        tree = ast.parse(code, "model.py")
    except SyntaxError as e:
        raise JobError(f"syntax error: {e.msg}", e.lineno)
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            mods = [a.name for a in node.names]
        elif isinstance(node, ast.ImportFrom):
            mods = [node.module or ""]
        else:
            mods = []
        for m in mods:
            if m.split(".")[0] not in ALLOWED_IMPORTS:
                raise JobError(f"import {m!r} is not allowed; use {', '.join(sorted(ALLOWED_IMPORTS))}", node.lineno)
        if isinstance(node, ast.Name) and node.id in FORBIDDEN_NAMES:
            raise JobError(f"{node.id}() is not available in model code", node.lineno)
        if isinstance(node, ast.Attribute) and node.attr.startswith("__"):
            raise JobError(f"dunder attribute {node.attr!r} is not allowed", node.lineno)
    if not any(isinstance(n, ast.FunctionDef) and n.name == "build" for n in tree.body):
        raise JobError("the code must define build() returning the model's parts")
    return tree


def _guarded_import(name, globals=None, locals=None, fromlist=(), level=0):
    if level or name.split(".")[0] not in ALLOWED_IMPORTS:
        raise ImportError(f"import {name!r} is not allowed")
    return builtins.__import__(name, globals, locals, fromlist, level)


def run_code(tree: ast.Module):
    safe = {k: getattr(builtins, k) for k in dir(builtins) if not k.startswith("_") and k not in FORBIDDEN_NAMES}
    safe["__import__"] = _guarded_import
    safe["__build_class__"] = builtins.__build_class__
    ns = {"__name__": "model", "__builtins__": safe}
    try:
        exec(compile(tree, "model.py", "exec"), ns)
        return ns["build"]()
    except JobError:
        raise
    except Exception as e:
        tb = traceback.extract_tb(e.__traceback__)
        line = next((fr.lineno for fr in reversed(tb) if fr.filename == "model.py"), None)
        raise JobError(f"{type(e).__name__}: {e}", line,
                       "".join(traceback.format_exception_only(type(e), e)).strip())


def watertight(ps: list[parts.Part], tolerance: float) -> tuple[trimesh.Trimesh, float, list[str]]:
    """One closed mesh of the whole model, and its exact volume (B-rep
    union), for collision fitting."""
    warnings = []
    try:
        fused = ps[0].shape.fuse(*[p.shape for p in ps[1:]]) if len(ps) > 1 else ps[0].shape
        vol = float(fused.volume)
        mesh = tessellate.tessellate_shape(fused, tolerance, 0.3)
    except Exception as e:
        warnings.append(f"could not fuse the parts ({e}); collision uses the parts as they are")
        mesh = trimesh.util.concatenate([tessellate.tessellate_shape(p.shape, tolerance, 0.3) for p in ps])
        vol = float(sum(p.shape.volume for p in ps))
    mesh.merge_vertices()
    return mesh, vol, warnings


def link_texture_size(area: float, total: float, size: int) -> int:
    """Texture size for a link with `area` of the model's `total` surface:
    the same texel density as one atlas of `size` for the whole model,
    rounded to a power of two, at least 256."""
    want = size * math.sqrt(area / total) if total > 0 else size
    return int(min(size, max(256, 2 ** round(math.log2(max(want, 1))))))


def build_model(job: dict, out: str) -> dict:
    t0 = time.monotonic()
    timings = {}
    j = check_job(job)
    name = j["name"]
    tree = check_code(j["code"])
    asm = parts.normalize(run_code(tree))
    single = len(asm.links) == 1
    timings["code_s"] = time.monotonic() - t0

    t = time.monotonic()
    all_parts = [p for lk in asm.links for p in lk.parts]
    meshes, tess = tessellate.tessellate_parts(all_parts, j["tri_budget"])
    by_link, i = [], 0
    for lk in asm.links:
        by_link.append(meshes[i:i + len(lk.parts)])
        i += len(lk.parts)
    offset = tessellate.bottom_center_offset(tessellate.combine(meshes)[0])
    timings["tessellate_s"] = time.monotonic() - t

    inertials = inertia.compute_links(asm.links, j.get("mass"), j.get("density"))
    total_area = sum(m.area for m in meshes)
    warnings, links, link_reports, solids = [], [], [], {}
    timings["bake_s"] = timings["collision_s"] = 0.0
    for lk, lmeshes, inert in zip(asm.links, by_link, inertials):
        t = time.monotonic()
        mesh, face_part = tessellate.combine(lmeshes)
        mesh.apply_translation(offset)
        size = j["texture_size"] if single else link_texture_size(mesh.area, total_area, j["texture_size"])
        visual, utilization = atlas.bake(mesh, face_part, [p.material for p in lk.parts], size)
        timings["bake_s"] += time.monotonic() - t

        t = time.monotonic()
        solid, solid_volume, w = watertight(lk.parts, max(tess["tolerance_m"], 1e-4))
        warnings += w
        solid.apply_translation(offset)
        solids[lk.name] = solid
        shapes, coll = collision.build(solid, solid_volume, j["collision"])
        timings["collision_s"] += time.monotonic() - t

        inert.center = inert.center + offset
        try:
            inert.check()
        except ValueError as e:
            raise ValueError(f"link {lk.name!r}: {e}") from None
        where = "" if single else f"link {lk.name!r}: "
        if inert.volume > solid_volume * 1.02:
            warnings.append(f"{where}parts overlap: their volumes add up to {inert.volume:.4g} m³ but the solid is "
                            f"{solid_volume:.4g} m³, so the mass counts the overlap twice")
        links.append(export.LinkOut(lk.name, visual, inert, shapes, name if single else f"{name}_{lk.name}",
                                    friction=lk.friction))
        link_reports.append({
            "name": lk.name,
            "parts": [{"name": p.name, "material": p.material.describe(),
                       "volume_m3": round(float(p.shape.volume), 8)} for p in lk.parts],
            "triangles": int(len(visual.faces)),
            "bounding_box_m": [round(float(x), 4) for x in visual.extents],
            "texture": {"size": size, "atlas_utilization": round(utilization, 3)},
            "collision": {**coll, "shapes": [
                {"kind": s.kind, **({"vertices": int(len(s.mesh.vertices))} if s.kind == "mesh" else {})}
                for s in shapes]},
            "inertial": {"mass_kg": round(inert.mass, 6), "volume_m3": round(solid_volume, 8),
                         "center_of_mass_m": [round(float(x), 5) for x in inert.center],
                         "principal_moments": [float(f"{x:.6g}") for x in np.linalg.eigvalsh(inert.inertia)]},
        })
    if tess["decimated"]:
        warnings.append(f"decimated from {tess['before_decimation']} to {tess['triangles']} triangles to fit "
                        f"tri_budget; raise it if detail was lost")
    warnings += joint_warnings(asm, links, offset)
    by_name = {lk.name: lk for lk in links}
    sensor_reports, w = place_sensors(asm, by_name, solids, offset)
    warnings += w
    plugins, plugin_reports, w = resolve_plugins(asm, by_name, offset)
    warnings += w

    root = os.path.join(out, "models", name)
    files = export.write_model(root, name, links=links, joints=asm.joints, offset=offset, static=j["static"],
                               sdf_version=j["sdf_version"], description=j["description"], source=j["code"],
                               plugins=plugins)
    t = time.monotonic()
    moved = moved_links(asm, links, offset) if asm.joints else None
    markers = [(sn.name, sn.kind, sn.pos, look_direction(sn.rpy)) for lk in links for sn in lk.sensors]
    preview.sheet([lk.visual for lk in links], moved=moved, markers=markers).save(os.path.join(out, "preview.png"))
    timings["preview_s"] = time.monotonic() - t
    timings["total_s"] = time.monotonic() - t0

    lo = np.min([lk.visual.bounds[0] for lk in links], axis=0)
    hi = np.max([lk.visual.bounds[1] for lk in links], axis=0)
    report = {
        "ok": True,
        "name": name,
        "uri": f"model://{name}",
        "files": [f"models/{name}/{f}" for f in files],
        "bounding_box_m": [round(float(x), 4) for x in hi - lo],
        "triangles": sum(r["triangles"] for r in link_reports),
        "tessellation": tess,
        "static": j["static"],
        "warnings": warnings,
        "timings": {k: round(v, 3) for k, v in timings.items()},
        "versions": versions(),
    }
    if single:
        lr = link_reports[0]
        report.update(parts=lr["parts"], texture=lr["texture"], collision=lr["collision"], inertial=lr["inertial"])
    else:
        report["inertial"] = {"mass_kg": round(sum(lk.inertial.mass for lk in links), 6)}
        report["links"] = link_reports
        report["joints"] = [{
            "name": jt.name, "type": jt.kind, "parent": jt.parent, "child": jt.child,
            **({} if jt.kind == "fixed" else {
                "origin_m": [round(float(x), 5) for x in jt.origin + offset],
                "axis": [round(float(x), 5) for x in jt.axis]}),
            **({"limits": [jt.lower, jt.upper]} if jt.kind in ("revolute", "prismatic") else {}),
        } for jt in asm.joints]
    if sensor_reports:
        report["sensors"] = sensor_reports
    if plugin_reports:
        report["plugins"] = plugin_reports
    systems = world_systems(asm)
    if systems:
        report["world_systems"] = {
            "note": "the world must load these systems or the sensors publish nothing; add each <plugin> "
                    "inside <world>, next to Physics, UserCommands and SceneBroadcaster",
            "plugins": systems}
    return report


# Topics each sensor type publishes, under
# /world/<world>/model/<instance>/link/<link>/sensor/<name>/.
SENSOR_TOPICS = {"camera": ["image", "camera_info"], "depth_camera": ["depth_image", "points", "camera_info"],
                 "rgbd_camera": ["image", "depth_image", "points", "camera_info"],
                 "gpu_lidar": ["scan", "scan/points"], "imu": ["imu"]}
SENSORS_SYSTEM = ('<plugin filename="gz-sim-sensors-system" name="gz::sim::systems::Sensors">'
                  '<render_engine>ogre2</render_engine></plugin>')
IMU_SYSTEM = '<plugin filename="gz-sim-imu-system" name="gz::sim::systems::Imu"/>'


def world_systems(asm) -> list[str]:
    kinds = {s.kind for s in asm.sensors}
    out = []
    if kinds - {"imu"}:
        out.append(SENSORS_SYSTEM)
    if "imu" in kinds:
        out.append(IMU_SYSTEM)
    return out


def look_direction(rpy) -> np.ndarray:
    """The sensor's +X in the model frame, for roll-pitch-yaw rpy."""
    from trimesh.transformations import euler_matrix
    return euler_matrix(*rpy, axes="sxyz")[:3, 0]


def place_sensors(asm, links: dict, solids: dict, offset) -> tuple[list, list]:
    """Attaches each sensor to its link's output, moved by offset like
    everything else; warns about sensors buried in or far from their
    link."""
    reports, warnings = [], []
    for s in asm.sensors:
        lk = links[s.link]
        placed = parts.Sensor(s.name, s.kind, s.link, s.pos + offset, s.rpy, s.rate, s.params)
        lk.sensors.append(placed)
        lo, hi = lk.visual.bounds
        margin = 0.25 * float((hi - lo).max())
        if ((placed.pos < lo - margin) | (placed.pos > hi + margin)).any():
            warnings.append(f"sensor {s.name!r}: pos {np.round(s.pos, 3).tolist()} is far from link {s.link!r}; "
                            f"sensor positions are in the model frame, as built")
        else:
            try:
                buried = bool(solids[s.link].contains([placed.pos])[0])
            except Exception:
                buried = False
            if buried and s.kind != "imu":
                warnings.append(f"sensor {s.name!r} is inside link {s.link!r}'s solid, so it sees the inside "
                                f"of the part; move it onto the surface")
        base = f"/world/<world>/model/<instance>/link/{s.link}/sensor/{s.name}/"
        reports.append({"name": s.name, "type": s.kind, "link": s.link,
                        "position_m": [round(float(x), 4) + 0.0 for x in placed.pos],
                        **({} if s.kind == "imu" else {
                            "looks_toward": [round(float(x), 3) + 0.0 for x in look_direction(s.rpy)]}),
                        "rate_hz": s.rate, "topics": [base + t for t in SENSOR_TOPICS[s.kind]]})
    return reports, warnings


def wheel_radius(jt, child, offset) -> float:
    """The largest distance of the wheel link's surface from the joint
    axis."""
    o = jt.origin + offset
    return float(np.linalg.norm(np.cross(child.visual.vertices - o, jt.axis), axis=1).max())


def resolve_plugins(asm, links: dict, offset) -> tuple[list, list, list]:
    """(kind, params) for export with wheel geometry filled in, reports
    with each plugin's topics, and warnings about wheels that would drive
    the robot the wrong way."""
    joints = {jt.name: jt for jt in asm.joints}
    plugins, reports, warnings = [], [], []
    for p in asm.plugins:
        k = dict(p.params)
        if p.kind == "diff_drive":
            left = [joints[n] for n in k["left"]]
            right = [joints[n] for n in k["right"]]
            L = np.mean([jt.origin for jt in left], axis=0)
            R = np.mean([jt.origin for jt in right], axis=0)
            if "wheel_separation" not in k:
                axis = left[0].axis
                k["wheel_separation"] = float(abs((L - R) @ axis)) or float(np.linalg.norm(L - R))
            if "wheel_radius" not in k:
                k["wheel_radius"] = float(np.mean([wheel_radius(jt, links[jt.child], offset) for jt in left + right]))
            if (L - R)[1] <= 0:
                warnings.append("diff_drive: the left wheels aren't on the +Y side of the right ones; the front is "
                                "+X, so left is +Y. Swap left and right, or the robot turns the wrong way")
            for jt in left + right:
                if jt.axis[1] < 0.7:
                    warnings.append(f"diff_drive: joint {jt.name!r} axis is {np.round(jt.axis, 3).tolist()}; "
                                    f"wheel axes must be (0, 1, 0) so positive speed rolls toward +X")
            reports.append({"type": "diff_drive", "cmd_vel": "/model/<instance>/cmd_vel",
                            "odometry": "/model/<instance>/odometry",
                            "wheel_separation_m": round(k["wheel_separation"], 4),
                            "wheel_radius_m": round(k["wheel_radius"], 4), "max_speed_mps": k["max_speed"]})
        elif p.kind == "joint_states":
            reports.append({"type": "joint_states", "topic": "/world/<world>/model/<instance>/joint_state"})
        elif p.kind == "position_controller":
            reports.append({"type": "position_controller", "joint": k["joint"],
                            "topic": f"/model/<instance>/joint/{k['joint']}/0/cmd_pos"})
        elif p.kind == "velocity_controller":
            reports.append({"type": "velocity_controller", "joint": k["joint"],
                            "topic": f"/model/<instance>/joint/{k['joint']}/cmd_vel"})
        plugins.append((p.kind, k))
    return plugins, reports, warnings


def moved_links(asm, links, offset) -> list:
    """Link visuals with every joint moved: revolute and prismatic to 60%
    of their larger travel from 0, continuous by 0.8 rad. Transforms
    compose down the tree, so a door on a moved drawer moves with it."""
    from trimesh.transformations import rotation_matrix, translation_matrix
    by_child = {jt.child: jt for jt in asm.joints}
    cache = {}

    def transform(name):
        if name in cache:
            return cache[name]
        jt = by_child.get(name)
        if jt is None or jt.kind == "fixed":
            T = np.eye(4) if jt is None else transform(jt.parent)
        else:
            if jt.kind == "continuous":
                q = 0.8
            else:
                q = 0.6 * (jt.upper if abs(jt.upper) >= abs(jt.lower) else jt.lower)
            o = jt.origin + offset
            motion = (translation_matrix(jt.axis * q) if jt.kind == "prismatic"
                      else rotation_matrix(q, jt.axis, o))
            T = transform(jt.parent) @ motion
        cache[name] = T
        return T

    out = []
    for lk in links:
        m = lk.visual.copy()
        m.apply_transform(transform(lk.name))
        out.append(m)
    return out


def joint_warnings(asm, links, offset) -> list[str]:
    """A joint whose origin is far from both links it connects is almost
    always a mistake (an origin in the wrong frame, or millimeters for
    meters)."""
    out = []
    boxes = {lk.name: lk.visual.bounds for lk in links}
    for jt in asm.joints:
        if jt.kind == "fixed":
            continue
        o = jt.origin + offset
        lo = np.minimum(boxes[jt.parent][0], boxes[jt.child][0])
        hi = np.maximum(boxes[jt.parent][1], boxes[jt.child][1])
        margin = 0.25 * float((hi - lo).max())
        if ((o < lo - margin) | (o > hi + margin)).any():
            out.append(f"joint {jt.name!r}: origin {np.round(jt.origin, 3).tolist()} is far from links "
                       f"{jt.parent!r} and {jt.child!r}; joint origins are in the model frame, as built")
    return out


def versions() -> dict:
    from importlib.metadata import version
    out = {"gzmesh": VERSION}
    for pkg in ("build123d", "cadquery-ocp-novtk", "trimesh", "xatlas", "coacd", "fast-simplification"):
        try:
            out[pkg] = version(pkg)
        except Exception:
            pass
    return out


def main(argv=None) -> int:
    argv = argv or sys.argv[1:]
    if len(argv) != 2:
        print(__doc__, file=sys.stderr)
        return 1
    job_path, out = argv
    os.makedirs(out, exist_ok=True)
    try:
        with open(job_path) as f:
            job = json.load(f)
        report, code = build_model(job, out), 0
    except JobError as e:
        report = {"ok": False, "error": str(e), "line": e.line, "detail": e.detail}
        code = 2
    except ValueError as e:  # contract violations surfaced by the pipeline
        report, code = {"ok": False, "error": str(e)}, 2
    except Exception as e:
        report = {"ok": False, "error": f"internal error: {type(e).__name__}: {e}",
                  "traceback": traceback.format_exc()}
        code = 1
    with open(os.path.join(out, "report.json"), "w") as f:
        json.dump(report, f, indent=2)
    return code


if __name__ == "__main__":
    sys.exit(main())
