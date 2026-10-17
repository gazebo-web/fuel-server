"""The contract between agent-written code and the pipeline (plan §5.2).

The code defines build(), which returns a build123d shape, a Part from
part(), or a list of those. Everything else (tessellation, UVs, materials,
collision, inertia, files) is the pipeline's job.
"""

from dataclasses import dataclass
from typing import Optional

from .materials import Material, plastic


@dataclass
class Part:
    shape: object  # a build123d Shape (Solid, Part, Compound)
    name: str
    material: Material


def part(shape, name: str = "part", material: Optional[Material] = None) -> Part:
    """A named piece of the model with its material (default: grey plastic)."""
    if not hasattr(shape, "wrapped"):
        raise TypeError(f"part {name!r}: want a build123d shape, got {type(shape).__name__}")
    if material is not None and not isinstance(material, Material):
        raise TypeError(f"part {name!r}: material must come from gzmesh (e.g. gz.plastic('red')), "
                        f"got {type(material).__name__}")
    return Part(shape, name, material or plastic("grey"))


def normalize(result) -> list[Part]:
    """Turn whatever build() returned into a non-empty list of Parts with
    unique names."""
    items = result if isinstance(result, (list, tuple)) else [result]
    parts = []
    for i, it in enumerate(items):
        parts.append(it if isinstance(it, Part) else part(it, f"part_{i}"))
    if not parts:
        raise ValueError("build() returned no parts")
    seen = {}
    for p in parts:
        n = seen.get(p.name, 0)
        seen[p.name] = n + 1
        if n:
            p.name = f"{p.name}_{n}"
        vol = getattr(p.shape, "volume", 0) or 0
        if vol <= 0:
            raise ValueError(f"part {p.name!r} has no volume: build() must return solids "
                             f"(extrude sketches, or use Box/Cylinder/...)")
    return parts
