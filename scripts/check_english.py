#!/usr/bin/env python3
"""Reject Han characters in tracked text files in the English fork."""

from pathlib import Path
import re
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]
HAN = re.compile(r"[\u3400-\u9fff\uf900-\ufaff\U00020000-\U0003134f]")


def main() -> int:
    tracked = subprocess.check_output(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
        cwd=ROOT,
    ).split(b"\0")
    errors = []
    for raw_path in tracked:
        if not raw_path:
            continue
        path = ROOT / raw_path.decode("utf-8")
        if not path.is_file() or ".terraform" in path.parts:
            continue
        if HAN.search(str(path.relative_to(ROOT))):
            errors.append(str(path.relative_to(ROOT)))
        try:
            content = path.read_text(encoding="utf-8")
        except (UnicodeError, OSError):
            continue
        for number, line in enumerate(content.splitlines(), 1):
            if HAN.search(line):
                errors.append(f"{path.relative_to(ROOT)}:{number}")
    if errors:
        print("Han characters remain in the English source:", file=sys.stderr)
        print("\n".join(errors[:100]), file=sys.stderr)
        if len(errors) > 100:
            print(f"... and {len(errors) - 100} more lines", file=sys.stderr)
        return 1
    print("English source scan passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
