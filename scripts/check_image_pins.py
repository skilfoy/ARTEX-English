#!/usr/bin/env python3
"""Every pull of a pinned base image must use the index digest in infra/image-pins.txt."""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
PIN_FILE = ROOT / "infra" / "image-pins.txt"

# Files that pull these images. A mention without the pinned digest fails.
REQUIRED = {
    "postgres:16-alpine": (
        "docker-compose.yml",
        "docker-compose.cloud.yml",
        "install.sh",
        ".github/workflows/ci.yml",
    ),
    "caddy:2-alpine": ("docker-compose.cloud.yml",),
}


def load_pins() -> dict[str, str]:
    pins: dict[str, str] = {}
    for line in PIN_FILE.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        image, digest = line.split()
        if not digest.startswith("sha256:") or len(digest) != 7 + 64:
            print(f"bad digest for {image}: {digest}", file=sys.stderr)
            sys.exit(1)
        pins[image] = digest
    return pins


def main() -> int:
    pins = load_pins()
    missing = [name for name in REQUIRED if name not in pins]
    if missing:
        print("pin file is missing: " + ", ".join(missing), file=sys.stderr)
        return 1

    errors: list[str] = []
    for image, digest in pins.items():
        pinned = f"{image}@{digest}"
        for rel in REQUIRED.get(image, ()):
            text = (ROOT / rel).read_text()
            if pinned not in text:
                errors.append(f"{rel}: expected {pinned}")
            bare = text.count(image) - text.count(pinned)
            if bare:
                errors.append(f"{rel}: {image} appears {bare} time(s) without the pinned digest")
    if errors:
        print("image pin check failed:", file=sys.stderr)
        for err in errors:
            print(f"  {err}", file=sys.stderr)
        return 1
    print(f"image pins ok ({len(pins)} images)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
