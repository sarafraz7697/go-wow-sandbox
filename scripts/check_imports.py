#!/usr/bin/env python3
"""Fail if forbidden package import edges appear. Run from repo root."""
from __future__ import annotations

import re
import sys
from pathlib import Path

RULES = [
    ("internal/domain/", (
        "internal/protocol/",
        "internal/storage/",
        "internal/application/",
        "internal/app/",
        "internal/world/",
    )),
    ("internal/application/", (
        "internal/protocol/",
        "internal/storage/",
        "internal/app/",
        "internal/world/",
    )),
    ("internal/world/", (
        "internal/protocol/",
        "internal/storage/",
        "internal/application/",
        "internal/app/",
    )),
    ("internal/protocol/", (
        "internal/storage/characters",
        "internal/storage/realmd",
        "internal/storage/world",
        "internal/world/",
    )),
    ("internal/storage/", (
        "internal/protocol/",
    )),
]


def main() -> int:
    root = Path(".")
    bad = []
    for path in root.rglob("*.go"):
        rel = path.as_posix()
        if any(x in rel for x in ("/vendor/", "/bin/", "/.git/")):
            continue
        text = path.read_text(encoding="utf-8", errors="replace")
        imports = re.findall(r'"wowserver/(internal/[^"]+)"', text)
        for imp in imports:
            for imp_prefix, forbidden_list in RULES:
                if not rel.startswith(imp_prefix):
                    continue
                for fb in forbidden_list:
                    if imp == fb.rstrip("/") or imp.startswith(fb):
                        bad.append((rel, imp, fb))
    bad = sorted(set(bad))
    if bad:
        print("forbidden imports:")
        for rel, imp, fb in bad:
            print("  %s imports %s  (forbidden: %s)" % (rel, imp, fb))
        return 1
    print("import rules ok")
    return 0


if __name__ == "__main__":
    sys.exit(main())
