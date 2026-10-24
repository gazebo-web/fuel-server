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


@needs_resvg
def test_tall_svg_is_bounded(tmp_path):
    (tmp_path / "tall.svg").write_text('<svg xmlns="http://www.w3.org/2000/svg" width="10" height="20000">'
                                       '<rect width="10" height="20000"/></svg>')
    code, report, out = run(tmp_path, "drawing", "tall.svg")
    assert code == 0, report
    w, h = Image.open(out / "ai.jpg").size
    assert h == inspect.AI_SIZE and w < 10


@needs_resvg
def test_empty_dxf(tmp_path):
    import ezdxf
    ezdxf.new().saveas(tmp_path / "empty.dxf")
    code, report, _ = run(tmp_path, "drawing", "empty.dxf")
    assert code == 2 and "empty" in report["error"], report


@pytest.mark.parametrize("name, kind", [
    ("Metal032_2K-JPG_Color.jpg", "color"), ("metal_plate_diff_1k.jpg", "color"),
    ("Metal032_2K-JPG_Metalness.jpg", "metalness"), ("Bricks_NormalGL.png", "normal"),
    ("brick_rough_4k.png", "roughness"), ("Wood_AO.png", "ao"), ("BricksNormal.png", "normal"),
    ("wood_basecolor.png", "color"), ("readme.jpg", None),
])
def test_map_names(name, kind):
    assert inspect.map_kind(name) == kind


def test_texture_set_ignores_mac_leftovers(tmp_path):
    d = tmp_path / "input"
    (d / "__MACOSX").mkdir(parents=True)
    (d / "__MACOSX" / "._wood_color.jpg").write_bytes(b"\x00\x05\x16\x07")
    (d / "._wood_color.jpg").write_bytes(b"\x00\x05\x16\x07")
    Image.new("RGB", (16, 16), (120, 80, 40)).save(d / "wood_color.jpg")
    code, report, _ = run(tmp_path, "texture", "input")
    assert code == 0, report
    assert report["info"]["maps"] == {"color": "wood_color.jpg"}


MODEL_SDF = """<?xml version="1.0"?>
<sdf version="1.9"><model name="cart">
  <link name="base"><pose>0 0 0.25 0 0 0</pose>
    <visual name="body"><geometry><box><size>1 0.6 0.5</size></box></geometry>
      <material><diffuse>0.9 0.1 0.1 1</diffuse></material></visual>
    <visual name="handle"><pose>0.5 0 0.5 0 0 0</pose>
      <geometry><mesh><uri>model://cart/meshes/tri.dae</uri><scale>2 2 2</scale></mesh></geometry></visual>
    <visual name="lost"><geometry><mesh><uri>model://other/meshes/x.dae</uri></mesh></geometry></visual>
  </link>
</model></sdf>"""


def model_folder(tmp_path, sdf=MODEL_SDF):
    from test_measure import DAE

    root = tmp_path / "input" / "cart"
    (root / "meshes").mkdir(parents=True)
    (root / "model.config").write_text("<model><name>cart</name><sdf version='1.9'>model.sdf</sdf></model>")
    (root / "model.sdf").write_text(sdf)
    (root / "meshes" / "tri.dae").write_text(DAE)
    return root


def test_model(tmp_path):
    model_folder(tmp_path)
    code, report, out = run(tmp_path, "model", "input")
    assert code == 0, report
    info = report["info"]
    assert info["sdf"] == "model.sdf" and info["drawn_from"] == "visual" and info["drawn"] == 2
    assert any("model://other" in w for w in info["preview_warnings"])
    assert {"preview.png", "ai.jpg", "thumb.jpg"} <= set(report["views"])
    # The body's red shows in the views.
    px = np.asarray(Image.open(out / "preview.png").convert("RGB")).reshape(-1, 3).astype(int)
    assert ((px[:, 0] > 150) & (px[:, 1] < 80) & (px[:, 2] < 80)).sum() > 1000


def test_model_collada_units_and_pose(tmp_path):
    from gzmesh import measure

    root, _, model = measure.load_model(str(model_folder(tmp_path).parent))
    shapes = {where.rsplit("/", 1)[1]: (el, t) for el, t, where in measure.shapes(model, "visual", [])}
    el, t = shapes["handle"]
    got = inspect.shape_meshes(el.find("geometry"), root, inspect.GREY, t, {}, [], "handle")
    # The triangle in meters (0,0,0.1) (2,0,0.1) (0,1,0.6), scaled by 2,
    # moved up 0.25 (link) and by (0.5, 0, 0.5) (visual).
    assert len(got) == 1
    assert np.allclose(got[0].bounds, [[0.5, 0, 0.95], [4.5, 2, 1.95]])


def test_model_collisions_when_no_visuals(tmp_path):
    sdf = ("<sdf version='1.9'><model name='cart'><link name='l'><collision name='c'>"
           "<geometry><cylinder><radius>0.2</radius><length>1</length></cylinder></geometry>"
           "</collision></link></model></sdf>")
    model_folder(tmp_path, sdf)
    code, report, _ = run(tmp_path, "model", "input")
    assert code == 0, report
    assert report["info"]["drawn_from"] == "collision"


def test_model_with_nothing_to_draw(tmp_path):
    sdf = ("<sdf version='1.9'><model name='cart'><link name='l'><visual name='v'>"
           "<geometry><mesh><uri>model://gone/m.dae</uri></mesh></geometry></visual></link></model></sdf>")
    model_folder(tmp_path, sdf)
    code, report, _ = run(tmp_path, "model", "input")
    assert code == 2 and "nothing that can be drawn" in report["error"]


# A textured triangle whose material points straight at its image, as
# 3ds Max writes Collada.
DIRECT_TEXTURE_DAE = """<?xml version="1.0"?>
<COLLADA xmlns="http://www.collada.org/2005/11/COLLADASchema" version="1.4.1">
  <asset><unit meter="1"/><up_axis>Z_UP</up_axis></asset>
  <library_images><image id="Map #1-image"><init_from>../materials/textures/red.png</init_from></image></library_images>
  <library_materials><material id="m"><instance_effect url="#fx"/></material></library_materials>
  <library_effects><effect id="fx"><profile_COMMON><technique sid="common"><phong>
    <diffuse><texture texture="Map #1-image" texcoord="CHANNEL0"/></diffuse>
  </phong></technique></profile_COMMON></effect></library_effects>
  <library_geometries><geometry id="g"><mesh>
    <source id="p"><float_array id="pa" count="9">0 0 0 1 0 0 0 1 0</float_array>
      <technique_common><accessor source="#pa" count="3" stride="3">
        <param name="X" type="float"/><param name="Y" type="float"/><param name="Z" type="float"/>
      </accessor></technique_common></source>
    <source id="uv"><float_array id="uva" count="6">0 0 1 0 0 1</float_array>
      <technique_common><accessor source="#uva" count="3" stride="2">
        <param name="S" type="float"/><param name="T" type="float"/>
      </accessor></technique_common></source>
    <vertices id="v"><input semantic="POSITION" source="#p"/></vertices>
    <triangles count="1" material="mat"><input semantic="VERTEX" source="#v" offset="0"/>
      <input semantic="TEXCOORD" source="#uv" offset="0" set="0"/><p>0 1 2</p></triangles>
  </mesh></geometry></library_geometries>
  <library_visual_scenes><visual_scene id="s"><node id="n"><instance_geometry url="#g"><bind_material>
    <technique_common><instance_material symbol="mat" target="#m">
      <bind_vertex_input semantic="CHANNEL0" input_semantic="TEXCOORD" input_set="0"/>
    </instance_material></technique_common></bind_material></instance_geometry></node></visual_scene>
  </library_visual_scenes>
  <scene><instance_visual_scene url="#s"/></scene>
</COLLADA>"""


def test_model_collada_texture_without_sampler(tmp_path):
    root = tmp_path / "cart"
    (root / "meshes").mkdir(parents=True)
    (root / "materials" / "textures").mkdir(parents=True)
    (root / "meshes" / "tri.dae").write_text(DIRECT_TEXTURE_DAE)
    Image.new("RGB", (4, 4), (220, 20, 20)).save(root / "materials" / "textures" / "red.png")
    geom = inspect.ET.fromstring("<geometry><mesh><uri>model://cart/meshes/tri.dae</uri></mesh></geometry>")
    got = inspect.shape_meshes(geom, str(root), inspect.GREY, np.eye(4), {}, [], "v")
    assert len(got) == 1
    tex = got[0].visual.material.baseColorTexture.convert("RGB")
    assert tex.getpixel((0, 0)) == (220, 20, 20)
    # Files outside the model stay out of reach.
    (tmp_path / "secret.png").write_bytes(b"x")
    with pytest.raises(ValueError):
        inspect.model_resolver(str(root / "meshes" / "tri.dae"), str(root)).get("../../secret.png")
