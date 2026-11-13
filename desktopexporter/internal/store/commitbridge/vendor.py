#!/usr/bin/env python3
"""Copy the bridge's transitive headers from the pinned DuckDB source archive."""
import hashlib
from pathlib import Path
import re
import sys
import tarfile

SOURCE = "d54cbbe3df17e8e5ce68312f483b4155836112aa"
SHA256 = "93e3d1f9c84b6ec3493237d21565c01c2c19b9862e54e5552d3d7999ccc3bbb4"

archive = Path(sys.argv[1])
if hashlib.sha256(archive.read_bytes()).hexdigest() != SHA256:
    raise SystemExit("DuckDB source archive checksum does not match the pinned archive")

root = Path(__file__).resolve().parent
with tarfile.open(archive) as tar:
    prefix = f"duckdb-{SOURCE}/"
    headers = {
        member.name.removeprefix(prefix + "src/include/"): member
        for member in tar.getmembers()
        if member.isfile() and member.name.startswith(prefix + "src/include/")
    }
    queue = re.findall(r'#include "([^"]+)"', (root / "bridge.cpp").read_text())
    seen = set()
    while queue:
        name = queue.pop()
        if name in seen:
            continue
        seen.add(name)
        data = tar.extractfile(headers[name]).read()
        target = root / "include" / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
        queue.extend(dep for dep in re.findall(r'#include ["<]([^">]+)[">]', data.decode()) if dep in headers)
    (root / "LICENSE.duckdb").write_bytes(tar.extractfile(prefix + "LICENSE").read())
print(f"Copied {len(seen)} DuckDB headers from {SOURCE}")
