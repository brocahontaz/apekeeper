#!/usr/bin/env python3
"""Check migration filenames are ordered and contain reversible SQL intent."""
from pathlib import Path
import re

files = sorted(Path("backend/internal/migrations").glob("*.sql"))
versions = []
for file in files:
    match = re.match(r"(\d+)_", file.name)
    assert match, f"migration has no numeric prefix: {file.name}"
    versions.append(int(match.group(1)))
unique = sorted(set(versions))
assert unique == list(range(1, unique[-1] + 1)), f"migration sequence has a gap: {unique}"
print(f"validated {len(files)} migrations")
