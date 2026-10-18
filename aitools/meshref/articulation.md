# Moving parts: links and joints

For doors, drawers, lids, wheels and arms, return `gz.model(links=[...], joints=[...])` instead of a list of parts:
- **Links:** `gz.link(name, part, part, ...)` is a rigid body. Its parts move together, and each link gets its own mesh, collision and inertia.
- **Joints** connect a parent link to a child link. Every link except one root has exactly one parent joint, so the links form a tree.
  - `gz.revolute(name, parent, child, origin, axis, lower, upper)` is a hinge. The child rotates about `axis` through `origin`, between `lower` and `upper` radians.
  - `gz.prismatic(name, parent, child, origin, axis, lower, upper)` is a slider. The child moves along `axis`, between `lower` and `upper` meters.
  - `gz.continuous(name, parent, child, origin, axis)` is an unlimited rotation, for wheels.
  - `gz.fixed(name, parent, child)` welds two links. It's rarely needed: put the parts in one link instead.

Build every part where it sits in the closed or rest pose, and give joint origins and axes in that same model frame. Joint position 0 is the pose as built.

**Picking the hinge direction:** the positive direction follows the right-hand rule about `axis`. A door hinged at its +Y edge, with its face toward +X, opens outward for positive angles about +Z.

Joints have damping by default (revolute 0.5, prismatic 2.0), so doors and drawers don't swing freely. Pass `damping=` to change it.

```python
from build123d import *
import gzmesh as gz

def build():
    # A cabinet (front +X) with a shelf and a door hinged on its +Y edge.
    body = Box(0.4, 0.6, 0.9, align=(Align.CENTER, Align.CENTER, Align.MIN))
    body = offset(body, amount=-0.018, openings=body.faces().sort_by(Axis.X)[-1])
    shelf = Pos(0, 0, 0.45) * Box(0.36, 0.56, 0.018)
    door = Pos(0.209, 0, 0.45) * Box(0.018, 0.6, 0.9)
    knob = Pos(0.233, -0.25, 0.5) * Rot(0, 90, 0) * Cylinder(0.012, 0.03)
    wood = gz.wood("#8a5a35")
    return gz.model(
        links=[gz.link("body", gz.part(body, "carcass", wood), gz.part(shelf, "shelf", wood)),
               gz.link("door", gz.part(door, "door", wood), gz.part(knob, "knob", gz.brushed_metal()))],
        joints=[gz.revolute("door_hinge", "body", "door", origin=(0.209, 0.3, 0.45), axis=(0, 0, 1),
                            lower=0, upper=1.9)])
```

```python
from build123d import *
import gzmesh as gz

def build():
    # A chest with one drawer that slides out along +X up to 30 cm.
    body = Box(0.5, 0.5, 0.3, align=(Align.CENTER, Align.CENTER, Align.MIN))
    body -= Pos(0.05, 0, 0.15) * Box(0.42, 0.44, 0.22)  # the drawer pocket, open at the front
    tray = Pos(0.055, 0, 0.15) * Box(0.39, 0.42, 0.2)
    tray = offset(tray, amount=-0.01, openings=tray.faces().sort_by(Axis.Z)[-1])
    front = Pos(0.26, 0, 0.15) * Box(0.02, 0.48, 0.26)
    wood = gz.wood("#b8935a")
    return gz.model(
        links=[gz.link("chest", gz.part(body, "body", wood)),
               gz.link("drawer", gz.part(tray, "tray", wood), gz.part(front, "front", gz.flat("#3a5a40", density=600)))],
        joints=[gz.prismatic("drawer_slide", "chest", "drawer", origin=(0.25, 0, 0.15), axis=(1, 0, 0),
                             lower=0, upper=0.3)])
```

```python
from build123d import *
import gzmesh as gz

def build():
    # A cart: a platform on four wheels that spin about Y.
    deck = Pos(0, 0, 0.2) * Box(0.6, 0.4, 0.04)
    links, joints = [gz.link("deck", gz.part(deck, "deck", gz.wood("#a07a4a")))], []
    for name, x, y in (("fl", 0.22, 0.24), ("fr", 0.22, -0.24), ("rl", -0.22, 0.24), ("rr", -0.22, -0.24)):
        wheel = Pos(x, y, 0.1) * Rot(90, 0, 0) * Cylinder(0.1, 0.04)
        links.append(gz.link(f"wheel_{name}", gz.part(wheel, "tire", gz.rubber())))
        joints.append(gz.continuous(f"axle_{name}", "deck", f"wheel_{name}", origin=(x, y, 0.1), axis=(0, 1, 0)))
    return gz.model(links=links, joints=joints)
```

**Checks:**
- The report lists each link's mass and collision, and each joint.
- A warning flags a joint origin far from both its links. That usually means the wrong frame, or millimeters where meters were meant.
- The preview shows the rest pose.

For sensors and drive controllers, see topic "robots".
