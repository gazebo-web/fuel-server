# meshworker

The AI Builder's mesh worker (plan §5 in `docs/ai-builder-models-and-meshes-plan.md`, decisions D-2 and D-9). Agent-written build123d code goes in; a sim-ready Gazebo model folder comes out.

- `gzmesh/`: the pipeline, a Python package. One job runs per sandboxed process: `python -m gzmesh.run JOB.json OUT`.
  - `materials.py`: procedural PBR materials (solid textures, sRGB).
  - `parts.py`: the code contract (`build()` returns parts).
  - `tessellate.py`: triangles within a budget.
  - `atlas.py`: xatlas plus a single baked base-color/ORM atlas.
  - `collision.py`: a primitive, a hull or a CoACD decomposition.
  - `inertia.py`: exact mass properties from the B-rep (OCCT).
  - `export.py`: GLB, `model.sdf` and `model.config`.
  - `preview.py`: a 2×2 software-rendered sheet.
  - `run.py`: job checks, the import allow-list, the report.
- `spike/`: the C1 format spike that validated GLB in Gazebo Harmonic, gz-sim 11 and gzweb.
- `tests/`: unit and end-to-end tests: `python -m pytest tests`.

To come in C1:
- the container image (with nsjail), and the Go front end in `cmd/meshworker` (queue, cache, limits);
- the `model_generate` tools in `aitools/`.

## Development

```sh
uv venv --python 3.12 venv && uv pip install --python venv/bin/python -r requirements.txt
PYTHONPATH=. venv/bin/python -m pytest tests
```

GLB files are written Z-up with no root rotation, the convention both Gazebo and gzweb need (see `spike/README.md`). `gltf-transform validate` reports no errors or warnings on them.
