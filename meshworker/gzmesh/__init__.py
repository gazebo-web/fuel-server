"""gzmesh: sim-ready models from agent-written build123d code (plan §5).

What model code uses:

    from build123d import *
    import gzmesh as gz

    def build():
        base = Box(0.36, 0.36, 0.03)
        cone = Pos(0, 0, 0.315) * Cone(0.14, 0.025, 0.57)
        return [
            gz.part(Pos(0, 0, 0.015) * base, "base", gz.rubber()),
            gz.part(cone, "cone", gz.stripes(gz.plastic("orange"), "white", [(0.33, 0.45)])),
        ]

The pipeline (run.py) does the rest: tessellation within a triangle
budget, one baked PBR atlas, collision, inertia, GLB, SDF and a preview.
"""

from .materials import (LIBRARY, Material, asphalt, brushed_metal, cardboard, concrete, fabric, flat, glass,
                        painted_metal, plastic, rubber, stripes, texture, texture_sets, wood)
from .parts import (Assembly, Joint, Link, Part, Plugin, Sensor, camera, continuous, depth_camera, diff_drive,
                    fixed, imu, joint_states, lidar, link, model, part, position_controller, prismatic, revolute,
                    rgbd_camera, velocity_controller)

__all__ = ["part", "Part", "link", "Link", "model", "Assembly", "Joint", "revolute", "prismatic", "continuous",
           "fixed", "Sensor", "camera", "depth_camera", "rgbd_camera", "lidar", "imu", "Plugin", "diff_drive",
           "joint_states", "position_controller", "velocity_controller", "Material", "LIBRARY", "flat", "plastic", "rubber", "painted_metal", "brushed_metal",
           "wood", "concrete", "asphalt", "fabric", "cardboard", "glass", "stripes", "texture", "texture_sets"]
