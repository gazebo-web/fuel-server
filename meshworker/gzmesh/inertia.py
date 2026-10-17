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

from .parts import Part


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


def compute(parts: list[Part], mass: float | None = None, density: float | None = None) -> Inertial:
    if mass is not None and density is not None:
        raise ValueError("give mass or density, not both")
    props, volumes = [], []
    for p in parts:
        g = GProp_GProps()
        BRepGProp.VolumeProperties_s(p.shape.wrapped, g)
        volumes.append(g.Mass())  # with unit density, mass is the volume
        props.append(g)
    densities = [density or p.material.density for p in parts]
    if mass is not None:
        if mass <= 0:
            raise ValueError("mass must be positive")
        natural = sum(d * v for d, v in zip(densities, volumes))
        densities = [d * mass / natural for d in densities]
    total = GProp_GProps()
    for g, d in zip(props, densities):
        total.Add(g, d)
    c = total.CentreOfMass()
    m = total.MatrixOfInertia()
    tensor = np.array([[m.Value(i, j) for j in (1, 2, 3)] for i in (1, 2, 3)])
    return Inertial(total.Mass(), np.array([c.X(), c.Y(), c.Z()]), tensor, float(sum(volumes)))
