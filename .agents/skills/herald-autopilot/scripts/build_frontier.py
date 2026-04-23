#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

from optimizer_common import list_runs, now_utc, save_json, save_text, state_dir


def metrics_for(record) -> dict[str, float]:
    score = record.score or {}
    axes = score.get("axes", {})
    counts = score.get("counts", {})
    return {
        "verification_completeness": float(axes.get("verification_completeness", 0.0)),
        "overall_score": float(score.get("overall_score", 0)),
        "retry_count": float(counts.get("retry_count", record.run.get("metrics", {}).get("retry_count", 0))),
        "followup_needed": float(score.get("pareto_axes", {}).get("followup_needed", 1 if record.run.get("metrics", {}).get("human_followup_needed", False) else 0)),
    }


def dominates(left: dict[str, float], right: dict[str, float]) -> bool:
    maximize = ("verification_completeness", "overall_score")
    minimize = ("retry_count", "followup_needed")
    not_worse = all(left[key] >= right[key] for key in maximize) and all(left[key] <= right[key] for key in minimize)
    strictly_better = any(left[key] > right[key] for key in maximize) or any(left[key] < right[key] for key in minimize)
    return not_worse and strictly_better


def main() -> int:
    parser = argparse.ArgumentParser(description="Build a lightweight Pareto frontier over Herald Autopilot runs.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    parser.add_argument("--limit", type=int, default=30, help="Maximum number of scored runs to consider")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    records = [record for record in list_runs(repo_root, limit=args.limit) if record.score is not None]
    frontier = []

    for candidate in records:
        candidate_metrics = metrics_for(candidate)
        dominated = False
        for other in records:
            if other.run_id == candidate.run_id:
                continue
            if dominates(metrics_for(other), candidate_metrics):
                dominated = True
                break
        if not dominated:
            frontier.append(
                {
                    "run_id": candidate.run_id,
                    "status": candidate.run.get("status", "unknown"),
                    "task": candidate.run.get("task", {}).get("request", ""),
                    "metrics": candidate_metrics,
                }
            )

    result = {
        "generated_at": now_utc(),
        "candidate_count": len(records),
        "frontier_count": len(frontier),
        "frontier": frontier,
    }

    out_dir = state_dir(repo_root)
    save_json(out_dir / "frontier.json", result)

    lines = [
        "# Frontier",
        "",
        f"- Generated at: {result['generated_at']}",
        f"- Candidates considered: {result['candidate_count']}",
        f"- Frontier members: {result['frontier_count']}",
        "",
        "## Members",
    ]
    if frontier:
        for item in frontier:
            metrics = item["metrics"]
            lines.append(
                f"- {item['run_id']}: score={metrics['overall_score']}, verification={metrics['verification_completeness']}, retries={metrics['retry_count']}, followup={metrics['followup_needed']}"
            )
    else:
        lines.append("- none")

    save_text(out_dir / "frontier.md", "\n".join(lines) + "\n")
    print(str(out_dir / "frontier.json"))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
