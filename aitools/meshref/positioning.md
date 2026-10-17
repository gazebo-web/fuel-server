# Positioning and primitives

**Primitives** are centered on the origin unless `align` says otherwise:
- `Box(length_x, width_y, height_z)`
- `Cylinder(radius, height)`, along Z
- `Sphere(radius)`
- `Cone(bottom_radius, top_radius, height)`
- `Torus(major_radius, minor_radius)`

`align=(Align.CENTER, Align.CENTER, Align.MIN)` puts a primitive's bottom at z = 0.

**Moving and rotating:** `Pos(x, y, z) * shape` moves a shape, and `Rot(x_deg, y_deg, z_deg) * shape` rotates it. They chain right to left: `Pos(...) * Rot(...) * shape` rotates first, then moves.

```python
from build123d import *
import gzmesh as gz

def build():
    # A table: 1.2 x 0.7 m top, 0.75 m tall, four legs inset 5 cm.
    top = Pos(0, 0, 0.73) * Box(1.2, 0.7, 0.04, align=(Align.CENTER, Align.CENTER, Align.MIN))
    legs = [Pos(x, y, 0) * Cylinder(0.025, 0.73, align=(Align.CENTER, Align.CENTER, Align.MIN))
            for x in (-0.55, 0.55) for y in (-0.3, 0.3)]
    wood = gz.wood("#9a6b3f")
    return [gz.part(top, "top", wood)] + [gz.part(l, f"leg_{i}", wood) for i, l in enumerate(legs)]
```

`Rot(0, 90, 0) * Cylinder(...)` lays a cylinder along X, for an axle or a pipe. Use `Location((x, y, z), (rx, ry, rz))` when you need both at once.
