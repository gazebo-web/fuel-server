"""Every example in the agent's build123d reference (aitools/meshref, served
by mesh_api_reference) must build, so the reference can't drift from
what actually works. Set GZMESH_PREVIEW_DIR to keep the previews."""

import json
import os
import re
import shutil
from pathlib import Path

import pytest

import gzmesh as gz

REF = Path(__file__).resolve().parents[2] / "aitools" / "meshref"
BLOCK = re.compile(r"```python\n(.*?)```", re.S)
EXAMPLES = [(f"{doc.stem}_{i}", code) for doc in sorted(REF.glob("*.md"))
            for i, code in enumerate(BLOCK.findall(doc.read_text()))]


def test_reference_has_examples():
    assert len(EXAMPLES) >= 8
    assert {"overview", "positioning", "sketches", "sweep_loft", "fillet_chamfer", "booleans_patterns",
            "materials", "errors"} <= {p.stem for p in REF.glob("*.md")}


@pytest.mark.parametrize("name, code", EXAMPLES, ids=[n for n, _ in EXAMPLES])
def test_example_builds(build, name, code):
    status, report, out = build(code, name="example")
    assert status == 0 and report["ok"], report
    assert not [w for w in report["warnings"] if "overlap" in w], report["warnings"]
    keep = os.environ.get("GZMESH_PREVIEW_DIR")
    if keep:
        os.makedirs(keep, exist_ok=True)
        shutil.copy(out / "preview.png", os.path.join(keep, f"{name}.png"))
        (Path(keep) / f"{name}.json").write_text(json.dumps(report, indent=1))


def test_materials_table_matches_library():
    table = (REF / "materials.md").read_text()
    for name in gz.LIBRARY:
        assert f"gz.{name}(" in table, f"materials.md doesn't document gz.{name}"


def test_texture_table_lists_every_set():
    table = (REF / "materials.md").read_text()
    for name in gz.texture_sets():
        assert f"`{name}`" in table, f"materials.md doesn't list texture set {name}"
