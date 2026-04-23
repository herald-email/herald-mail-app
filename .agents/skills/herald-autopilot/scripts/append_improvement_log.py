#!/usr/bin/env python3
from __future__ import annotations

import argparse
import uuid
from pathlib import Path

from optimizer_common import load_json, now_utc, save_json, state_dir


def compute_metrics(summary: dict, frontier: dict, brief: dict) -> dict:
    return {
        "recent_run_count": int(summary.get("total_runs", 0)),
        "average_score": summary.get("average_score"),
        "average_retry_count": summary.get("average_retry_count"),
        "failed_run_count": int(summary.get("status_counts", {}).get("failed", 0)),
        "frontier_count": int(frontier.get("frontier_count", 0)),
        "top_recommendation": brief.get("recommended_experiment", {}).get("name", ""),
    }


def metric_delta(previous: dict | None, current: dict) -> dict:
    if not previous:
        return {}
    delta = {}
    for key in ("recent_run_count", "average_score", "average_retry_count", "failed_run_count", "frontier_count"):
        prev_value = previous.get(key)
        curr_value = current.get(key)
        if isinstance(prev_value, (int, float)) and isinstance(curr_value, (int, float)):
            delta[key] = curr_value - prev_value
    return delta


def main() -> int:
    parser = argparse.ArgumentParser(description="Append a GEPA improvement-history entry.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    parser.add_argument("--title", required=True, help="Improvement title")
    parser.add_argument("--summary", required=True, help="One-paragraph summary of the change")
    parser.add_argument("--kind", default="workflow-improvement", help="Improvement kind")
    parser.add_argument("--status", default="applied", choices=["planned", "applied", "validated", "reconstructed"], help="Improvement status")
    parser.add_argument("--bottleneck", default="", help="Bottleneck addressed")
    parser.add_argument("--change", action="append", default=[], help="Concrete change that was made")
    parser.add_argument("--article-note", action="append", default=[], help="Article-friendly note or takeaway")
    parser.add_argument("--followup", action="append", default=[], help="Follow-up idea")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    out_dir = state_dir(repo_root)
    summary = load_json(out_dir / "recent-run-summary.json")
    frontier = load_json(out_dir / "frontier.json")
    brief = load_json(out_dir / "improvement-brief.json")

    log_path = out_dir / "improvement-log.json"
    if log_path.exists():
        log = load_json(log_path)
    else:
        log = {"schema_version": "herald-autopilot.improvement-log.v1", "updated_at": now_utc(), "entries": []}

    current_metrics = compute_metrics(summary, frontier, brief)
    previous_entry = log["entries"][-1] if log["entries"] else None
    previous_metrics = previous_entry.get("metrics_snapshot") if previous_entry else None

    entry = {
        "id": str(uuid.uuid4()),
        "logged_at": now_utc(),
        "title": args.title,
        "kind": args.kind,
        "status": args.status,
        "summary": args.summary,
        "bottleneck": args.bottleneck or brief.get("current_bottleneck", ""),
        "changes": args.change,
        "article_notes": args.article_note,
        "followups": args.followup,
        "metrics_snapshot": current_metrics,
        "delta_from_previous": metric_delta(previous_metrics, current_metrics),
        "recommended_experiment_at_log_time": brief.get("recommended_experiment", {}),
        "evidence_paths": {
            "recent_run_summary": str(out_dir / "recent-run-summary.json"),
            "frontier": str(out_dir / "frontier.json"),
            "feedback_patterns": str(out_dir / "feedback-patterns.json"),
            "improvement_brief": str(out_dir / "improvement-brief.json"),
        },
    }

    log["entries"].append(entry)
    log["updated_at"] = now_utc()
    save_json(log_path, log)
    print(str(log_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
