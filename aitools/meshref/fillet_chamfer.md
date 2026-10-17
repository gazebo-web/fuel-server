# Fillets, chamfers and selecting edges

`fillet(edges, radius)` rounds edges and `chamfer(edges, length)` bevels them. Pick the edges from the shape:
- `shape.edges()`: every edge
- `.filter_by(Axis.Z)`: edges parallel to Z (a box's vertical corners)
- `.group_by(Axis.Z)[-1]`: the topmost group of edges; `[0]` is the bottom
- `.sort_by(Axis.X)[-1]`: the edge furthest along +X
- `shape.faces().sort_by(Axis.Z)[-1]`: the top face

A radius larger than the edge allows fails with a geometry error. Use a smaller one.

```python
from build123d import *
import gzmesh as gz

def build():
    # A storage bin: rounded vertical corners, chamfered bottom edges, then
    # hollowed. Round and chamfer before hollowing: a 4 mm wall is too thin
    # to chamfer afterwards.
    bin_ = Box(0.4, 0.3, 0.25, align=(Align.CENTER, Align.CENTER, Align.MIN))
    bin_ = fillet(bin_.edges().filter_by(Axis.Z), 0.03)
    bin_ = chamfer(bin_.edges().group_by(Axis.Z)[0], 0.01)
    bin_ = offset(bin_, amount=-0.004, openings=bin_.faces().sort_by(Axis.Z)[-1])
    return gz.part(bin_, "bin", gz.plastic("#2f7d4a"))
```
