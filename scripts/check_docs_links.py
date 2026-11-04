#!/usr/bin/env python3
"""Check relative Markdown links in the repository."""

from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SKIP_DIRS = {".git", "node_modules", "dist", ".mimocode"}
LINK_RE = re.compile(r"\[[^\]]*\]\(([^)]+)\)")
FENCE_RE = re.compile(r"```.*?```", re.DOTALL)


def markdown_files() -> list[Path]:
    return sorted(
        p
        for p in ROOT.rglob("*.md")
        if not any(part in SKIP_DIRS for part in p.relative_to(ROOT).parts)
    )


def check(path: Path) -> list[str]:
    text = FENCE_RE.sub("", path.read_text(encoding="utf-8"))
    errors = []
    for raw in LINK_RE.findall(text):
        target = raw.strip()
        if target.startswith(("http://", "https://", "mailto:", "#", "tel:")):
            continue
        target = target.split("#", 1)[0].split("?", 1)[0]
        if not target:
            continue
        resolved = (path.parent / target).resolve()
        if not resolved.exists():
            errors.append(f"{path.relative_to(ROOT)}: broken link {raw!r}")
    return errors


def main() -> int:
    errors = []
    for path in markdown_files():
        errors.extend(check(path))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print(f"checked {len(markdown_files())} markdown files")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
