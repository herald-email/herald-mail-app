#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

from artifact_io import save_json, save_text
from phase_impact import build_phase_impact, render_phase_impact_markdown


def main() -> int:
    parser = argparse.ArgumentParser(description="Measure the impact of the first Herald GEPA improvement phases.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    impact = build_phase_impact(repo_root)
    json_path = repo_root / ".superpowers" / "autopilot" / "state" / "phase-impact.json"
    markdown_path = repo_root / "docs" / "superpowers" / "gepa-phase-impact.md"
    save_json(json_path, impact)
    save_text(markdown_path, render_phase_impact_markdown(impact))
    print(str(json_path))
    print(str(markdown_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
