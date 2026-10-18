"""Mass properties from the exact B-rep (OCCT), plan §5.4.

Each part contributes with its own density (from its material unless the
job sets one), so a steel frame with a plastic cover gets the right center
of mass. A job's total mass, if given, scales every density by the same
factor.
"""

from dataclasses import dataclass

import numpy as np
from OCP.BRepGProp import BRepGProp
from OCP.GProp import GProp_GProps

from .parts import Link, Part


@dataclass
class Inertial:
    mass: float
    center: np.ndarray   # model frame, before the bottom-center shift
    inertia: np.ndarray  # 3x3 about the center of mass, model axes
    volume: float        # sum of the parts' volumes

    def check(self):
        """Positive mass and a physically valid tensor (positive definite,
        triangle inequality on the principal moments)."""
        ev = np.linalg.eigvalsh(self.inertia)
        if self.mass <= 0 or (ev <= 0).any():
            raise ValueError(f"invalid inertia: mass {self.mass}, principal moments {ev}")
        a, b, c = sorted(ev)
        if a + b < c * (1 - 1e-6):
            raise ValueError(f"invalid inertia: principal moments {ev} violate the triangle inequality")


def _props(p: Part) -> GProp_GProps:
    g = GProp_GProps()
    BRepGProp.VolumeProperties_s(p.shape.wrapped, g)  # with unit density, mass is the volume
    return g


def compute_links(links, mass: float | None = None, density: float | None = None) -> list[Inertial]:
    """Mass properties per link (gzmesh.parts.Link). Densities come from
    each part's material unless the job sets one; a total mass, if given,
    scales every density by the same factor, so heavier materials keep
    their share."""
    if mass is not None and density is not None:
        raise ValueError("give mass or density, not both")
    if mass is not None and mass <= 0:
        raise ValueError("mass must be positive")
    props = [[(_props(p), density or p.material.density) for p in lk.parts] for lk in links]
    factor = 1.0
    if mass is not None:
        natural = sum(g.Mass() * d for link in props for g, d in link)
        factor = mass / natural
    out = []
    for link in props:
        total = GProp_GProps()
        for g, d in link:
            total.Add(g, d * factor)
        c = total.CentreOfMass()
        m = total.MatrixOfInertia()
        tensor = np.array([[m.Value(i, j) for j in (1, 2, 3)] for i in (1, 2, 3)])
        out.append(Inertial(total.Mass(), np.array([c.X(), c.Y(), c.Z()]), tensor,
                            float(sum(g.Mass() for g, _ in link))))
    return out


def compute(parts: list[Part], mass: float | None = None, density: float | None = None) -> Inertial:
    """Mass properties of one rigid body made of parts."""
    return compute_links([Link("link", parts)], mass, density)[0]
