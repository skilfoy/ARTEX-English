#!/usr/bin/env python3
"""Reject non-English leftovers in tracked text files of the English fork.

The check fails on:
- literal Han characters, including in file paths
- the music-note marker U+266A
- a blocked personal name produced by a bad translation of "frontend"
- comment openings that are word-for-word calques of the Chinese source

\\uXXXX escapes are not Han characters and stay allowed. Recorded transcripts
that are not comments are not scanned for those comment openings.
"""

from pathlib import Path
import re
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]
HAN = re.compile(r"[\u3400-\u9fff\uf900-\ufaff\U00020000-\U0003134f]")
JEAN = re.compile(r"\bJe" + r"an\b")
CALQUE = re.compile(r"It" + "'s (?:a |an |the |not )")
NOTE = "\u266a"


def comment_text(line: str) -> str | None:
    """Return the comment portion of a line, or None when the line is not a comment."""
    stripped = line.lstrip()
    if stripped.startswith("//"):
        return stripped[2:]
    if stripped.startswith("/*"):
        return stripped[2:]
    if stripped.startswith("*"):
        return stripped[1:]
    if stripped.startswith("#"):
        return stripped[1:]
    if stripped.startswith("--"):
        return stripped[2:]
    start = 0
    while True:
        index = line.find("//", start)
        if index < 0:
            return None
        if index > 0 and line[index - 1] == ":":
            start = index + 2
            continue
        return line[index + 2 :]


def line_errors(line: str) -> list[str]:
    """Reasons this line fails, without the path. Empty when the line is clean."""
    reasons = []
    if HAN.search(line):
        reasons.append("Han character")
    if NOTE in line:
        reasons.append("music-note marker")
    if JEAN.search(line):
        reasons.append("blocked name")
    comment = comment_text(line)
    if comment is not None and CALQUE.search(comment):
        reasons.append("comment calque")
    return reasons


def scan_path(relative: str, content: str) -> list[str]:
    errors = []
    if HAN.search(relative):
        errors.append(relative)
    for number, line in enumerate(content.splitlines(), 1):
        if line_errors(line):
            errors.append(f"{relative}:{number}")
    return errors


def main() -> int:
    tracked = subprocess.check_output(
        ["git", "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
        cwd=ROOT,
    ).split(b"\0")
    errors = []
    for raw_path in tracked:
        if not raw_path:
            continue
        relative = raw_path.decode("utf-8")
        path = ROOT / relative
        if not path.is_file() or ".terraform" in path.parts:
            continue
        try:
            content = path.read_text(encoding="utf-8")
        except (UnicodeError, OSError):
            if HAN.search(relative):
                errors.append(relative)
            continue
        errors.extend(scan_path(relative, content))
    if errors:
        print("English source check failed:", file=sys.stderr)
        print("\n".join(errors[:100]), file=sys.stderr)
        if len(errors) > 100:
            print(f"... and {len(errors) - 100} more lines", file=sys.stderr)
        return 1
    print("English source scan passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
