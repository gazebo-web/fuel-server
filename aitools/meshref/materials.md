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
| `gz.texture(name, scale=1.0, tint=None)` | A photo texture set, to scale (below) | the set's |

**Photo textures** are CC0 sets from ambientCG. Their color, roughness and metalness maps are projected onto the shape from three sides, so they need no UVs and stay to real-world scale:
- `scale` multiplies the tile size, for bigger planks or smaller bricks.
- `tint` recolors a set while keeping its detail. Use it for painted metal, plastic, fabric and leather.

**The user's own textures:** an image or texture set uploaded to the project (asset_list, kind image or texture) works as `gz.texture("project:<file name>")`, e.g. `gz.texture("project:brick.zip")`. It tiles every 1 m, so set `scale` to its real size. `mesh_list_materials` with the project name lists them.

| Set | Looks like | Tile (m) | Density (kg/m³) |
|---|---|---|---|
| `wood_planks` | Worn wooden floor planks | 2.0 | 600 |
| `wood_grain` | Light wood with fine straight grain | 1.0 | 600 |
| `brushed_steel` | Brushed steel | 1.0 | 7850 |
| `painted_metal` | Scratched painted metal (tint it) | 1.0 | 7850 |
| `rusty_metal` | Weathered steel with rust streaks | 1.5 | 7850 |
| `concrete` | Smooth poured concrete | 2.0 | 2400 |
| `asphalt` | Road asphalt | 2.0 | 2300 |
| `fabric` | Woven fabric (tint it) | 0.3 | 300 |
| `leather` | Smooth leather (tint it) | 0.5 | 900 |
| `cardboard` | Cardboard | 0.6 | 150 |
| `bricks` | Red brick wall with mortar | 1.4 | 1900 |
| `stone_tiles` | Square stone floor tiles | 1.2 | 2400 |
| `rubber` | Black textured rubber | 0.5 | 1100 |
| `plastic` | Lightly textured plastic (tint it) | 0.5 | 1000 |

Prefer a photo texture where it reads better than a procedural one: wood planks, bricks, worn or rusty metal, tiles.

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

```python
from build123d import *
import gzmesh as gz

def build():
    # A park bench: wooden slats on dark green painted metal legs.
    slats = [Pos(0, y, 0.45) * Box(1.5, 0.08, 0.03) for y in (-0.15, -0.05, 0.05, 0.15)]
    legs = [Pos(x, 0, 0) * Box(0.05, 0.4, 0.43, align=(Align.CENTER, Align.CENTER, Align.MIN)) for x in (-0.6, 0.6)]
    wood = gz.texture("wood_grain", scale=0.5)
    steel = gz.texture("painted_metal", tint="#2e4d32")
    return [gz.part(s, f"slat_{i}", wood) for i, s in enumerate(slats)] + \
           [gz.part(l, f"leg_{i}", steel) for i, l in enumerate(legs)]
```

