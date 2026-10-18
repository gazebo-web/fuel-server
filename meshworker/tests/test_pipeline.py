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


def test_every_face_is_baked(build):
    """Thin, densely tessellated parts (a mug handle) produce triangles
    smaller than a texel; each must still sample a baked texel, not the
    black background."""
    # The agent's own mug from the first mesh smoke run: 418 of its 13094
    # faces sampled black before every triangle baked at least one texel.
    code, r, out = build("""
from build123d import *
import gzmesh as gz

def build():
    R, H, wall, base = 0.04, 0.095, 0.004, 0.006
    body = Cylinder(R, H, align=(Align.CENTER, Align.CENTER, Align.MIN))
    body = fillet(body.edges().group_by(Axis.Z)[0], 0.006)
    path = Spline((0, 0.035, 0.08), (0, 0.07, 0.077), (0, 0.083, 0.052), (0, 0.07, 0.027), (0, 0.035, 0.02))
    handle = sweep((path ^ 0) * Circle(0.0065), path)
    mug = (body + handle) - Pos(0, 0, base) * Cylinder(R - wall, H, align=(Align.CENTER, Align.CENTER, Align.MIN))
    return gz.part(mug, "mug", gz.plastic("#c8372d", roughness=0.2))
""")
    assert code == 0, r
    m = trimesh.load(out / "models/thing/meshes/thing.glb").geometry.popitem()[1]
    tex = np.asarray(m.visual.material.baseColorTexture.convert("RGB"))
    h, w = tex.shape[:2]
    c = m.visual.uv[m.faces].mean(1)
    px = tex[np.clip(((1 - c[:, 1]) * h).astype(int), 0, h - 1), np.clip((c[:, 0] * w).astype(int), 0, w - 1)]
    assert (px.max(1) < 40).sum() == 0, f"{(px.max(1) < 40).sum()} of {len(px)} faces sample unbaked texels"


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


LEVER = """
from build123d import *
import gzmesh as gz

def build():
    post = Box(0.2, 0.2, 1.0, align=(Align.CENTER, Align.CENTER, Align.MIN))
    arm = Pos(0.4, 0, 0.95) * Box(0.6, 0.05, 0.05)
    return gz.model(
        links=[gz.link("post", gz.part(post, "post", gz.concrete())),
               gz.link("arm", gz.part(arm, "arm", gz.painted_metal("red")))],
        joints=[gz.revolute("pivot", "post", "arm", origin=(0.1, 0, 0.95), axis=(0, 2, 0), lower=-3, upper=3)])
"""


def test_articulated_model(build):
    """Verified in Gazebo Harmonic: the arm swings about the pivot, keeping
    its distance to it (meshworker/spike/README.md)."""
    code, r, out = build(LEVER, mass=50)
    assert code == 0, r
    m = out / "models" / "thing"
    for f in ("meshes/thing_post.glb", "meshes/thing_arm.glb"):
        assert (m / f).is_file(), f
    assert [l["name"] for l in r["links"]] == ["post", "arm"]
    assert sum(l["inertial"]["mass_kg"] for l in r["links"]) == pytest.approx(50)
    # Steel arm vs concrete post: densities keep their ratio when scaled.
    post, arm = r["links"]
    assert arm["inertial"]["mass_kg"] / arm["inertial"]["volume_m3"] / (
        post["inertial"]["mass_kg"] / post["inertial"]["volume_m3"]) == pytest.approx(7850 / 2400, rel=1e-3)

    root = sdf(out, "thing")
    assert [l.get("name") for l in root.findall("model/link")] == ["post", "arm"]
    j = root.find("model/joint")
    assert j.get("type") == "revolute" and j.find("parent").text == "post" and j.find("child").text == "arm"
    # The model's bounding box spans x -0.1..0.7, so the origin moves by -0.3.
    assert [float(x) for x in j.find("pose").text.split()] == pytest.approx([-0.2, 0, 0.95, 0, 0, 0])
    assert [float(x) for x in j.find("axis/xyz").text.split()] == pytest.approx([0, 1, 0])
    assert float(j.find("axis/limit/lower").text) == -3 and float(j.find("axis/limit/upper").text) == 3
    assert r["joints"][0]["origin_m"] == pytest.approx([-0.2, 0, 0.95])
    for link in root.findall("model/link"):
        assert link.find("visual/geometry/mesh/uri").text == f"meshes/thing_{link.get('name')}.glb"


def test_continuous_and_prismatic_joints(build):
    code, r, out = build("""
from build123d import *
import gzmesh as gz

def build():
    body = Box(0.4, 0.3, 0.2, align=(Align.CENTER, Align.CENTER, Align.MIN))
    slider = Pos(0.25, 0, 0.1) * Box(0.1, 0.1, 0.1)
    wheel = Pos(0, 0.2, 0.1) * Rot(90, 0, 0) * Cylinder(0.08, 0.04)
    return gz.model(
        links=[gz.link("body", body), gz.link("slider", slider), gz.link("wheel", wheel)],
        joints=[gz.prismatic("slide", "body", "slider", origin=(0.2, 0, 0.1), axis=(1, 0, 0), lower=0, upper=0.2),
                gz.continuous("axle", "body", "wheel", origin=(0, 0.2, 0.1), axis=(0, 1, 0))])
""")
    assert code == 0, r
    joints = {j.get("name"): j for j in sdf(out, "thing").findall("model/joint")}
    assert joints["slide"].get("type") == "prismatic"
    assert joints["axle"].get("type") == "revolute"  # unlimited, as Gazebo's diff_drive example does
    assert float(joints["axle"].find("axis/limit/upper").text) > 1e300
    assert r["warnings"] == []


@pytest.mark.parametrize("joints, error", [
    ('[gz.revolute("j", "a", "nope", (0, 0, 0), (0, 0, 1), 0, 1)]', "no link named 'nope'"),
    ('[]', "unconnected roots"),
    ('[gz.revolute("j", "a", "b", (0, 0, 0), (0, 0, 1), 0, 1), gz.revolute("k", "b", "a", (0, 0, 0), (0, 0, 1), 0, 1)]',
     "unconnected roots"),
    ('[gz.revolute("j", "a", "b", (0, 0, 0), (0, 0, 1), 1, 0)]', "lower (1.0) must be less than upper"),
    ('[gz.revolute("j", "a", "b", (0, 0, 0), (0, 0, 0), 0, 1)]', "axis must not be zero"),
    ('[gz.revolute("j", "a", "b", (0, 0, 0), (0, 0, 1), 0, 7)]', "use gz.continuous"),
    ('[gz.revolute("j", "a", "b", (0, 0, 0), (0, 0, 1), 0, 1), gz.revolute("j", "a", "b", (0, 0, 0), (0, 0, 1), 0, 1)]',
     "joint names must be unique"),
])
def test_joint_errors(build, joints, error):
    code, r, _ = build(f"""
from build123d import *
import gzmesh as gz

def build():
    a, b = gz.link("a", Box(1, 1, 1)), gz.link("b", Pos(2, 0, 0) * Box(1, 1, 1))
    return gz.model(links=[a, b], joints={joints})
""")
    assert code == 2 and error in r["error"], r


def test_far_joint_origin_warns(build):
    code, r, _ = build(LEVER.replace("origin=(0.1, 0, 0.95)", "origin=(100, 0, 950)"))
    assert code == 0 and any("far from links" in w for w in r["warnings"]), r["warnings"]


def test_ambient_occlusion_darkens_enclosed_surfaces(build):
    """An open bin's inside corners are occluded; a plain cube has nothing
    to occlude it. Occlusion is baked into the base color and stored as
    1 - occlusion in the ORM red channel."""
    def orm_red(code):
        status, r, out = build(code)
        assert status == 0, r
        m = trimesh.load(out / "models/thing/meshes/thing.glb").geometry.popitem()[1]
        red = np.asarray(m.visual.material.metallicRoughnessTexture.convert("RGB"))[..., 0]
        return red[red > 0]  # baked texels; the background between charts is 0
    bin_ = orm_red("""
from build123d import *
def build():
    b = Box(0.4, 0.4, 0.4, align=(Align.CENTER, Align.CENTER, Align.MIN))
    return offset(b, amount=-0.01, openings=b.faces().sort_by(Axis.Z)[-1])
""")
    cube = orm_red("from build123d import *\ndef build():\n    return Box(0.4, 0.4, 0.4)\n")
    assert (bin_ < 160).mean() > 0.02, "inside corners should be strongly occluded"
    assert (cube < 240).mean() < 0.001, "a convex cube occludes nothing"


def test_preview_shows_joints_moved(build):
    """For an articulated model the third tile is the first view's camera
    with the joints moved: the swung arm makes the two tiles differ."""
    from PIL import Image
    code, r, out = build(LEVER)
    assert code == 0, r
    sheet = np.asarray(Image.open(out / "preview.png").convert("L"), float)
    t = sheet.shape[0] // 2
    rest, moved = sheet[20:t, :t], sheet[t + 20:, :t]  # skip the labels
    assert np.abs(rest - moved).mean() > 2


ROVER = """
from build123d import *
import gzmesh as gz

def build():
    body = Pos(0, 0, 0.12) * Box(0.4, 0.28, 0.1)
    caster = Pos(-0.15, 0, 0.035) * Sphere(0.035)
    links = [gz.link("chassis", body), gz.link("caster", caster, friction=0)]
    joints = [gz.fixed("caster_mount", "chassis", "caster")]
    for side, y in (("left", 0.17), ("right", -0.17)):
        links.append(gz.link(f"wheel_{side}", Pos(0.05, y, 0.07) * Rot(90, 0, 0) * Cylinder(0.07, 0.04)))
        joints.append(gz.continuous(f"wheel_{side}_joint", "chassis", f"wheel_{side}",
                                    origin=(0.05, y, 0.07), axis={"left": AXIS_L, "right": AXIS_R}[side]))
    return gz.model(links=links, joints=joints,
                    sensors=[gz.lidar("lidar", "chassis", pos=(0, 0, 0.18), samples=90, fov=1.5),
                             gz.camera("cam", "chassis", pos=CAM, rpy=(0, 0.2, 0)),
                             gz.depth_camera("depth", "chassis", pos=(0.2, 0.05, 0.12)),
                             gz.imu("imu", "chassis", pos=(0, 0, 0.12))],
                    plugins=[gz.diff_drive(LEFT, RIGHT), gz.joint_states()])
"""


def rover(cam=(0.201, 0, 0.14), left="wheel_left_joint", right="wheel_right_joint", axis_left=(0, 1, 0),
          axis_right=(0, 1, 0)):
    return (ROVER.replace("CAM", repr(cam)).replace("LEFT", repr(left)).replace("RIGHT", repr(right))
            .replace("AXIS_L", repr(axis_left)).replace("AXIS_R", repr(axis_right)))


def test_robot_sensors_and_plugins(build):
    code, r, out = build(rover())
    assert code == 0, r
    assert r["warnings"] == [], r["warnings"]
    m = sdf(out, "thing").find("model")
    sensors = {s.get("name"): s for s in m.findall("link[@name='chassis']/sensor")}
    assert {n: s.get("type") for n, s in sensors.items()} == {
        "lidar": "gpu_lidar", "cam": "camera", "depth": "depth_camera", "imu": "imu"}
    assert sensors["cam"].find("pose").text == "0.201 0 0.14 0 0.2 0"
    assert sensors["lidar"].find("lidar/scan/horizontal/samples").text == "90"
    assert sensors["lidar"].find("lidar/scan/horizontal/max_angle").text == "0.75"
    assert sensors["depth"].find("camera/image/format").text == "R_FLOAT32"
    assert m.find("link[@name='caster']/collision/surface/friction/ode/mu").text == "0"
    assert m.find("link[@name='chassis']/collision/surface") is None

    dd = m.find("plugin[@name='gz::sim::systems::DiffDrive']")
    assert dd.find("left_joint").text == "wheel_left_joint" and dd.find("right_joint").text == "wheel_right_joint"
    assert float(dd.find("wheel_separation").text) == pytest.approx(0.34)
    assert float(dd.find("wheel_radius").text) == pytest.approx(0.07, abs=2e-3)
    assert m.find("plugin[@name='gz::sim::systems::JointStatePublisher']") is not None

    by = {s["name"]: s for s in r["sensors"]}
    assert by["cam"]["topics"][0] == "/world/<world>/model/<instance>/link/chassis/sensor/cam/image"
    assert by["cam"]["looks_toward"][0] > 0.95 and by["cam"]["looks_toward"][2] < 0  # pitched down
    assert "looks_toward" not in by["imu"]
    assert r["plugins"][0]["cmd_vel"] == "/model/<instance>/cmd_vel"
    assert [p.split('"')[1] for p in r["world_systems"]["plugins"]] == ["gz-sim-sensors-system", "gz-sim-imu-system"]


def test_preview_marks_sensors(build):
    from PIL import Image
    _, _, out = build(rover().replace("sensors=[", "sensors=[] and ["))
    a = np.asarray(Image.open(out / "preview.png").convert("RGB"), int)
    _, _, out = build(rover())
    b = np.asarray(Image.open(out / "preview.png").convert("RGB"), int)
    red = (b[..., 0] > 200) & (b[..., 1] < 80) & (b[..., 2] < 80)  # the lidar marker
    assert red.sum() > 30 and ((a[..., 0] > 200) & (a[..., 1] < 80) & (a[..., 2] < 80)).sum() == 0


@pytest.mark.parametrize("kw, warning", [
    ({"cam": (0, 0, 0.12)}, "sensor 'cam' is inside link 'chassis'"),
    ({"cam": (5, 0, 0.12)}, "sensor 'cam': pos [5.0, 0.0, 0.12] is far from link 'chassis'"),
    ({"left": "wheel_right_joint", "right": "wheel_left_joint"}, "aren't on the +Y side"),
    ({"axis_right": (0, -1, 0)}, "joint 'wheel_right_joint' axis is [0.0, -1.0, 0.0]"),
])
def test_robot_warnings(build, kw, warning):
    code, r, _ = build(rover(**kw))
    assert code == 0 and any(warning in w for w in r["warnings"]), r["warnings"]


@pytest.mark.parametrize("change, error", [
    (("gz.imu(\"imu\", \"chassis\"", "gz.imu(\"imu\", \"nope\""), "sensor 'imu': no link named 'nope'"),
    (("gz.imu(\"imu\"", "gz.imu(\"cam\""), "sensor names must be unique"),
    (("samples=90", "samples=0"), "sensor 'lidar' samples must be a number from 1"),
    (("gz.diff_drive(", "gz.diff_drive(\"caster_mount\", "), "joint 'caster_mount' is fixed; wheels need"),
    (("gz.joint_states()", "gz.joint_states(), gz.joint_states()"), "at most one gz.joint_states()"),
    (("plugins=[", "plugins=[gz.position_controller(\"nope\"), "), "position_controller: no joint named 'nope'"),
    (("friction=0", "friction=-1"), "friction must be a number from 0 to 10"),
])
def test_robot_errors(build, change, error):
    code_ = rover().replace(*change)
    if "gz.diff_drive(\"caster_mount\"" in code_:
        code_ = code_.replace("'wheel_left_joint', 'wheel_right_joint'", "'wheel_right_joint'")
    code, r, _ = build(code_)
    assert code == 2 and error in r["error"], r
