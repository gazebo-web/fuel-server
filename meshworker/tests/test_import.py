"""gzmesh.import_mesh: uploaded meshes to models."""

import json
import xml.etree.ElementTree as ET

import numpy as np
import pytest
import trimesh
from PIL import Image

from gzmesh import import_mesh


def run(tmp_path, **job):
    job.setdefault("name", "thing")
    path = tmp_path / "job.json"
    path.write_text(json.dumps(job))
    out = tmp_path / "out"
    code = import_mesh.main([str(path), str(out)])
    return code, json.loads((out / "report.json").read_text()), out


def test_obj_in_millimeters_y_up(tmp_path):
    (tmp_path / "input").mkdir()
    trimesh.creation.box((400, 900, 300)).export(tmp_path / "input" / "cabinet.obj")  # mm, Y up
    code, report, out = run(tmp_path, name="cabinet", path="input/cabinet.obj", source_name="cabinet.obj")
    assert code == 0, report
    assert report["bounding_box_m"] == [0.4, 0.3, 0.9]
    assert report["import"]["how"] == "units mm (guessed)" and report["import"]["up_axis"] == "y"
    assert report["collision"]["primitive"] == "box"
    assert report["inertial"]["mass_kg"] == pytest.approx(108, rel=1e-3)
    assert any("no mass given" in w for w in report["warnings"])
    root = out / "models" / "cabinet"
    for f in ("model.sdf", "model.config", "meshes/cabinet.glb", "source/import.json"):
        assert (root / f).is_file(), f
    assert json.loads((root / "source/import.json").read_text())["imported_from"] == "cabinet.obj"
    sdf = (root / "model.sdf").read_text()
    assert "Imported by the AI Builder from the project file cabinet.obj" in sdf
    ET.fromstring(sdf.split("\n", 1)[1])  # well-formed after the XML declaration
    glb = trimesh.load(root / "meshes" / "cabinet.glb", force="scene")
    assert np.allclose(glb.bounds, [[-0.2, -0.15, 0], [0.2, 0.15, 0.9]], atol=1e-6)
    assert (out / "preview.png").is_file()


def test_height_and_mass(tmp_path):
    trimesh.creation.icosphere(radius=1).export(tmp_path / "ball.stl")
    code, report, _ = run(tmp_path, path="ball.stl", height=0.5, mass=2.0)
    assert code == 0, report
    assert report["bounding_box_m"] == pytest.approx([0.5, 0.5, 0.5], abs=1e-3)
    assert report["import"]["up_axis"] == "z"
    assert report["inertial"]["mass_kg"] == pytest.approx(2.0)
    assert not any("no mass" in w for w in report["warnings"])


def test_textured_obj_keeps_its_texture(tmp_path):
    d = tmp_path / "input"
    d.mkdir()
    Image.new("RGB", (8, 8), (200, 30, 30)).save(d / "red.png")
    box = trimesh.creation.box((1, 1, 1))
    uv = (box.vertices[:, :2] + 0.5)
    box.visual = trimesh.visual.TextureVisuals(uv=uv, material=trimesh.visual.material.SimpleMaterial(
        image=Image.open(d / "red.png")))
    box.export(d / "crate.obj")
    code, report, out = run(tmp_path, name="crate", path="input/crate.obj", units="m")
    assert code == 0, report
    glb = trimesh.load(out / "models" / "crate" / "meshes" / "crate.glb", force="scene")
    mat = next(iter(glb.geometry.values())).visual.material
    tex = mat.baseColorTexture
    assert tex is not None and tex.convert("RGB").getpixel((4, 4))[0] > 150


def test_simplified_to_budget(tmp_path):
    trimesh.creation.icosphere(subdivisions=7, radius=0.3).export(tmp_path / "scan.stl")
    code, report, _ = run(tmp_path, path="scan.stl", tri_budget=5000, static=True)
    assert code == 0, report
    assert report["triangles"] <= 5300
    assert report["import"]["triangles_before"] == 327680
    assert any("simplified" in w for w in report["warnings"])
    assert not any("no mass" in w for w in report["warnings"]), "static models need no mass"


def test_open_mesh_uses_its_hull(tmp_path):
    shell = trimesh.creation.cylinder(radius=0.2, height=0.4)
    shell.update_faces(np.abs(shell.face_normals[:, 2]) < 0.5)  # drop the caps: an open tube
    shell.export(tmp_path / "tube.stl")
    code, report, _ = run(tmp_path, path="tube.stl", units="m")
    assert code == 0, report
    assert report["import"]["closed"] is False
    assert any("convex hull" in w for w in report["warnings"])
    assert report["inertial"]["mass_kg"] > 0


@pytest.mark.parametrize("job, msg", [
    ({"name": "Bad Name", "path": "x.stl"}, "name must be"),
    ({"path": "x.stl", "scale": 2, "height": 1}, "scale or height"),
    ({"path": "x.stl", "units": "furlongs"}, "units must be"),
    ({"path": "../x.stl"}, "missing"),
])
def test_bad_jobs(tmp_path, job, msg):
    trimesh.creation.box().export(tmp_path / "x.stl")
    code, report, _ = run(tmp_path, **job)
    assert code == 2 and msg in report["error"], report
