#!/usr/bin/env python3
"""The detection pack names the same images in every format."""

from __future__ import annotations

import csv
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
IMAGES = ("tomicdd/artex", "congdonglinux/artex")
REQUIRED = (
    "README.md",
    "sigma/artex_console_process.yml",
    "sigma/artex_bad_image.yml",
    "suricata/artex.rules",
    "yara/artex_images.yar",
    "indicators/images.csv",
    "tests/test_pack.py",
)


def main() -> int:
    missing = [name for name in REQUIRED if not (ROOT / name).is_file()]
    if missing:
        print("missing files:", ", ".join(missing), file=sys.stderr)
        return 1

    with (ROOT / "indicators/images.csv").open(newline="") as handle:
        rows = list(csv.DictReader(handle))
    names = {row["name"] for row in rows}
    if names != set(IMAGES):
        print(f"csv images {sorted(names)} != {list(IMAGES)}", file=sys.stderr)
        return 1
    if any(row["status"] != "do-not-run" for row in rows):
        print("every image row must be do-not-run", file=sys.stderr)
        return 1

    named = (ROOT / "sigma/artex_bad_image.yml").read_text()
    named += (ROOT / "suricata/artex.rules").read_text()
    named += (ROOT / "yara/artex_images.yar").read_text()
    for image in IMAGES:
        if image not in named:
            print(f"{image} missing from sigma, suricata, or yara", file=sys.stderr)
            return 1

    console = (ROOT / "sigma/artex_console_process.yml").read_text()
    if "tomicdd" in console or "congdonglinux" in console:
        print("console rule must not embed the bad image names", file=sys.stderr)
        return 1
    print("detection pack ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
