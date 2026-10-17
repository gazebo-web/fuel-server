# Sketches, extrude and revolve

**2D shapes:**
- `Rectangle(w, h)`, `Circle(r)`, `Ellipse(rx, ry)`
- `RegularPolygon(r, sides)`
- `Polygon((x, y), ...)` (closed automatically)
- `SlotOverall(length, width)`
- `Text("ABC", font_size)`

They sit on `Plane.XY` unless placed with `Plane.XZ * ...` or `Pos(...) * ...`. Combine them with `+` and `-`.

**Extrude:** `extrude(sketch, amount)` extrudes along the sketch plane's normal.

**Revolve:** `revolve(profile, Axis.Z)` turns a profile drawn on `Plane.XZ`, with x ≥ 0, around Z. Use it for bottles, cups, wheels and lamps.

```python
from build123d import *
import gzmesh as gz

def build():
    # A coffee mug: revolved body, hollowed, with a handle.
    profile = Plane.XZ * Polygon((0, 0), (0.042, 0), (0.045, 0.1), (0, 0.1))
    body = revolve(profile, Axis.Z)
    body = offset(body, amount=-0.004, openings=body.faces().sort_by(Axis.Z)[-1])
    handle = Pos(0.05, 0, 0.05) * Rot(90, 0, 0) * Torus(0.025, 0.007)
    handle -= Cylinder(0.04, 0.1, align=(Align.CENTER, Align.CENTER, Align.MIN))
    return [gz.part(body, "mug", gz.plastic("#e8e4dc", roughness=0.3)),
            gz.part(handle, "handle", gz.plastic("#e8e4dc", roughness=0.3))]
```

```python
from build123d import *
import gzmesh as gz

def build():
    # A sign: an octagon with raised letters.
    plate = extrude(RegularPolygon(0.3, 8), 0.01)
    letters = Pos(0, 0, 0.01) * extrude(Text("STOP", 0.15), 0.004)
    return [gz.part(Rot(90, 0, 0) * plate, "plate", gz.painted_metal("red", wear=0.1)),
            gz.part(Rot(90, 0, 0) * letters, "letters", gz.flat("white"))]
```
