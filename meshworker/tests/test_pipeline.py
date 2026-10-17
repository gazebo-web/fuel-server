"""End-to-end jobs through gzmesh.run against the output contract (plan §5.4)."""

import xml.etree.ElementTree as ET

import numpy as np
import pytest
import trimesh

BOX = """
from build123d import *
import gzmesh as gz

def build():
    return gz.part(Pos(0, 0, 0.25) * Box(1, 2, 0.5), "body", gz.flat("#3366cc"))
"""

CONE = """
from build123d import *
import gzmesh as gz

def build():
    base = Pos(0, 0, 0.015) * Box(0.36, 0.36, 0.03)
    cone = Pos(0, 0, 0.315) * Cone(0.14, 0.025, 0.57)
    return [gz.part(base, "base", gz.rubber()),
            gz.part(cone, "cone", gz.stripes(gz.plastic("orange"), "white", [(0.33, 0.45)]))]
"""


def sdf(out, name):
    return ET.parse(out / "models" / name / "model.sdf").getroot()


def test_box_contract(build):
    code, r, out = build(BOX, density=1000)
    assert code == 0 and r["ok"], r
    m = out / "models" / "thing"
    for f in ("model.sdf", "model.config", "meshes/thing.glb", "source/model.py"):
        assert (m / f).is_file(), f
    assert (m / "source/model.py").read_text() == BOX
    assert (out / "preview.png").is_file()

    # One mesh with one PBR material, base color and ORM textures.
    scene = trimesh.load(m / "meshes/thing.glb")
    meshes = list(scene.geometry.values())
    assert len(meshes) == 1
    mat = meshes[0].visual.material
    assert mat.baseColorTexture is not None and mat.metallicRoughnessTexture is not None
    # Origin at the bottom center; Z up; meters.
    lo, hi = meshes[0].bounds
    assert np.allclose(lo, [-0.5, -1, 0], atol=1e-6) and np.allclose(hi, [0.5, 1, 0.5], atol=1e-6)
    assert r["bounding_box_m"] == [1.0, 2.0, 0.5]

    # Exact inertia: a 1 x 2 x 0.5 m box at 1000 kg/m³.
    mass = 1000.0
    root = sdf(out, "thing")
    inertial = root.find("model/link/inertial")
    assert float(inertial.find("mass").text) == pytest.approx(mass)
    assert [float(x) for x in inertial.find("pose").text.split()] == pytest.approx([0, 0, 0.25, 0, 0, 0], abs=1e-9)
    I = inertial.find("inertia")
    assert float(I.find("ixx").text) == pytest.approx(mass * (4 + 0.25) / 12, rel=1e-5)
    assert float(I.find("iyy").text) == pytest.approx(mass * (1 + 0.25) / 12, rel=1e-5)
    assert float(I.find("izz").text) == pytest.approx(mass * (1 + 4) / 12, rel=1e-5)

    # A box is its own best primitive.
    assert r["collision"]["primitive"] == "box" and r["collision"]["primitive_fill"] == pytest.approx(1, abs=1e-3)
    box = root.find("model/link/collision/geometry/box/size")
    assert [float(x) for x in box.text.split()] == pytest.approx([1, 2, 0.5], abs=1e-6)


def test_products_of_inertia_sign(build):
    """An L-shape has a nonzero Ixy; compare with trimesh, whose tensor uses
    the same convention as SDF (I_xy = -∫ x y dm). Both arms lie on
    opposite sides of the center of mass in x and y, so ∫ x y dm < 0 and
    I_xy > 0."""
    code, r, out = build("""
from build123d import *
import gzmesh as gz

def build():
    return [gz.part(Pos(0.5, 0.1, 0.1) * Box(1, 0.2, 0.2), "a"),
            gz.part(Pos(0.1, 0.6, 0.1) * Box(0.2, 0.8, 0.2), "b")]
""", density=1000)
    assert code == 0, r
    I = sdf(out, "thing").find("model/link/inertial/inertia")
    ixy = float(I.find("ixy").text)
    m = trimesh.util.concatenate([trimesh.creation.box((1, 0.2, 0.2), trimesh.transformations.translation_matrix((0.5, 0.1, 0.1))),
                                  trimesh.creation.box((0.2, 0.8, 0.2), trimesh.transformations.translation_matrix((0.1, 0.6, 0.1)))])
    m.density = 1000
    assert ixy == pytest.approx(m.moment_inertia[0, 1], rel=1e-4)
    assert ixy > 0


def test_cone_textures_and_decomposition(build):
    code, r, out = build(CONE, mass=2.5)
    assert code == 0, r
    assert r["inertial"]["mass_kg"] == pytest.approx(2.5)
    assert r["bounding_box_m"] == pytest.approx([0.36, 0.36, 0.6], abs=1e-4)
    # Neither a primitive nor one hull fits a cone on a base well.
    c = r["collision"]
    assert c["primitive_fill"] < 0.9 and c["hull_fill"] < 0.85 and 1 < c["hulls"] <= 16
    for s in c["shapes"]:
        assert s["kind"] == "mesh" and s["vertices"] <= 255
    files = [f for f in r["files"] if "_collision_" in f]
    assert len(files) == c["hulls"]
    # The band: texels on the cone at z = 0.39 are white, at z = 0.2 orange.
    tex = np.asarray(trimesh.load(out / "models/thing/meshes/thing.glb").geometry.popitem()[1]
                     .visual.material.baseColorTexture.convert("RGB"))
    assert (tex.reshape(-1, 3) > 230).all(1).any() and ((tex[..., 0] > 180) & (tex[..., 2] < 80)).any()


def test_sphere_and_cylinder_primitives(build):
    code, r, _ = build("from build123d import *\ndef build():\n    return Sphere(0.3)\n")
    assert code == 0 and r["collision"]["primitive"] == "sphere", r["collision"]
    code, r, out = build("from build123d import *\ndef build():\n    return Cylinder(0.1, 1.0)\n")
    assert code == 0 and r["collision"]["primitive"] == "cylinder", r["collision"]
    cyl = sdf(out, "thing").find("model/link/collision/geometry/cylinder")
    assert float(cyl.find("radius").text) == pytest.approx(0.1, rel=0.02)
    assert float(cyl.find("length").text) == pytest.approx(1.0, rel=0.02)


def test_triangle_budget(build):
    code, r, _ = build("from build123d import *\ndef build():\n    return Sphere(0.5) - Pos(0.3, 0, 0) * Sphere(0.3)\n",
                       tri_budget=300)
    assert code == 0, r
    assert r["triangles"] <= 300
    assert r["tessellation"]["triangles"] == r["triangles"]


def test_tolerance_reaches_budget_without_decimation(build):
    """A torus at the default tolerance is ~95k triangles; loosening the
    tolerance must reach a small budget (OCCT's cached triangulation once
    made every pass return the first, finest one)."""
    code, r, _ = build("from build123d import *\ndef build():\n    return Torus(0.14, 0.03)\n", tri_budget=3000)
    assert code == 0, r
    assert r["triangles"] <= 3000 and not r["tessellation"]["decimated"], r["tessellation"]


def test_static_and_names(build):
    code, r, out = build(BOX, name="crate_2", static=True)
    assert code == 0 and r["uri"] == "model://crate_2"
    assert sdf(out, "crate_2").find("model/static").text == "true"


@pytest.mark.parametrize("code_, error, line", [
    ("import os\ndef build():\n    pass\n", "import 'os' is not allowed", 1),
    ("from build123d import *\ndef build():\n    return Box(1, 1\n", "syntax error", 3),
    ("from build123d import *\ndef make():\n    return Box(1, 1, 1)\n", "must define build()", None),
    ("from build123d import *\n\ndef build():\n    x = 1 / 0\n", "ZeroDivisionError", 4),
    ("def build():\n    return open('/etc/passwd')\n", "open() is not available", 2),
    ("def build():\n    return ().__class__\n", "dunder attribute", 2),
    ("from build123d import *\ndef build():\n    return Circle(1)\n", "has no volume", None),
    ("import gzmesh as gz\nfrom build123d import *\ndef build():\n    return gz.part(Box(1, 1, 1), 'b', 'red')\n",
     "material must come from gzmesh", 4),
])
def test_code_errors_are_reported_with_lines(build, code_, error, line):
    code, r, _ = build(code_)
    assert code == 2 and not r["ok"], r
    assert error in r["error"], r["error"]
    if line is not None:
        assert r["line"] == line


@pytest.mark.parametrize("job, error", [
    ({"name": "Bad-Name"}, "name must be"),
    ({"tri_budget": 50}, "tri_budget"),
    ({"texture_size": 1000}, "power of two"),
    ({"mass": 1, "density": 2}, "mass or density"),
    ({"collision": "fancy"}, "collision 'fancy'"),
])
def test_job_errors(build, job, error):
    code, r, _ = build(BOX, **job)
    assert code == 2 and error in r["error"], r
