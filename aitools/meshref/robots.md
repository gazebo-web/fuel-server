# Robots: sensors and controllers

A robot is a `gz.model` with links and joints (topic "articulation"), plus `sensors=[...]` and `plugins=[...]`. Declare them in the code, never by editing model.sdf: regenerating replaces model.sdf, and the code keeps them.

**Sensors.** `pos` is in the model frame as built, like joint origins. `rpy` turns the sensor (radians: roll about X, then pitch about Y, then yaw about Z). Cameras and lidars look along the sensor's +X, so `rpy=(0, 0, 0)` looks toward the robot's front (+X). Put a sensor on the surface of its link, not inside a part: inside, it sees the part.
- `gz.camera(name, link, pos, rpy=(0,0,0), width=640, height=480, fov=1.047, rate=30, near=0.05, far=100)`. fov is the horizontal field of view, in radians.
- `gz.depth_camera(...)` and `gz.rgbd_camera(...)` take the same arguments.
- `gz.lidar(name, link, pos, rpy=(0,0,0), samples=360, fov=2π, vertical_samples=1, vertical_fov=0, min_range=0.1, max_range=30, rate=10)`. Set `vertical_samples` above 1 for a 3D lidar.
- `gz.imu(name, link, pos=(0,0,0), rate=100)`

**Plugins** drive the robot:
- `gz.diff_drive(left, right, wheel_separation=None, wheel_radius=None, max_speed=1.0)` steers with the left and right wheel joints. Pass a name or a list for each side.
  - The left wheels are on +Y and their axes are (0, 1, 0), so positive speed rolls the robot toward +X.
  - Wheel separation and radius are measured from the model unless given.
  - Commands go to `/model/<instance>/cmd_vel`; odometry comes back on `/model/<instance>/odometry`.
- `gz.joint_states()` publishes every joint's position and velocity.
- `gz.position_controller(joint, p_gain=10, d_gain=0.5)` holds a joint at the position sent on its topic, for arms and grippers.
- `gz.velocity_controller(joint)` turns a joint at the speed sent on its topic, for conveyors and fans.

`gz.link(name, *parts, friction=None)` sets the friction of a link's collisions. Use `friction=0` for a caster ball, so it slides instead of dragging.

**The world needs systems.** Sensors publish nothing unless world.sdf loads the system for them. The report's `world_systems` lists the `<plugin>` lines to add inside `<world>`:
- the Sensors system for cameras and lidars;
- the Imu system for IMUs.

A world that lists any system gets none of Gazebo's defaults, so keep Physics, UserCommands and SceneBroadcaster as well.

```python
from build123d import *
import gzmesh as gz

def build():
    # A two-wheeled robot (front +X): a rounded chassis, a caster ball, a
    # lidar on a mast, a front camera and an IMU.
    chassis = Pos(0, 0, 0.12) * Box(0.4, 0.28, 0.1)
    chassis = fillet(chassis.edges().filter_by(Axis.Z), 0.03)
    mast = Pos(0.05, 0, 0.19) * Cylinder(0.02, 0.04)
    lidar_head = Pos(0.05, 0, 0.225) * Cylinder(0.035, 0.03)
    caster = Pos(-0.15, 0, 0.035) * Sphere(0.035)
    grey, black = gz.painted_metal("#5a6470", wear=0.1), gz.rubber()
    links = [gz.link("chassis", gz.part(chassis, "body", grey), gz.part(mast, "mast", grey),
                     gz.part(lidar_head, "lidar", gz.plastic("#222222"))),
             gz.link("caster", gz.part(caster, "ball", gz.plastic("#dddddd")), friction=0)]
    joints = [gz.fixed("caster_mount", "chassis", "caster")]
    for side, y in (("left", 0.17), ("right", -0.17)):
        wheel = Pos(0.05, y, 0.07) * Rot(90, 0, 0) * Cylinder(0.07, 0.04)
        links.append(gz.link(f"wheel_{side}", gz.part(wheel, "tire", black)))
        joints.append(gz.continuous(f"wheel_{side}_joint", "chassis", f"wheel_{side}", origin=(0.05, y, 0.07),
                                    axis=(0, 1, 0)))
    return gz.model(
        links=links, joints=joints,
        sensors=[gz.lidar("lidar", "chassis", pos=(0.05, 0, 0.245), max_range=12),
                 gz.camera("front_camera", "chassis", pos=(0.201, 0, 0.14), width=320, height=240),
                 gz.imu("imu", "chassis", pos=(0, 0, 0.12))],
        plugins=[gz.diff_drive(left="wheel_left_joint", right="wheel_right_joint"), gz.joint_states()])
```

**Checks:**
- The preview marks each sensor with a dot, plus a line showing where cameras and lidars look. Blue is a camera, red a lidar, green an IMU.
- The report lists each sensor's topics. Their topics fill in `<world>` and `<instance>` when the world includes the model.
- A warning flags a sensor buried inside a part, and wheels that would drive the robot backward.
