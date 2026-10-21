"""Inspect an uploaded project file so the AI can see it.

    python -m gzmesh.inspect JOB.json OUT_DIR

JOB.json: {"kind": "document" | "drawing" | "mesh" | "texture",
"path": the file, relative to the job folder (for a zip, its main file
or its folder)}.

OUT_DIR gets report.json, {"ok", "version", "info", "views"}, and the
views it lists:

- ai.jpg, the picture models look at (at most AI_SIZE pixels on the long
  side), and thumb.jpg;
- document (PDF): page-N.jpg for the first MAX_PAGES pages, and the text
  of the pages in info;
- drawing (SVG, DXF): the drawing on white; for DXF, its units, extent,
  layers and text labels with their positions;
- mesh: preview.png, four views; its size, triangle count, and guesses at
  its units and up axis;
- texture (a zip of maps): a swatch of the color map, and which file is
  which map.

Exit status 0 means the file was read; 2 means it couldn't be (damaged,
password-protected, unsupported), and report.json says why; 1 is an
internal error.

Uploads are untrusted input to these parsers: this runs in the same
per-job sandbox as gzmesh.run.
"""

import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import traceback
import xml.etree.ElementTree as ET

import numpy as np
from PIL import Image

from .simplify import main_color, simplify

VERSION = "1"
AI_SIZE = 1568
THUMB_SIZE = 256
MAX_PAGES = 20
MAX_TEXT = 8000
MAX_LABELS = 200
MAX_PREVIEW_FACES = 200_000

# Texture maps by the words in their file names (ambientCG, Poly Haven,
# Substance and most other exporters).
MAP_WORDS = [
    ("normal", ("normalgl", "normal_gl", "normal", "nor_gl", "nrm", "_nor")),
    ("roughness", ("roughness", "rough", "_rgh")),
    ("metalness", ("metalness", "metallic", "metal")),
    ("ao", ("ambientocclusion", "ambient_occlusion", "_ao", "occlusion")),
    ("height", ("displacement", "height", "_disp")),
    ("color", ("basecolor", "base_color", "albedo", "diffuse", "color", "colour", "_col", "_diff")),
]
IMAGE_EXTS = {".png", ".jpg", ".jpeg", ".webp"}


class InspectError(Exception):
    """The file can't be read; the message says why, for the user."""


# -- pictures ------------------------------------------------------------------

def limit_image_pixels():
    """Refuse images over 50 megapixels (PIL only warns below twice its
    limit; make that an error too)."""
    import warnings
    Image.MAX_IMAGE_PIXELS = 50_000_000
    warnings.simplefilter("error", Image.DecompressionBombWarning)


def fit(img: Image.Image, size: int) -> Image.Image:
    img = img.copy()
    img.thumbnail((size, size), Image.LANCZOS)
    return img


def flatten(img: Image.Image) -> Image.Image:
    """img on white, as RGB."""
    if img.mode in ("RGBA", "LA", "P"):
        img = img.convert("RGBA")
        bg = Image.new("RGB", img.size, "white")
        bg.paste(img, mask=img.split()[-1])
        return bg
    return img.convert("RGB")


def save_views(out: str, img: Image.Image, views: list):
    """ai.jpg and thumb.jpg from img."""
    img = flatten(img)
    fit(img, AI_SIZE).save(os.path.join(out, "ai.jpg"), quality=85)
    fit(img, THUMB_SIZE).save(os.path.join(out, "thumb.jpg"), quality=80)
    views += ["ai.jpg", "thumb.jpg"]


# -- documents -----------------------------------------------------------------

def inspect_pdf(path: str, out: str) -> tuple[dict, list]:
    import pypdfium2 as pdfium
    try:
        pdf = pdfium.PdfDocument(path)
    except pdfium.PdfiumError as e:
        raise InspectError(f"the PDF can't be opened (damaged or password-protected): {e}")
    n = len(pdf)
    if n == 0:
        raise InspectError("the PDF has no pages")
    views, texts = [], []
    first = None
    for i in range(min(n, MAX_PAGES)):
        page = pdf[i]
        w, h = page.get_size()  # points
        if i == 0:
            first = (w, h)
        img = page.render(scale=AI_SIZE / max(w, h, 1)).to_pil()
        img = flatten(img)
        name = f"page-{i + 1}.jpg"
        img.save(os.path.join(out, name), quality=85)
        views.append(name)
        if i == 0:
            save_views(out, img, views)
        text = page.get_textpage().get_text_bounded().strip()
        if text:
            texts.append(f"[page {i + 1}]\n{text}")
    text = "\n\n".join(texts)
    info = {"pages": n, "pages_shown": min(n, MAX_PAGES),
            "page_size_mm": [round(first[0] * 25.4 / 72), round(first[1] * 25.4 / 72)]}
    if text:
        info["text"] = text[:MAX_TEXT] + ("…" if len(text) > MAX_TEXT else "")
    return info, views


# -- drawings ------------------------------------------------------------------

def resvg() -> str:
    exe = os.environ.get("GZMESH_RESVG") or shutil.which("resvg")
    if not exe:
        raise RuntimeError("resvg is not installed")
    return exe


def render_svg(svg_path: str) -> Image.Image:
    """Rasterize an SVG on white, fitted in AI_SIZE pixels square. Giving
    resvg both sides keeps the aspect and bounds the raster, however tall
    or wide the drawing is."""
    with tempfile.TemporaryDirectory() as tmp:
        png = os.path.join(tmp, "out.png")
        try:
            r = subprocess.run([resvg(), "--background", "white", "--width", str(AI_SIZE), "--height", str(AI_SIZE),
                                svg_path, png], capture_output=True, text=True, timeout=60)
        except subprocess.TimeoutExpired:
            raise InspectError("the drawing took too long to draw")
        if r.returncode != 0:
            raise InspectError("the drawing can't be drawn: " + (r.stderr.strip().splitlines() or ["?"])[-1])
        img = Image.open(png)
        img.load()
        return img


def svg_info(path: str) -> dict:
    info = {}
    try:
        root = ET.parse(path).getroot()
    except ET.ParseError as e:
        raise InspectError(f"the SVG isn't valid XML: {e}")
    for k in ("width", "height", "viewBox"):
        if root.get(k):
            info[k if k != "viewBox" else "view_box"] = root.get(k)
    labels = []
    for el in root.iter():
        if el.tag.rsplit("}", 1)[-1] == "text":
            t = " ".join("".join(el.itertext()).split())
            if t:
                labels.append(t)
        if len(labels) >= MAX_LABELS:
            break
    if labels:
        info["labels"] = labels
    return info


DXF_UNITS = {0: "unitless", 1: "inches", 2: "feet", 4: "millimeters", 5: "centimeters", 6: "meters",
             14: "decimeters"}


def inspect_dxf(path: str, out: str) -> tuple[dict, list]:
    import ezdxf
    from ezdxf import bbox, recover
    from ezdxf.addons.drawing import Frontend, RenderContext, config, layout, svg

    try:
        doc, auditor = recover.readfile(path)
    except (IOError, ezdxf.DXFStructureError) as e:
        raise InspectError(f"the DXF can't be read: {e}")
    msp = doc.modelspace()
    units = DXF_UNITS.get(doc.header.get("$INSUNITS", 0), "unknown")
    info = {"units": units, "entities": len(msp)}
    ext = bbox.extents(msp, fast=True)
    if not ext.has_data:
        raise InspectError("the drawing is empty: its model space has nothing to draw")
    xy = lambda v: [round(v.x, 4), round(v.y, 4)]  # noqa: E731
    info["extent"] = {"min": xy(ext.extmin), "max": xy(ext.extmax), "size": xy(ext.size)}
    layers, labels = set(), []
    for e in msp:
        layers.add(e.dxf.get("layer", "0"))
        t = e.dxftype()
        try:
            if t == "TEXT":
                text, at = e.dxf.text, e.dxf.insert
            elif t == "MTEXT":
                text, at = e.plain_text(), e.dxf.insert
            elif t == "DIMENSION":
                text, at = f"{e.get_measurement():g}", e.dxf.get("text_midpoint", e.dxf.defpoint)
            else:
                continue
        except Exception:  # noqa: BLE001 - a label we can't read is left out
            continue
        text = " ".join(str(text).split())
        if text and len(labels) < MAX_LABELS:
            labels.append({"text": text, "at": [round(at[0], 3), round(at[1], 3)]})
    info["layers"] = sorted(layers)[:100]  # the layers in use
    if labels:
        info["labels"] = labels

    backend = svg.SVGBackend()
    # Lines at least 0.5 mm wide on the 400 mm page, so thin walls survive
    # scaling down to AI_SIZE.
    cfg = config.Configuration(background_policy=config.BackgroundPolicy.WHITE,
                               color_policy=config.ColorPolicy.BLACK, min_lineweight=6)
    # A page of the drawing's shape with a thin margin, scaled to fit; a
    # line-like drawing gets a page no thinner than 1:100.
    w, h = max(ext.size.x, 1e-9), max(ext.size.y, 1e-9)
    w, h = max(w, h / 100), max(h, w / 100)
    page = layout.Page(400, 400 * h / w, layout.Units.mm, margins=layout.Margins.all(4))
    try:
        Frontend(RenderContext(doc), backend, config=cfg).draw_layout(msp)
        drawing = backend.get_string(page, settings=layout.Settings(fit_page=True))
    except InspectError:
        raise
    except Exception as e:  # noqa: BLE001 - ezdxf raises many kinds on bad drawings
        raise InspectError(f"the DXF can't be drawn: {type(e).__name__}: {e}")
    with tempfile.TemporaryDirectory() as tmp:
        svg_path = os.path.join(tmp, "drawing.svg")
        with open(svg_path, "w") as f:
            f.write(drawing)
        img = render_svg(svg_path)
    views = []
    save_views(out, img, views)
    return info, views


def inspect_svg(path: str, out: str) -> tuple[dict, list]:
    info = svg_info(path)
    img = render_svg(path)
    views = []
    save_views(out, img, views)
    return info, views


# -- meshes --------------------------------------------------------------------

def up_axis(path: str) -> str:
    """The up axis a mesh file's format implies: glTF and OBJ are Y-up by
    convention, Collada says, STL is usually Z-up (CAD)."""
    ext = os.path.splitext(path)[1].lower()
    if ext == ".dae":
        with open(path, "rb") as f:
            m = re.search(rb"<up_axis>\s*([XYZ])_UP\s*</up_axis>", f.read(1 << 20))
        return m.group(1).decode().lower() if m else "y"
    return "z" if ext == ".stl" else "y"


def units_guess(size: np.ndarray, ext: str = "") -> str:
    """Units a mesh's largest dimension suggests: few objects to place in
    a world are over 50 m, or under 1 cm. STL files come from CAD and 3D
    printing, which work in millimeters."""
    d = float(np.max(size))
    if ext == ".stl" and d > 2:
        return "millimeters"
    if d > 500:
        return "millimeters"
    if d > 50:
        return "centimeters"
    return "meters"


def to_z_up(up: str) -> np.ndarray:
    if up == "y":  # +Y up to +Z up
        return np.array([[1, 0, 0, 0], [0, 0, -1, 0], [0, 1, 0, 0], [0, 0, 0, 1]], float)
    if up == "x":  # +X up to +Z up
        return np.array([[0, 0, -1, 0], [0, 1, 0, 0], [1, 0, 0, 0], [0, 0, 0, 1]], float)
    return np.eye(4)


def missing_textures(root: str) -> list:
    """Image files that OBJ material libraries name but the upload lacks."""
    missing = []
    for dirpath, _, files in os.walk(root):
        for f in files:
            if not f.lower().endswith(".mtl"):
                continue
            with open(os.path.join(dirpath, f), errors="replace") as fh:
                for line in fh:
                    parts = line.split()
                    if parts and parts[0].lower().startswith("map_") and len(parts) > 1:
                        ref = parts[-1].replace("\\", "/")
                        if not os.path.isfile(os.path.join(dirpath, ref)):
                            missing.append(ref)
    return sorted(set(missing))[:20]


def load_scene(path: str):
    import trimesh
    try:
        return trimesh.load(path, force="scene")
    except Exception as e:  # noqa: BLE001 - every parser raises its own
        raise InspectError(f"the mesh can't be read: {type(e).__name__}: {e}")


def preview_meshes(scene, transform: np.ndarray) -> list:
    """The scene's meshes, Z up, each with a base color texture for the
    preview renderer (a 1-pixel one of its color when it has none)."""
    import trimesh
    from trimesh.visual.material import PBRMaterial
    from trimesh.visual.texture import TextureVisuals

    out = []
    for geom in scene.dump():  # with the scene graph's transforms applied
        if not isinstance(geom, trimesh.Trimesh) or len(geom.faces) == 0:
            continue
        m = geom.copy()
        m.apply_transform(transform)
        image, uv = None, None
        vis = m.visual
        if isinstance(vis, TextureVisuals) and vis.uv is not None and len(vis.uv) == len(m.vertices):
            mat = vis.material
            pbr = mat.to_pbr() if hasattr(mat, "to_pbr") else mat
            image = getattr(pbr, "baseColorTexture", None)
            uv = vis.uv
        if image is None:
            color = main_color(m)
            image = Image.new("RGB", (1, 1), tuple(int(c) for c in color[:3]))
            uv = np.zeros((len(m.vertices), 2))
        m.visual = TextureVisuals(uv=uv, material=PBRMaterial(baseColorTexture=image.convert("RGB")))
        out.append(m)
    return out


def inspect_mesh(path: str, out: str) -> tuple[dict, list]:
    import trimesh

    from . import preview

    scene = load_scene(path)
    tris = sum(len(g.faces) for g in scene.geometry.values() if isinstance(g, trimesh.Trimesh))
    if tris == 0 or scene.bounds is None:
        raise InspectError("the mesh has no triangles")
    size = scene.extents
    up = up_axis(path)
    info = {
        "format": os.path.splitext(path)[1].lower().lstrip("."),
        "triangles": int(tris),
        "parts": len(scene.geometry),
        "size": [round(float(v), 4) for v in size],
        "bounds": {"min": [round(float(v), 4) for v in scene.bounds[0]],
                   "max": [round(float(v), 4) for v in scene.bounds[1]]},
        "units_guess": units_guess(size, os.path.splitext(path)[1].lower()),
        "up_axis_guess": up,
        "materials": sorted({getattr(getattr(g.visual, "material", None), "name", None) or ""
                             for g in scene.geometry.values()} - {""})[:50],
    }
    missing = missing_textures(os.path.dirname(path))
    if missing:
        info["missing_textures"] = missing
    meshes = preview_meshes(scene, to_z_up(up))
    faces = sum(len(m.faces) for m in meshes)
    if faces > MAX_PREVIEW_FACES:
        # Coarser copies for the preview only.
        ratio = 1 - MAX_PREVIEW_FACES / faces
        meshes = [simplify(m, ratio) if len(m.faces) > 50 else m for m in meshes]
    sheet = preview.sheet(meshes)
    sheet.save(os.path.join(out, "preview.png"))
    views = ["preview.png"]
    save_views(out, sheet, views)
    return info, views


# -- texture sets --------------------------------------------------------------

# Name tokens that are a resolution or format, not a map: 1k, 2K, 4096, jpg.
NOISE_TOKEN = re.compile(r"^(\d+k|\d{3,5}(px)?|jpe?g|png|webp|exr|tiff?|hd|lod\d*)$")
TOKEN_KINDS = {}
for _kind, _words in MAP_WORDS:
    for _w in _words:
        TOKEN_KINDS.setdefault(_w.strip("_"), _kind)


def map_kind(name: str):
    """The map a file is, from the last word of its name that names one:
    Metal032_2K-JPG_Color.jpg is a color map, metal_plate_diff_1k.jpg too."""
    stem = os.path.splitext(os.path.basename(name))[0].lower()
    tokens = [t for t in re.split(r"[^a-z0-9]+", stem) if t and not NOISE_TOKEN.match(t)]
    for t in reversed(tokens):
        if t in TOKEN_KINDS:
            return TOKEN_KINDS[t]
    # Run-together names (BricksNormal.png): look inside, color first.
    for kind, words in sorted(MAP_WORDS, key=lambda kw: kw[0] != "color"):
        if any(w.strip("_") in stem for w in words if len(w.strip("_")) > 3):
            return kind
    return None


def inspect_texture(path: str, out: str) -> tuple[dict, list]:
    files = []
    for dirpath, dirs, names in os.walk(path):
        dirs[:] = [d for d in dirs if d != "__MACOSX" and not d.startswith(".")]
        for n in names:
            if os.path.splitext(n)[1].lower() in IMAGE_EXTS and not n.startswith("."):
                files.append(os.path.relpath(os.path.join(dirpath, n), path))
    if not files:
        raise InspectError("the texture set has no images")
    maps, unknown = {}, []
    for f in sorted(files):
        kind = map_kind(f)
        if kind and kind not in maps:
            maps[kind] = f
        else:
            unknown.append(f)
    if "color" not in maps:
        if len(unknown) == 1 and len(files) == 1:
            maps["color"] = unknown.pop()
        else:
            raise InspectError("no color map: name it with color, albedo, basecolor or diffuse "
                               f"(files: {', '.join(sorted(files)[:10])})")
    try:
        img = Image.open(os.path.join(path, maps["color"]))
        img.load()
    except Exception as e:  # noqa: BLE001
        raise InspectError(f"{maps['color']} can't be read: {e}")
    info = {"maps": maps, "size": list(img.size)}
    if unknown:
        info["unused"] = unknown[:20]
    views = []
    save_views(out, img, views)
    return info, views


# -- main ----------------------------------------------------------------------

def inspect(job: dict, base: str, out: str) -> dict:
    kind = job.get("kind")
    path = os.path.normpath(os.path.join(base, job.get("path", "")))
    if not path.startswith(os.path.normpath(base) + os.sep) or not os.path.exists(path):
        raise InspectError("the file to inspect is missing")
    ext = os.path.splitext(path)[1].lower()
    limit_image_pixels()
    if kind == "document":
        info, views = inspect_pdf(path, out)
    elif kind == "drawing" and ext == ".dxf":
        info, views = inspect_dxf(path, out)
    elif kind == "drawing":
        info, views = inspect_svg(path, out)
    elif kind == "mesh":
        info, views = inspect_mesh(path, out)
    elif kind == "texture":
        info, views = inspect_texture(path if os.path.isdir(path) else os.path.dirname(path), out)
    else:
        raise InspectError(f"unknown kind {kind!r}")
    return {"ok": True, "version": VERSION, "info": info, "views": views}


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
        report = inspect(job, os.path.dirname(os.path.abspath(job_path)), out)
    except InspectError as e:
        report, code = {"ok": False, "version": VERSION, "error": str(e)}, 2
    except Exception as e:  # noqa: BLE001 - report the failure, then exit 1
        report = {"ok": False, "version": VERSION, "error": f"internal error: {type(e).__name__}: {e}",
                  "detail": traceback.format_exc(limit=5)}
        code = 1
    with open(os.path.join(out, "report.json"), "w") as f:
        json.dump(report, f, indent=1)
    return code


if __name__ == "__main__":
    sys.exit(main())
