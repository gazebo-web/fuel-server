# model_generate: overview

Write Python that defines `build()`. It returns one build123d solid, a part from `gz.part(shape, name, material)`, or a list of parts. Units are meters, and Z is up. The pipeline does everything else:
- triangles within `tri_budget`;
- one baked PBR texture;
- collision: a primitive, one hull, or a convex decomposition;
- exact inertia from the solids;
- the GLB, `model.sdf` and `model.config`;
- a preview image with four views: three-quarter from the front (+X -Y), three-quarter from the back (-X +Y), front (+X) and top. For a model with joints, the front view is replaced by the front three-quarter view with every joint moved, to check that doors, drawers and wheels move the right way.

The model's origin ends up at the bottom center of its bounding box, so `<pose>` with z = 0 sets it on the floor. Build it upright, with its front facing +X.

Only these imports work: `build123d`, `gzmesh`, `math`, `numpy`, `random`, `itertools`, `functools`, `typing`, `dataclasses`, `collections`, `copy`. There are no files and no network.

```python
from build123d import *
import gzmesh as gz

def build():
    # A 0.6 m traffic cone on a square rubber base with rounded corners.
    base = Box(0.36, 0.36, 0.03, align=(Align.CENTER, Align.CENTER, Align.MIN))
    base = fillet(base.edges().filter_by(Axis.Z), 0.03)
    cone = Pos(0, 0, 0.03) * Cone(0.14, 0.025, 0.57, align=(Align.CENTER, Align.CENTER, Align.MIN))
    return [
        gz.part(base, "base", gz.rubber()),
        gz.part(cone, "cone", gz.stripes(gz.plastic("orange"), "white", [(0.33, 0.45)])),
    ]
```

Topics: `positioning`, `sketches`, `sweep_loft`, `fillet_chamfer`, `booleans_patterns`, `articulation` (doors, drawers, wheels), `robots` (sensors, drive controllers), `materials`, `errors`.
