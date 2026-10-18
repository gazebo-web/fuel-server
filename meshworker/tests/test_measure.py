"""gzmesh.measure: bounding boxes of Fuel models, as Gazebo shows them."""

import json
import math

import pytest
import trimesh

from gzmesh import measure

CONFIG = """<?xml version="1.0"?>
<model><name>thing</name>
  <sdf version="1.4">model-1_4.sdf</sdf>
  <sdf version="1.6">model.sdf</sdf>
</model>"""

# A 2 x 2 x 2 cube in centimeters, Y up; Gazebo applies the unit and
# ignores the up axis.
DAE = """<?xml version="1.0"?>
<COLLADA xmlns="http://www.collada.org/2005/11/COLLADASchema" version="1.4.1">
  <asset><unit name="centimeter" meter="0.01"/><up_axis>Y_UP</up_axis></asset>
  <library_geometries><geometry id="g"><mesh>
    <source id="p"><float_array id="pa" count="9">0 0 0 200 0 0 0 100 50</float_array>
      <technique_common><accessor source="#pa" count="3" stride="3">
        <param name="X" type="float"/><param name="Y" type="float"/><param name="Z" type="float"/>
      </accessor></technique_common></source>
    <vertices id="v"><input semantic="POSITION" source="#p"/></vertices>
    <triangles count="1"><input semantic="VERTEX" source="#v" offset="0"/><p>0 1 2</p></triangles>
  </mesh></geometry></library_geometries>
  <library_visual_scenes><visual_scene id="s"><node id="n">
    <translate>0 0 10</translate>
    <instance_geometry url="#g"/>
  </node></visual_scene></library_visual_scenes>
  <scene><instance_visual_scene url="#s"/></scene>
</COLLADA>"""


def write(tmp_path, sdf_body, files=None, config=True, folder="thing"):
    root = tmp_path / "unzipped" / folder
    root.mkdir(parents=True)
    if config:
        (root / "model.config").write_text(CONFIG)
        (root / "model-1_4.sdf").write_text("<sdf version='1.4'><model name='old'/></sdf>")
    (root / "model.sdf").write_text(f"<sdf version='1.6'><model name='thing'>{sdf_body}</model></sdf>")
    for rel, data in (files or {}).items():
        p = root / rel
        p.parent.mkdir(parents=True, exist_ok=True)
        if isinstance(data, bytes):
            p.write_bytes(data)
        else:
            p.write_text(data)
    return tmp_path / "unzipped"


def visual(geometry, pose="", name="v", kind="visual"):
    p = f"<pose>{pose}</pose>" if pose else ""
    return f"<{kind} name='{name}'>{p}<geometry>{geometry}</geometry></{kind}>"


def approx(v, want, tol=1e-3):
    return all(abs(a - b) <= tol for a, b in zip(v, want)) and len(v) == len(want)


def test_primitives_and_poses(tmp_path):
    sdf = (
        "<pose>5 5 5 0 0 1</pose>"  # where a world puts it: not applied
        "<link name='base'><pose>0 0 0.5 0 0 0</pose>"
        + visual("<box><size>1 2 1</size></box>")
        + visual("<cylinder><radius>0.5</radius><length>2</length></cylinder>", "0 0 1 1.5707963 0 0", "c")
        + "</link>"
        "<link name='ball'><pose>3 0 0 0 0 0</pose>"
        + visual("<sphere><radius>0.25</radius></sphere>") + "</link>"
    )
    r = measure.measure(str(write(tmp_path, sdf)))
    assert r["ok"] and r["sdf"] == "model.sdf" and r["from"] == "visual"
    # The cylinder lies along Y (length 2) centered at z = 1.5; the ball
    # sits at z = 0.
    assert approx(r["bbox"]["min"], [-0.5, -1, -0.25])
    assert approx(r["bbox"]["max"], [3.25, 1, 2])
    assert r["visuals"] == 3 and r["measured"] == 3 and not r["warnings"]


def test_collada_units_and_nested_model(tmp_path):
    sdf = (
        "<model name='inner'><pose>1 0 0 0 0 1.5707963</pose><link name='l'>"
        + visual("<mesh><uri>model://thing/meshes/tri.dae</uri><scale>2 2 2</scale></mesh>")
        + "</link></model>"
    )
    r = measure.measure(str(write(tmp_path, sdf, {"meshes/tri.dae": DAE})))
    # Triangle (0,0,0.1) (2,0,0.1) (0,1,0.6) in meters, scaled by 2, then
    # turned 90 degrees about Z and moved 1 along X.
    assert approx(r["size"], [2, 4, 1])
    assert approx(r["bbox"]["min"], [-1, 0, 0.2]) and approx(r["bbox"]["max"], [1, 4, 1.2])


def test_mesh_formats_and_fuel_urls(tmp_path):
    box = trimesh.creation.box((1, 2, 3))
    sdf = (
        "<link name='l'>"
        + visual("<mesh><uri>https://fuel.gazebosim.org/1.0/o/models/thing/tip/files/meshes/BOX.STL</uri></mesh>")
        + visual("<mesh><uri>meshes/box.glb</uri></mesh>", "10 0 0 0 0 0", "g")
        + visual("<mesh><uri>model://other_model/meshes/x.dae</uri></mesh>", name="other")
        + "</link>"
    )
    files = {"meshes/box.stl": box.export(file_type="stl"), "meshes/box.glb": box.export(file_type="glb")}
    r = measure.measure(str(write(tmp_path, sdf, files)))
    # The STL is found despite the case of its name; the other model's mesh
    # can't be, and is named in the warnings.
    assert approx(r["bbox"]["min"], [-0.5, -1, -1.5]) and approx(r["bbox"]["max"], [10.5, 1, 1.5])
    assert r["measured"] == 2 and r["visuals"] == 3
    assert any("other_model" in w for w in r["warnings"])


def test_collisions_when_no_visuals(tmp_path):
    sdf = "<link name='l'>" + visual("<box><size>1 1 1</size></box>", kind="collision") + "</link>"
    r = measure.measure(str(write(tmp_path, sdf, config=False)))
    assert r["from"] == "collision" and approx(r["size"], [1, 1, 1])


def test_degrees_and_quaternions(tmp_path):
    sdf = (
        "<link name='l'>"
        "<visual name='a'><pose degrees='true'>0 0 0 0 0 90</pose><geometry><box><size>2 1 1</size></box>"
        "</geometry></visual>"
        f"<visual name='b'><pose rotation_format='quat_xyzw'>0 0 5 {math.sin(math.pi / 4)} 0 0 "
        f"{math.cos(math.pi / 4)}</pose><geometry><box><size>1 1 2</size></box></geometry></visual>"
        "</link>"
    )
    r = measure.measure(str(write(tmp_path, sdf)))
    # a: turned to 1 x 2 x 1; b: turned about X to 1 x 2 x 1 at z = 5.
    assert approx(r["bbox"]["min"], [-0.5, -1, -0.5]) and approx(r["bbox"]["max"], [0.5, 1, 5.5])


def test_unmeasurable(tmp_path):
    sdf = "<link name='l'>" + visual("<mesh><uri>meshes/missing.dae</uri></mesh>") + "</link>"
    with pytest.raises(measure.MeasureError, match="no visual or collision"):
        measure.measure(str(write(tmp_path, sdf)))
    with pytest.raises(measure.MeasureError, match="no model.config"):
        measure.measure(str(tmp_path / "nothing-here"))


def test_main_writes_report(tmp_path):
    root = write(tmp_path, "<link name='l'>" + visual("<box><size>1 1 1</size></box>") + "</link>")
    out = tmp_path / "out"
    assert measure.main(["measure", str(root), str(out)]) == 0
    assert json.loads((out / "report.json").read_text())["size"] == [1.0, 1.0, 1.0]
    assert measure.main(["measure", str(tmp_path / "missing"), str(out)]) == 2
    assert json.loads((out / "report.json").read_text())["ok"] is False
