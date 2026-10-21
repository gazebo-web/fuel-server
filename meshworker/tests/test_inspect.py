"""gzmesh.inspect: pictures and details of uploaded files."""

import json
import os
import shutil
import zipfile

import numpy as np
import pytest
import trimesh
from PIL import Image

from gzmesh import inspect

RESVG = os.environ.get("GZMESH_RESVG") or shutil.which("resvg")
needs_resvg = pytest.mark.skipif(not RESVG, reason="resvg is not installed (set GZMESH_RESVG)")


def run(tmp_path, kind, path):
    job = tmp_path / "job.json"
    job.write_text(json.dumps({"kind": kind, "path": path}))
    out = tmp_path / "out"
    code = inspect.main([str(job), str(out)])
    return code, json.loads((out / "report.json").read_text()), out


def make_pdf(path, pages=2):
    """A PDF with text on each page, written by hand."""
    objs = ["<< /Type /Catalog /Pages 2 0 R >>", None, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"]
    kids = []
    for i in range(pages):
        stream = f"BT /F1 24 Tf 72 720 Td (Room {i + 1}: 4.2 m x 3 m) Tj ET".encode()
        objs.append(f"<< /Length {len(stream)} >>\nstream\n{stream.decode()}\nendstream")
        content = len(objs)
        objs.append(f"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents {content} 0 R "
                    f"/Resources << /Font << /F1 3 0 R >> >> >>")
        kids.append(f"{len(objs)} 0 R")
    objs[1] = f"<< /Type /Pages /Kids [{' '.join(kids)}] /Count {pages} >>"
    out, offsets = b"%PDF-1.4\n", []
    for n, body in enumerate(objs, 1):
        offsets.append(len(out))
        out += f"{n} 0 obj\n{body}\nendobj\n".encode()
    xref = len(out)
    out += f"xref\n0 {len(objs) + 1}\n0000000000 65535 f \n".encode()
    out += "".join(f"{o:010d} 00000 n \n" for o in offsets).encode()
    out += f"trailer\n<< /Size {len(objs) + 1} /Root 1 0 R >>\nstartxref\n{xref}\n%%EOF\n".encode()
    path.write_bytes(out)


def test_pdf(tmp_path):
    make_pdf(tmp_path / "plan.pdf", pages=3)
    code, report, out = run(tmp_path, "document", "plan.pdf")
    assert code == 0, report
    info = report["info"]
    assert info["pages"] == 3 and info["page_size_mm"] == [216, 279]
    assert "Room 2: 4.2 m x 3 m" in info["text"]
    assert report["views"][:3] == ["page-1.jpg", "ai.jpg", "thumb.jpg"]
    assert "page-3.jpg" in report["views"]
    assert max(Image.open(out / "page-1.jpg").size) == inspect.AI_SIZE


def test_bad_pdf(tmp_path):
    (tmp_path / "x.pdf").write_bytes(b"%PDF-1.4 not really")
    code, report, _ = run(tmp_path, "document", "x.pdf")
    assert code == 2 and "PDF" in report["error"]


@needs_resvg
def test_svg(tmp_path):
    (tmp_path / "room.svg").write_text(
        '<svg xmlns="http://www.w3.org/2000/svg" width="400" height="200" viewBox="0 0 400 200">'
        '<rect x="10" y="10" width="380" height="180" fill="none" stroke="black" stroke-width="4"/>'
        '<text x="150" y="100">Kitchen</text></svg>')
    code, report, out = run(tmp_path, "drawing", "room.svg")
    assert code == 0, report
    assert report["info"]["labels"] == ["Kitchen"]
    assert Image.open(out / "ai.jpg").size == (inspect.AI_SIZE, inspect.AI_SIZE // 2)


@needs_resvg
def test_dxf(tmp_path):
    import ezdxf
    doc = ezdxf.new()
    doc.header["$INSUNITS"] = 6
    msp = doc.modelspace()
    msp.add_lwpolyline([(0, 0), (8, 0), (8, 5), (0, 5)], close=True, dxfattribs={"layer": "WALLS"})
    msp.add_text("Office", dxfattribs={"insert": (3, 2), "height": 0.4})
    doc.saveas(tmp_path / "plan.dxf")
    code, report, out = run(tmp_path, "drawing", "plan.dxf")
    assert code == 0, report
    info = report["info"]
    assert info["units"] == "meters"
    assert info["extent"]["size"] == [8, 5]
    assert "WALLS" in info["layers"]
    assert info["labels"] == [{"text": "Office", "at": [3, 2]}]
    w, h = Image.open(out / "ai.jpg").size
    assert w == inspect.AI_SIZE and 0.55 < h / w < 0.7


def test_mesh_obj_zip(tmp_path):
    folder = tmp_path / "input"
    folder.mkdir()
    box = trimesh.creation.box((400, 900, 300))  # millimeters, Y up
    box.export(folder / "chair.obj")
    (folder / "chair.mtl").write_text("newmtl wood\nmap_Kd wood.png\n")
    code, report, out = run(tmp_path, "mesh", "input/chair.obj")
    assert code == 0, report
    info = report["info"]
    assert info["size"] == [400, 900, 300]
    assert info["triangles"] == 12
    assert info["units_guess"] == "millimeters" and info["up_axis_guess"] == "y"
    assert info["missing_textures"] == ["wood.png"]
    assert {"preview.png", "ai.jpg", "thumb.jpg"} <= set(report["views"])
    assert Image.open(out / "preview.png").size == (768, 768)


def test_mesh_stl_large_is_simplified(tmp_path):
    sphere = trimesh.creation.icosphere(subdivisions=7)  # 327,680 faces
    sphere.export(tmp_path / "ball.stl")
    code, report, _ = run(tmp_path, "mesh", "ball.stl")
    assert code == 0, report
    assert report["info"]["triangles"] == len(sphere.faces)
    assert report["info"]["up_axis_guess"] == "z"


def test_bad_mesh(tmp_path):
    (tmp_path / "x.glb").write_bytes(b"glTF\x02\x00\x00\x00garbage")
    code, report, _ = run(tmp_path, "mesh", "x.glb")
    assert code == 2 and "can't be read" in report["error"]


def test_texture_set(tmp_path):
    folder = tmp_path / "input"
    folder.mkdir()
    for name, color in [("Bricks_Color.jpg", (150, 60, 40)), ("Bricks_NormalGL.png", (128, 128, 255)),
                        ("Bricks_Roughness.jpg", (200, 200, 200)), ("notes.jpg", (0, 0, 0))]:
        Image.new("RGB", (64, 64), color).save(folder / name)
    code, report, out = run(tmp_path, "texture", "input")
    assert code == 0, report
    assert report["info"]["maps"] == {"color": "Bricks_Color.jpg", "normal": "Bricks_NormalGL.png",
                                      "roughness": "Bricks_Roughness.jpg"}
    assert report["info"]["unused"] == ["notes.jpg"]
    r, g, b = Image.open(out / "ai.jpg").getpixel((10, 10))
    assert r > 120 and g < 90


def test_path_must_stay_in_job(tmp_path):
    code, report, _ = run(tmp_path, "mesh", "../../etc/passwd")
    assert code == 2 and "missing" in report["error"]
