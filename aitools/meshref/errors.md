# Common errors

| Error | Cause and fix |
|---|---|
| `has no volume` | `build()` returned a 2D sketch or a wire. `extrude` or `revolve` it. |
| `StdFail_NotDone` / `fillet` or `chamfer` fails | The radius is too large for an edge, or the edges belong to a curved face. Use a smaller radius, or select fewer edges. Round and chamfer before hollowing with `offset`: thin walls can't take them afterwards. |
| `revolve`: invalid or empty solid | The profile crosses the axis, or isn't on a plane containing it. Draw on `Plane.XZ` with x ≥ 0. |
| `sweep`: twisted or failed | The section isn't at the path's start or isn't perpendicular to it. Use `(path ^ 0) * section`. |
| `offset` hollowing fails | The wall is thicker than a feature, or `openings` isn't a face of that shape. Select the face from the same object you offset. |
| `parts overlap` warning | Parts intersect, so the overlap's mass is counted twice. Fuse them with `+` or move them apart. |
| `decimated ... to fit tri_budget` warning | The shape needed more triangles than allowed, and detail may be lost. Raise `tri_budget`, or simplify small features. |
| `import 'x' is not allowed` | Only the listed modules exist in the sandbox. |
| Model lies on its side or faces backwards | Z is up and +X is the front. Rotate with `Rot(...)` in `build()`; don't rotate with `<pose>` to fix it. |
| Model is huge or tiny | Units are meters: a mug is about 0.1, a table about 0.75 tall. |

The error comes back with the line in your code. Fix the code and call `model_generate` again with the same model name: the model folder is replaced.
