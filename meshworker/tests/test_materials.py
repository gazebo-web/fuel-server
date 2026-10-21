import os

import numpy as np
import pytest

import gzmesh as gz
from gzmesh.materials import fbm, rgb


def test_rgb_forms():
    assert np.allclose(rgb("#ff8000"), [1, 128 / 255, 0])
    assert np.allclose(rgb((0.1, 0.2, 0.3)), [0.1, 0.2, 0.3])
    assert np.allclose(rgb("white"), rgb("#f2f2f2"))
    for bad in ("tomato", "#12345", (1, 2, 3), (0.1, 0.2)):
        with pytest.raises(ValueError):
            rgb(bad)


def test_every_library_material_colors_in_range():
    p = np.random.default_rng(0).uniform(-1, 1, (500, 3))
    n = np.tile([0, 0, 1.0], (500, 1))
    for name, (fn, doc) in gz.LIBRARY.items():
        if name == "texture" and not os.path.isdir(os.environ.get("GZMESH_TEXTURES", "")):
            continue
        m = (fn(gz.plastic("red"), "white", [(0, 0.5)]) if name == "stripes"
             else fn("bricks") if name == "texture" else fn())
        c = m.color(p, n)
        assert c.shape == (500, 3) and (c >= 0).all() and (c <= 1.0001).all(), name
        assert 0 <= m.roughness <= 1 and 0 <= m.metalness <= 1 and m.density > 0, name
        assert doc.startswith(tuple("ABCDEFGHIJKLMNOPQRSTUVWXYZ")), name


def test_stripes_only_inside_ranges():
    m = gz.stripes(gz.flat("black"), "#ffffff", [(0.2, 0.4)])
    p = np.array([[0, 0, 0.1], [0, 0, 0.3], [0, 0, 0.5]])
    c = m.color(p, np.zeros_like(p))
    assert c[0].max() < 0.1 and c[2].max() < 0.1 and c[1].min() > 0.9


def test_noise_is_deterministic_and_smooth():
    p = np.random.default_rng(1).uniform(0, 1, (200, 3))
    a, b = fbm(p, 10), fbm(p, 10)
    assert np.array_equal(a, b) and a.min() >= 0 and a.max() <= 1
    near = fbm(p + 1e-4, 10)
    assert np.abs(near - a).max() < 0.02


needs_textures = pytest.mark.skipif(not os.path.isdir(os.environ.get("GZMESH_TEXTURES", "")),
                                    reason="texture sets not installed (meshworker/textures/fetch.py)")


@needs_textures
def test_every_texture_set_loads_and_tints():
    p = np.random.default_rng(0).uniform(-1, 1, (400, 3))
    n = np.random.default_rng(1).normal(size=(400, 3))
    for name in gz.texture_sets():
        m = gz.texture(name)
        c = m.color(p, n)
        assert c.shape == (400, 3) and (c >= 0).all() and (c <= 1).all(), name
        if m.roughness_fn:
            r = m.roughness_fn(p, n)
            assert r.shape == (400,) and (r >= 0).all() and (r <= 1).all(), name
    red = gz.texture("painted_metal", tint="#2050c0").color(p, n).mean(0)
    assert red[2] > red[0] * 2, "the tint should make painted metal blue"
    with pytest.raises(ValueError):
        gz.texture("marble")


@needs_textures
def test_texture_is_to_scale():
    """Bricks repeat every tile_m meters: two points a tile apart along a
    wall sample the same color."""
    m = gz.texture("bricks")
    tile = gz.texture_sets()["bricks"]["tile_m"]
    p = np.array([[0.0, 0.13, 0.27], [0.0, 0.13 + tile, 0.27], [0.0, 0.13 + tile / 3, 0.27]])
    n = np.tile([1.0, 0, 0], (3, 1))
    c = m.color(p, n)
    assert np.allclose(c[0], c[1], atol=1e-6) and not np.allclose(c[0], c[2], atol=1e-3)



def _b64_jpeg(color, mode="RGB"):
    import base64
    import io

    from PIL import Image
    buf = io.BytesIO()
    Image.new(mode, (16, 16), color).save(buf, "JPEG")
    return base64.b64encode(buf.getvalue()).decode()


def test_project_texture(build):
    code = (
        "from build123d import *\n"
        "import gzmesh as gz\n"
        "def build():\n"
        "    return gz.part(Box(0.5, 0.5, 0.5), 'block', gz.texture('project:brick', scale=2))\n"
    )
    textures = {"brick": {"color": _b64_jpeg((160, 50, 30)), "roughness": _b64_jpeg(200, "L"), "tile_m": 0.5}}
    rc, report, out = build(code, project_textures=textures)
    assert rc == 0, report
    assert report["parts"][0]["material"]["texture"] == "project:brick"
    assert report["parts"][0]["material"]["roughness"] == pytest.approx(200 / 255, abs=0.02)

    rc, report, _ = build(code.replace("project:brick", "project:tiles"), project_textures=textures)
    assert rc == 2 and "no texture file named 'tiles'" in report["error"]
