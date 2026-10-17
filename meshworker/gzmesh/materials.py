"""Materials for generated models (plan §5.3).

A material colors texels from their 3D position and normal, so procedural
patterns are solid textures: they need no projection and show no seams
between UV charts. Every material is baked into the model's one atlas
(atlas.py).

Colors are sRGB, as written in the base color texture (glTF's convention;
three.js decodes it, see meshworker/spike/README.md). Accepted forms:
"#rrggbb", (r, g, b) floats in 0-1, or a name from NAMED_COLORS.
"""

from dataclasses import dataclass, field
from typing import Callable, Sequence

import numpy as np

# A few names the agent will reach for; anything else is "#rrggbb".
NAMED_COLORS = {
    "black": "#111111", "white": "#f2f2f2", "grey": "#808080", "gray": "#808080",
    "red": "#c41e1e", "green": "#2e8b3a", "blue": "#1f4fbf", "yellow": "#f2c40f",
    "orange": "#f26a0f", "brown": "#7a4b26", "silver": "#c0c0c0",
}


def rgb(color) -> np.ndarray:
    """Parse a color into sRGB floats in 0-1."""
    if isinstance(color, str):
        c = NAMED_COLORS.get(color.lower(), color)
        if not (c.startswith("#") and len(c) == 7):
            raise ValueError(f"color {color!r}: want '#rrggbb', (r, g, b) in 0-1, or one of {sorted(NAMED_COLORS)}")
        return np.array([int(c[i:i + 2], 16) / 255 for i in (1, 3, 5)])
    a = np.asarray(color, float)
    if a.shape != (3,) or (a < 0).any() or (a > 1).any():
        raise ValueError(f"color {color!r}: want three floats in 0-1")
    return a


@dataclass
class Material:
    """name: a short label for the report. color(p, n) returns sRGB (N, 3)
    for positions and normals (N, 3) in meters. density in kg/m³ is the
    default used for mass when the job gives neither mass nor density."""
    name: str
    color: Callable[[np.ndarray, np.ndarray], np.ndarray]
    roughness: float = 0.6
    metalness: float = 0.0
    density: float = 1000.0
    params: dict = field(default_factory=dict)

    def describe(self) -> dict:
        return {"name": self.name, "roughness": self.roughness, "metalness": self.metalness,
                "density": self.density, **self.params}


# -- noise: smooth 3D value noise, deterministic, vectorized.

def _hash(ix, iy, iz):
    h = (ix * 374761393 + iy * 668265263 + iz * 2147483647) & 0xFFFFFFFF
    h = ((h ^ (h >> 13)) * 1274126177) & 0xFFFFFFFF
    return (h ^ (h >> 16)) / 0xFFFFFFFF


def value_noise(p: np.ndarray, scale: float) -> np.ndarray:
    """Noise in 0-1 with features about 1/scale meters across."""
    q = p * scale
    i = np.floor(q).astype(np.int64)
    f = q - i
    f = f * f * (3 - 2 * f)  # smoothstep
    out = np.zeros(len(p))
    for dx in (0, 1):
        for dy in (0, 1):
            for dz in (0, 1):
                w = (f[:, 0] if dx else 1 - f[:, 0]) * (f[:, 1] if dy else 1 - f[:, 1]) * (f[:, 2] if dz else 1 - f[:, 2])
                out += w * _hash(i[:, 0] + dx, i[:, 1] + dy, i[:, 2] + dz)
    return out


def fbm(p: np.ndarray, scale: float, octaves: int = 4) -> np.ndarray:
    """Fractal noise in 0-1."""
    total, amp, norm = np.zeros(len(p)), 1.0, 0.0
    for o in range(octaves):
        total += amp * value_noise(p + o * 17.31, scale * 2 ** o)
        norm += amp
        amp *= 0.5
    return total / norm


def _mix(a, b, t):
    t = np.clip(t, 0, 1)[:, None]
    return a * (1 - t) + b * t


# -- library

def flat(color="grey", roughness=0.6, metalness=0.0, density=1000.0) -> Material:
    """One solid color."""
    c = rgb(color)
    return Material("flat", lambda p, n: np.broadcast_to(c, p.shape).copy(), roughness, metalness, density,
                    {"color": color})


def plastic(color="white", roughness=0.45) -> Material:
    c = rgb(color)
    return Material("plastic", lambda p, n: c * (0.96 + 0.04 * fbm(p, 40, 2))[:, None], roughness, 0.0, 1000.0,
                    {"color": color})


def rubber(color="black") -> Material:
    c = rgb(color)
    return Material("rubber", lambda p, n: c * (0.9 + 0.1 * fbm(p, 120, 2))[:, None], 0.9, 0.0, 1100.0,
                    {"color": color})


def painted_metal(color="blue", wear=0.2) -> Material:
    """Paint over steel; wear (0-1) exposes metal in noisy patches."""
    c, steel = rgb(color), rgb("#8a8d91")

    def color_fn(p, n):
        chips = fbm(p, 25, 4)
        bare = np.clip((chips - (1 - 0.5 * wear)) * 12, 0, 1)
        return _mix(c * (0.94 + 0.06 * fbm(p, 80, 2))[:, None], steel, bare)
    return Material("painted_metal", color_fn, 0.5, 0.2, 7850.0, {"color": color, "wear": wear})


def brushed_metal(color="silver") -> Material:
    """Fine streaks along X."""
    c = rgb(color)

    def color_fn(p, n):
        streak = value_noise(p * np.array([2.0, 400.0, 400.0]), 1.0)
        return c * (0.88 + 0.12 * streak)[:, None]
    return Material("brushed_metal", color_fn, 0.35, 1.0, 7850.0, {"color": color})


def wood(color="#a0703c", grain_scale=1.0, axis="x") -> Material:
    """Rings around `axis`, distorted by noise; grain runs along it."""
    c = rgb(color)
    dark = c * 0.6
    ax = "xyz".index(axis)

    def color_fn(p, n):
        q = np.delete(p, ax, axis=1)
        r = np.linalg.norm(q, axis=1) * 25 * grain_scale + 3 * fbm(p, 4, 3)
        rings = 0.5 + 0.5 * np.sin(r * 2 * np.pi)
        return _mix(c, dark, rings ** 3 * 0.8 + 0.2 * fbm(p, 200, 2))
    return Material("wood", color_fn, 0.7, 0.0, 600.0, {"color": color, "axis": axis})


def concrete(color="#9a9893") -> Material:
    c = rgb(color)
    return Material("concrete", lambda p, n: c * (0.82 + 0.18 * fbm(p, 30, 5))[:, None], 0.9, 0.0, 2400.0,
                    {"color": color})


def asphalt(color="#3a3a3c") -> Material:
    c = rgb(color)

    def color_fn(p, n):
        speckle = (value_noise(p, 900) > 0.8) * 0.25
        return np.clip(c * (0.85 + 0.15 * fbm(p, 50, 3))[:, None] + speckle[:, None], 0, 1)
    return Material("asphalt", color_fn, 0.95, 0.0, 2300.0, {"color": color})


def fabric(color="#4a5a7a") -> Material:
    c = rgb(color)

    def color_fn(p, n):
        weave = 0.5 + 0.25 * (np.sin(p[:, 0] * 1500) + np.sin(p[:, 1] * 1500))
        return c * (0.85 + 0.1 * weave + 0.05 * fbm(p, 60, 2))[:, None]
    return Material("fabric", color_fn, 0.95, 0.0, 300.0, {"color": color})


def cardboard(color="#b08a5a") -> Material:
    c = rgb(color)
    return Material("cardboard", lambda p, n: c * (0.88 + 0.12 * fbm(p, 20, 4))[:, None], 0.85, 0.0, 150.0,
                    {"color": color})


def glass(color="#a8c8d8") -> Material:
    """Opaque tinted glass: Gazebo only honors alpha masks, not blending."""
    c = rgb(color)
    return Material("glass", lambda p, n: np.broadcast_to(c, p.shape).copy(), 0.05, 0.0, 2500.0, {"color": color})


def stripes(base: Material, color, ranges: Sequence[Sequence[float]], axis="z") -> Material:
    """`base` with bands of `color` where the coordinate along `axis` (m,
    in the model frame) is inside one of `ranges`, e.g. a cone's
    reflective bands: stripes(plastic("orange"), "white", [(0.33, 0.45)])."""
    c, ax = rgb(color), "xyz".index(axis)
    spans = [(float(a), float(b)) for a, b in ranges]

    def color_fn(p, n):
        out = base.color(p, n)
        inside = np.zeros(len(p), bool)
        for a, b in spans:
            inside |= (p[:, ax] >= a) & (p[:, ax] <= b)
        if inside.any():
            out[inside] = c * (0.95 + 0.05 * fbm(p[inside], 40, 2))[:, None]
        return out
    return Material(f"{base.name}+stripes", color_fn, base.roughness, base.metalness, base.density,
                    {**base.params, "stripes": {"color": color, "ranges": spans, "axis": axis}})


LIBRARY = {
    "flat": (flat, "One solid color: flat(color, roughness=0.6, metalness=0.0, density=1000)"),
    "plastic": (plastic, "Slightly mottled plastic: plastic(color, roughness=0.45)"),
    "rubber": (rubber, "Matte rubber: rubber(color='black')"),
    "painted_metal": (painted_metal, "Paint over steel with chipped wear: painted_metal(color, wear=0.2)"),
    "brushed_metal": (brushed_metal, "Metal with fine streaks along X: brushed_metal(color='silver')"),
    "wood": (wood, "Wood with rings around an axis: wood(color, grain_scale=1.0, axis='x')"),
    "concrete": (concrete, "Mottled concrete: concrete(color)"),
    "asphalt": (asphalt, "Asphalt with light speckles: asphalt(color)"),
    "fabric": (fabric, "Woven fabric: fabric(color)"),
    "cardboard": (cardboard, "Corrugated-box cardboard: cardboard(color)"),
    "glass": (glass, "Tinted glass, rendered opaque (Gazebo has no blending): glass(color)"),
    "stripes": (stripes, "Bands of a color over another material: stripes(base, color, ranges=[(z0, z1)], axis='z')"),
}
