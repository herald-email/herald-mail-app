#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

from optimizer_common import load_json, state_dir


BEGIN_MARKER = "<!-- AUTOGEN:BEGIN -->"
END_MARKER = "<!-- AUTOGEN:END -->"


def replace_block(content: str, replacement: str) -> str:
    start = content.find(BEGIN_MARKER)
    end = content.find(END_MARKER)
    if start == -1 or end == -1 or end < start:
        raise ValueError("Ledger markers not found")
    prefix = content[: start + len(BEGIN_MARKER)]
    suffix = content[end:]
    return prefix + "\n" + replacement.rstrip() + "\n" + suffix


def main() -> int:
    parser = argparse.ArgumentParser(description="Sync the GEPA evolution ledger from optimizer artifacts.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    out_dir = state_dir(repo_root)
    summary = load_json(out_dir / "recent-run-summary.json")
    frontier = load_json(out_dir / "frontier.json")
    patterns = load_json(out_dir / "feedback-patterns.json")
    brief = load_json(out_dir / "improvement-brief.json")
    queue_path = out_dir / "pending-approvals.json"
    queue = load_json(queue_path) if queue_path.exists() else None
    phase_impact_path = out_dir / "phase-impact.json"
    phase_impact = load_json(phase_impact_path) if phase_impact_path.exists() else None
    ledger_path = repo_root / "docs" / "superpowers" / "gepa-evolution.md"
    content = ledger_path.read_text(encoding="utf-8")

    lines = [
        f"- [x] Auto snapshot generated at {brief['generated_at']}.",
        f"- [x] Recent runs analyzed: {summary['total_runs']}.",
        f"- [x] Frontier members available: {frontier['frontier_count']}.",
    ]
    if patterns.get("top_failing_evidence"):
        item = patterns["top_failing_evidence"][0]
        lines.append(f"- [x] Most repeated failing evidence: `{item['name']}` ({item['count']} occurrences).")
    else:
        lines.append("- [ ] No repeated failing evidence has been observed yet.")
    recommendation = brief["recommended_experiment"]
    lines.append(
        f"- [x] Current top recommended experiment: `{recommendation['name']}` ({recommendation['value']} value, {recommendation['risk']} risk)."
    )
    if queue:
        queue_summary = queue.get("summary", {})
        lines.append(
            f"- [x] Pending-approval queue: {queue_summary.get('pending', 0)} pending, {queue_summary.get('approved', 0)} approved, {queue_summary.get('implemented', 0)} implemented."
        )
    if phase_impact:
        current_real = phase_impact.get("real_task_current_vs_baseline", {}).get("current_metrics", {})
        lines.append(
            f"- [x] Phase-impact report: {current_real.get('run_count', 0)} post-Phase 1 real bug/feature run(s) measured so far."
        )

    updated = replace_block(content, "\n".join(lines))
    ledger_path.write_text(updated, encoding="utf-8")
    print(str(ledger_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
