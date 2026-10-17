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
import os
import re
import sys
import time
import traceback

import numpy as np
import trimesh

from . import atlas, collision, export, inertia, parts, preview, tessellate

VERSION = "0.1.0"
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


def build_model(job: dict, out: str) -> dict:
    t0 = time.monotonic()
    timings = {}
    j = check_job(job)
    name = j["name"]
    tree = check_code(j["code"])
    ps = parts.normalize(run_code(tree))
    timings["code_s"] = time.monotonic() - t0

    t = time.monotonic()
    meshes, tess = tessellate.tessellate_parts(ps, j["tri_budget"])
    mesh, face_part = tessellate.combine(meshes)
    offset = tessellate.bottom_center_offset(mesh)
    mesh.apply_translation(offset)
    timings["tessellate_s"] = time.monotonic() - t

    t = time.monotonic()
    visual, utilization = atlas.bake(mesh, face_part, [p.material for p in ps], j["texture_size"])
    timings["bake_s"] = time.monotonic() - t

    t = time.monotonic()
    solid, solid_volume, warnings = watertight(ps, max(tess["tolerance_m"], 1e-4))
    solid.apply_translation(offset)
    shapes, coll = collision.build(solid, solid_volume, j["collision"])
    timings["collision_s"] = time.monotonic() - t

    inert = inertia.compute(ps, j.get("mass"), j.get("density"))
    inert.center = inert.center + offset
    inert.check()
    if inert.volume > solid_volume * 1.02:
        warnings.append(f"parts overlap: their volumes add up to {inert.volume:.4g} m³ but the solid is "
                        f"{solid_volume:.4g} m³, so the mass counts the overlap twice")
    if tess["decimated"]:
        warnings.append(f"decimated from {tess['before_decimation']} to {tess['triangles']} triangles to fit "
                        f"tri_budget; raise it if detail was lost")

    root = os.path.join(out, "models", name)
    files = export.write_model(root, name, visual=visual, shapes=shapes, inertial=inert, static=j["static"],
                               sdf_version=j["sdf_version"], description=j["description"], source=j["code"])
    t = time.monotonic()
    preview.sheet(visual).save(os.path.join(out, "preview.png"))
    timings["preview_s"] = time.monotonic() - t
    timings["total_s"] = time.monotonic() - t0

    size = visual.extents
    principal = np.linalg.eigvalsh(inert.inertia)
    return {
        "ok": True,
        "name": name,
        "uri": f"model://{name}",
        "files": [f"models/{name}/{f}" for f in files],
        "bounding_box_m": [round(float(x), 4) for x in size],
        "triangles": int(len(visual.faces)),
        "vertices": int(len(visual.vertices)),
        "tessellation": tess,
        "parts": [{"name": p.name, "material": p.material.describe(), "volume_m3": round(float(p.shape.volume), 8)}
                  for p in ps],
        "texture": {"size": j["texture_size"], "atlas_utilization": round(utilization, 3)},
        "collision": {**coll, "shapes": [
            {"kind": s.kind, **({"vertices": int(len(s.mesh.vertices))} if s.kind == "mesh" else {})}
            for s in shapes]},
        "inertial": {"mass_kg": round(inert.mass, 6), "volume_m3": round(solid_volume, 8),
                     "center_of_mass_m": [round(float(x), 5) for x in inert.center],
                     "principal_moments": [float(f"{x:.6g}") for x in principal]},
        "static": j["static"],
        "warnings": warnings,
        "timings": {k: round(v, 3) for k, v in timings.items()},
        "versions": versions(),
    }


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
