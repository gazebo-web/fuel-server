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
        m = fn(gz.plastic("red"), "white", [(0, 0.5)]) if name == "stripes" else fn()
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
