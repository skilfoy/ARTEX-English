#!/usr/bin/env python3
"""Fail the build if a tracked file pulls a known-bad ARTEX image.

The markers are documented on purpose in a short allowlist. Anywhere else
they are a supply-chain failure, not a citation.
"""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

# Image names published as backdoored in November 2026. These are identifiers,
# not instructions.
MARKERS = (
    "tomicdd/artex",
    "congdonglinux/artex",
)

# Files that are allowed to name the bad images so operators can reject them.
ALLOWED = {
    "scripts/check_supply_chain.py",
    "SECURITY.md",
    "README.md",
    "CHANGELOG.md",
    "DISTRIBUTION.md",
}


def tracked_files() -> list[Path]:
    skip_dirs = {".git", "web/node_modules", "node_modules"}
    out: list[Path] = []
    for path in ROOT.rglob("*"):
        if not path.is_file():
            continue
        rel = path.relative_to(ROOT).as_posix()
        if any(rel == d or rel.startswith(d + "/") for d in skip_dirs):
            continue
        if rel in ALLOWED or rel.startswith("detections/"):
            continue
        out.append(path)
    return out


def main() -> int:
    hits: list[str] = []
    for path in tracked_files():
        try:
            text = path.read_text(encoding="utf-8", errors="ignore")
        except OSError:
            continue
        rel = path.relative_to(ROOT).as_posix()
        for marker in MARKERS:
            if marker in text.lower() or marker in text:
                hits.append(f"{rel}: contains {marker}")
    if hits:
        print("known-bad image reference outside the allowlist:", file=sys.stderr)
        for hit in hits:
            print(f"  {hit}", file=sys.stderr)
        return 1
    print(f"supply-chain check ok ({len(list(tracked_files()))} files, markers absent)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
