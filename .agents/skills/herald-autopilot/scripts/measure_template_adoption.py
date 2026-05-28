#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

from artifact_io import save_json, save_text
from template_adoption import build_template_adoption, render_template_adoption_markdown


def main() -> int:
    parser = argparse.ArgumentParser(description="Measure Herald GEPA remediation-template adoption across published runs.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    parser.add_argument("--limit", type=int, default=None, help="Optional maximum number of recent runs to analyze")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    adoption = build_template_adoption(repo_root, limit=args.limit)
    json_path = repo_root / ".superpowers" / "autopilot" / "state" / "template-adoption.json"
    markdown_path = repo_root / "docs" / "superpowers" / "gepa-template-adoption.md"
    save_json(json_path, adoption)
    save_text(markdown_path, render_template_adoption_markdown(adoption))
    print(str(json_path))
    print(str(markdown_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
