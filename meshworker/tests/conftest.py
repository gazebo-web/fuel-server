import json
import os
import sys

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from gzmesh import run  # noqa: E402


@pytest.fixture
def build(tmp_path):
    """Run a job through the pipeline; returns (exit code, report, out dir)."""
    def _build(code, **job):
        job.setdefault("name", "thing")
        job["code"] = code
        path = tmp_path / "job.json"
        path.write_text(json.dumps(job))
        out = tmp_path / "out"
        code_ = run.main([str(path), str(out)])
        return code_, json.loads((out / "report.json").read_text()), out
    return _build
