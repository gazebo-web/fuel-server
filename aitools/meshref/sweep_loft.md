# Sweep and loft

**Sweep:** `sweep(section, path)` moves a 2D section along a path. Paths can be:
- `Line(p0, p1)`
- `Polyline(p0, p1, ...)`
- `Spline(p0, p1, ...)`
- `ThreePointArc(p0, mid, p1)`
- `JernArc(...)`

Place the section at the path's start, perpendicular to it, with `(path ^ 0) * section`. Use it for pipes, rails, handles and hoses.

**Loft:** `loft([s0, s1, ...])` skins a solid through sections on parallel planes. Place the sections with `Plane.XY.offset(z) * ...`. Use it for vases, funnels and tapered housings.

```python
from build123d import *
import gzmesh as gz

def build():
    # A curved handrail on two posts.
    path = Spline((0, 0, 0.9), (0.5, 0.15, 0.95), (1.0, 0, 0.9))
    rail = sweep((path ^ 0) * Circle(0.02), path)
    posts = [Pos(x, 0, 0) * Cylinder(0.02, 0.9, align=(Align.CENTER, Align.CENTER, Align.MIN)) for x in (0, 1.0)]
    steel = gz.brushed_metal()
    return [gz.part(rail, "rail", steel)] + [gz.part(p, f"post_{i}", steel) for i, p in enumerate(posts)]
```

```python
from build123d import *
import gzmesh as gz

def build():
    # A vase lofted through a square base, a round belly and a narrow neck.
    vase = loft([Rectangle(0.12, 0.12), Plane.XY.offset(0.15) * Circle(0.09), Plane.XY.offset(0.3) * Circle(0.035)])
    return gz.part(vase, "vase", gz.plastic("#3a6ea5", roughness=0.25))
```
