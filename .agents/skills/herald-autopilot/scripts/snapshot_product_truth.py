#!/usr/bin/env python3
from __future__ import annotations

import argparse
import re
from pathlib import Path

from optimizer_common import now_utc, save_json, save_text, state_dir


def extract_title(path: Path) -> str:
    for line in path.read_text(encoding="utf-8").splitlines():
        if line.startswith("# "):
            return line[2:].strip()
    return path.stem


def extract_top_sections(path: Path, limit: int = 8) -> list[str]:
    sections: list[str] = []
    pattern = re.compile(r"^##\s+(.*)")
    for line in path.read_text(encoding="utf-8").splitlines():
        match = pattern.match(line)
        if match:
            sections.append(match.group(1).strip())
        if len(sections) >= limit:
            break
    return sections


def spec_inventory(spec_dir: Path) -> list[dict]:
    items = []
    if not spec_dir.exists():
        return items
    for path in sorted(spec_dir.glob("*.md")):
        stat = path.stat()
        items.append(
            {
                "path": str(path),
                "title": extract_title(path),
                "top_sections": extract_top_sections(path, limit=6),
                "updated_at": stat.st_mtime,
            }
        )
    return items


def main() -> int:
    parser = argparse.ArgumentParser(description="Snapshot the current product-definition source of truth for Herald Autopilot.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    vision = repo_root / "VISION.md"
    architecture = repo_root / "ARCHITECTURE.md"
    specs_dir = repo_root / "docs" / "superpowers" / "specs"
    out_dir = state_dir(repo_root)

    specs = spec_inventory(specs_dir)
    snapshot = {
        "generated_at": now_utc(),
        "canonical_sources": [
            {
                "path": str(vision),
                "role": "product-vision",
                "title": extract_title(vision),
                "top_sections": extract_top_sections(vision),
            },
            {
                "path": str(architecture),
                "role": "high-level-architecture",
                "title": extract_title(architecture),
                "top_sections": extract_top_sections(architecture),
            },
        ],
        "spec_count": len(specs),
        "specs": specs,
        "grounding_rule": "For product behavior, agents should read VISION.md, ARCHITECTURE.md, and relevant specs before inferring intent from code or screenshots.",
    }

    save_json(out_dir / "product-truth.json", snapshot)

    lines = [
        "# Product Truth Snapshot",
        "",
        f"- Generated at: {snapshot['generated_at']}",
        f"- Canonical sources: {len(snapshot['canonical_sources'])}",
        f"- Specs tracked: {snapshot['spec_count']}",
        "",
        "## Canonical Sources",
    ]
    for item in snapshot["canonical_sources"]:
        lines.append(f"- {item['role']}: `{item['path']}`")
        for section in item["top_sections"][:5]:
            lines.append(f"  - {section}")

    lines.extend(["", "## Specs"])
    if specs:
        for spec in specs:
            lines.append(f"- {spec['title']}: `{spec['path']}`")
    else:
        lines.append("- none")

    lines.extend(["", "## Grounding Rule", snapshot["grounding_rule"]])
    save_text(out_dir / "product-truth.md", "\n".join(lines) + "\n")
    print(str(out_dir / "product-truth.json"))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
