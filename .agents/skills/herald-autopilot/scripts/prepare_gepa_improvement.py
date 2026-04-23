#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

from optimizer_common import load_json, now_utc, save_json, save_text, state_dir


def main() -> int:
    parser = argparse.ArgumentParser(description="Prepare a GEPA improvement brief from recent optimizer artifacts.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    out_dir = state_dir(repo_root)
    summary = load_json(out_dir / "recent-run-summary.json")
    frontier = load_json(out_dir / "frontier.json")
    patterns = load_json(out_dir / "feedback-patterns.json")

    total_runs = int(summary.get("total_runs", 0))
    failed_runs = int(summary.get("status_counts", {}).get("failed", 0))
    real_task_gap = total_runs < 5
    top_failure = patterns.get("top_failing_evidence", [])
    top_failure_name = top_failure[0]["name"] if top_failure else ""

    if real_task_gap:
        bottleneck = "The workflow does not yet have enough real bug or feature runs to justify aggressive self-modification."
        recommendation = {
            "name": "auto-ledger-and-state-sync",
            "why": "The safest next step is to reduce operator effort and accumulate higher-quality history before adding challenger worktrees.",
            "risk": "low",
            "value": "high",
        }
    elif top_failure_name:
        bottleneck = f"The most repeated failing evidence is `{top_failure_name}`, which suggests a reusable verification or remediation pattern is missing."
        recommendation = {
            "name": f"template-{top_failure_name}-feedback",
            "why": "Repeated failure shapes are strong candidates for reusable feedback templates and tighter gate guidance.",
            "risk": "low",
            "value": "medium",
        }
    elif failed_runs > 0:
        bottleneck = "The workflow is still losing confidence on some runs because failures are not being converted into enough reusable policy."
        recommendation = {
            "name": "risk-ranked-remediation-policies",
            "why": "Codifying repeated failure classes is safer than jumping straight to challenger worktrees.",
            "risk": "medium",
            "value": "medium",
        }
    else:
        bottleneck = "The workflow has healthy bootstrap signals and can start exploring limited candidate comparison."
        recommendation = {
            "name": "two-candidate-worktree-trial",
            "why": "A narrow challenger-worktree experiment is the next step toward true GEPA-style search once the baseline is trustworthy.",
            "risk": "medium",
            "value": "high",
        }

    brief = {
        "generated_at": now_utc(),
        "current_bottleneck": bottleneck,
        "recommended_experiment": recommendation,
        "evidence": {
            "recent_run_count": total_runs,
            "frontier_count": frontier.get("frontier_count", 0),
            "failed_run_count": failed_runs,
            "top_failing_evidence": patterns.get("top_failing_evidence", []),
            "top_risks": patterns.get("top_risks", []),
        },
        "secondary_experiments": [
            "frontier-backed candidate comparison",
            "feedback-template mining",
            "verification cost measurement",
        ],
    }

    save_json(out_dir / "improvement-brief.json", brief)
    save_json(
        out_dir / "optimizer-state.json",
        {
            "generated_at": brief["generated_at"],
            "summary_path": str(out_dir / "recent-run-summary.json"),
            "frontier_path": str(out_dir / "frontier.json"),
            "patterns_path": str(out_dir / "feedback-patterns.json"),
            "brief_path": str(out_dir / "improvement-brief.json"),
        },
    )

    markdown = "\n".join(
        [
            "# GEPA Improvement Brief",
            "",
            f"- Generated at: {brief['generated_at']}",
            "",
            "## Current Bottleneck",
            brief["current_bottleneck"],
            "",
            "## Recommended Experiment",
            f"- Name: {recommendation['name']}",
            f"- Why: {recommendation['why']}",
            f"- Value: {recommendation['value']}",
            f"- Risk: {recommendation['risk']}",
            "",
            "## Evidence",
            f"- Recent runs: {brief['evidence']['recent_run_count']}",
            f"- Frontier members: {brief['evidence']['frontier_count']}",
            f"- Failed runs: {brief['evidence']['failed_run_count']}",
        ]
        + [f"- Top failing evidence: {item['name']} ({item['count']})" for item in brief["evidence"]["top_failing_evidence"][:3]]
        + [""]
    )
    save_text(out_dir / "improvement-brief.md", markdown)
    print(str(out_dir / "improvement-brief.json"))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
