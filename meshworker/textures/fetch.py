"""Fetch the CC0 texture sets in sets.json from ambientCG into a directory:

    python fetch.py OUT_DIR [CACHE_DIR]

OUT_DIR/<name>/color.jpg, roughness.jpg and (where the set has one)
metalness.jpg. Every download is checked against its pinned SHA-256. The
worker image runs this at build time, so the textures aren't in git;
CACHE_DIR keeps the zips between runs.
"""

import hashlib
import io
import json
import os
import sys
import urllib.request
import zipfile

HERE = os.path.dirname(os.path.abspath(__file__))
MAPS = {"_Color.jpg": "color.jpg", "_Roughness.jpg": "roughness.jpg", "_Metalness.jpg": "metalness.jpg"}


def fetch(out: str, cache: str | None = None):
    spec = json.load(open(os.path.join(HERE, "sets.json")))
    for name, s in spec["sets"].items():
        target = os.path.join(out, name)
        if os.path.exists(os.path.join(target, "color.jpg")):
            continue
        zname = f"{s['asset']}_{spec['resolution']}.zip"
        data = None
        if cache and os.path.exists(os.path.join(cache, zname)):
            data = open(os.path.join(cache, zname), "rb").read()
        if data is None:
            url = f"https://ambientcg.com/get?file={zname}"
            req = urllib.request.Request(url, headers={"User-Agent": "gazebo-meshworker"})
            data = urllib.request.urlopen(req, timeout=120).read()
            if cache:
                os.makedirs(cache, exist_ok=True)
                open(os.path.join(cache, zname), "wb").write(data)
        digest = hashlib.sha256(data).hexdigest()
        if digest != s["sha256"]:
            raise SystemExit(f"{zname}: sha256 {digest}, want {s['sha256']}")
        os.makedirs(target, exist_ok=True)
        z = zipfile.ZipFile(io.BytesIO(data))
        for member in z.namelist():
            for suffix, dest in MAPS.items():
                if member.endswith(suffix):
                    with open(os.path.join(target, dest), "wb") as f:
                        f.write(z.read(member))
        print(f"{name}: {s['asset']}", file=sys.stderr)


if __name__ == "__main__":
    fetch(sys.argv[1], sys.argv[2] if len(sys.argv) > 2 else None)
