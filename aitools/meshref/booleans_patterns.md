# Booleans and patterns

**Booleans:** `a + b` is a union, `a - b` cuts b out of a, and `a & b` keeps the overlap. The right-hand side can be a list.

**Patterns:**
- `GridLocations(x_spacing, y_spacing, nx, ny)` places copies on a grid.
- `PolarLocations(radius, count)` places copies around a circle.

Apply either with a list comprehension: `[loc * shape for loc in GridLocations(...)]`.

Several parts in one material can be fused with `+`, or returned as separate `gz.part`s; separate parts can each have their own material.

```python
from build123d import *
import gzmesh as gz

def build():
    # A warehouse pallet (1.2 x 0.8 m): deck boards on three runners.
    runners = [Pos(0, y, 0) * Box(1.2, 0.1, 0.1, align=(Align.CENTER, Align.CENTER, Align.MIN))
               for y in (-0.35, 0, 0.35)]
    boards = [loc * Box(0.1, 0.8, 0.022, align=(Align.CENTER, Align.CENTER, Align.MIN))
              for loc in GridLocations(0.1375, 0, 9, 1)]
    deck = Pos(0, 0, 0.1) * (boards[0] + boards[1:])
    wood = gz.wood("#b8935a", axis="y")
    return [gz.part(deck, "deck", wood)] + [gz.part(r, f"runner_{i}", wood) for i, r in enumerate(runners)]
```

```python
from build123d import *
import gzmesh as gz

def build():
    # A wheel: hub disc with six round cut-outs inside a rubber tire, axle
    # along Y. The disc stops at the tire's inner radius (0.14 - 0.03) so
    # the parts don't overlap.
    disc = Cylinder(0.11, 0.03) - [loc * Cylinder(0.025, 0.04) for loc in PolarLocations(0.065, 6)]
    tire = Torus(0.14, 0.03)
    return [gz.part(Rot(90, 0, 0) * disc, "hub", gz.painted_metal("silver")),
            gz.part(Rot(90, 0, 0) * tire, "tire", gz.rubber())]
```
