# Materials

Give each part a material: `gz.part(shape, name, material)`. Colors are `"#rrggbb"` (sRGB), `(r, g, b)` floats in 0–1, or one of black, white, grey, red, green, blue, yellow, orange, brown and silver. Each material also sets a default density, which gives the mass when `model_generate` gets neither `mass` nor `density`.

| Material | Use | Density (kg/m³) |
|---|---|---|
| `gz.flat(color, roughness=0.6, metalness=0.0, density=1000)` | One solid color | 1000 |
| `gz.plastic(color, roughness=0.45)` | Slightly mottled plastic | 1000 |
| `gz.rubber(color="black")` | Matte rubber | 1100 |
| `gz.painted_metal(color, wear=0.2)` | Paint over steel, chipped by `wear` (0–1) | 7850 |
| `gz.brushed_metal(color="silver")` | Metal with fine streaks along X | 7850 |
| `gz.wood(color, grain_scale=1.0, axis="x")` | Wood; rings around `axis`, grain along it | 600 |
| `gz.concrete(color)` | Mottled concrete | 2400 |
| `gz.asphalt(color)` | Asphalt with light speckles | 2300 |
| `gz.fabric(color)` | Woven fabric | 300 |
| `gz.cardboard(color)` | Corrugated-box cardboard | 150 |
| `gz.glass(color)` | Tinted glass, rendered opaque (Gazebo has no blending) | 2500 |
| `gz.stripes(base, color, ranges=[(z0, z1)], axis="z")` | Bands of `color` over another material | base's |

Patterns come from 3D position, so they continue across edges without seams. Stripe ranges are in meters, in the model frame as built, before the origin moves to the bottom center.

```python
from build123d import *
import gzmesh as gz

def build():
    # A bollard: painted steel post with two reflective bands.
    post = Cylinder(0.06, 1.0, align=(Align.CENTER, Align.CENTER, Align.MIN))
    post = fillet(post.edges().group_by(Axis.Z)[-1], 0.02)
    return gz.part(post, "post", gz.stripes(gz.painted_metal("yellow", wear=0.3), "white", [(0.75, 0.8), (0.85, 0.9)]))
```
