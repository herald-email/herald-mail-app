#!/usr/bin/env python3
from __future__ import annotations

import argparse
from collections import Counter
from pathlib import Path

from optimizer_common import list_runs, now_utc, save_json, save_text, state_dir


def main() -> int:
    parser = argparse.ArgumentParser(description="Summarize recent Herald Autopilot runs.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    parser.add_argument("--limit", type=int, default=20, help="Maximum number of recent runs to analyze")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    runs = list_runs(repo_root, limit=args.limit)
    status_counts: Counter[str] = Counter()
    type_counts: Counter[str] = Counter()
    surface_counts: Counter[str] = Counter()
    failure_gate_counts: Counter[str] = Counter()
    risk_counts: Counter[str] = Counter()
    product_truth_status_counts: Counter[str] = Counter()
    score_values: list[int] = []
    retry_counts: list[int] = []
    product_truth_required_runs = 0
    product_truth_grounded_runs = 0
    product_truth_updated_first_runs = 0

    run_items = []
    for record in runs:
        run = record.run
        score = record.score or {}
        status_counts[run.get("status", "unknown")] += 1
        type_counts[run.get("task", {}).get("type", "unknown")] += 1
        for surface in run.get("task", {}).get("surfaces", []):
            surface_counts[surface] += 1
        for result in run.get("verification", {}).get("results", []):
            if result.get("status") == "fail" and result.get("gate"):
                failure_gate_counts[result["gate"]] += 1
        for risk in run.get("outcome", {}).get("remaining_risks", []):
            risk_counts[risk] += 1
        product_truth = run.get("product_truth", {})
        truth_status = product_truth.get("status", "not-recorded")
        product_truth_status_counts[truth_status] += 1
        if product_truth.get("required", False):
            product_truth_required_runs += 1
            if truth_status in {"consulted", "updated-first"}:
                product_truth_grounded_runs += 1
            if truth_status == "updated-first":
                product_truth_updated_first_runs += 1
        if record.score is not None:
            score_values.append(int(score.get("overall_score", 0)))
        retry_counts.append(int(run.get("metrics", {}).get("retry_count", 0)))
        run_items.append(
            {
                "run_id": record.run_id,
                "status": run.get("status", "unknown"),
                "task": run.get("task", {}).get("request", ""),
                "type": run.get("task", {}).get("type", "unknown"),
                "surfaces": run.get("task", {}).get("surfaces", []),
                "score": score.get("overall_score"),
                "retry_count": run.get("metrics", {}).get("retry_count", 0),
                "product_truth_status": truth_status,
                "product_truth_required": bool(product_truth.get("required", False)),
            }
        )

    summary = {
        "generated_at": now_utc(),
        "limit": args.limit,
        "total_runs": len(runs),
        "status_counts": dict(status_counts),
        "type_counts": dict(type_counts),
        "surface_counts": dict(surface_counts),
        "average_score": (sum(score_values) / len(score_values)) if score_values else None,
        "average_retry_count": (sum(retry_counts) / len(retry_counts)) if retry_counts else 0,
        "top_failure_gates": [{"name": name, "count": count} for name, count in failure_gate_counts.most_common(5)],
        "top_risks": [{"name": name, "count": count} for name, count in risk_counts.most_common(5)],
        "product_truth": {
            "required_runs": product_truth_required_runs,
            "grounded_runs": product_truth_grounded_runs,
            "updated_first_runs": product_truth_updated_first_runs,
            "grounding_rate": (product_truth_grounded_runs / product_truth_required_runs) if product_truth_required_runs else None,
            "status_counts": dict(product_truth_status_counts),
        },
        "runs": run_items,
    }

    out_dir = state_dir(repo_root)
    save_json(out_dir / "recent-run-summary.json", summary)

    lines = [
        "# Recent Run Summary",
        "",
        f"- Generated at: {summary['generated_at']}",
        f"- Runs analyzed: {summary['total_runs']}",
        f"- Average score: {summary['average_score'] if summary['average_score'] is not None else 'n/a'}",
        f"- Average retries: {summary['average_retry_count']}",
        "",
        "## Status Counts",
    ]
    if status_counts:
        lines.extend([f"- {name}: {count}" for name, count in status_counts.most_common()])
    else:
        lines.append("- none")

    lines.extend(["", "## Product Truth"])
    lines.append(f"- Required runs: {product_truth_required_runs}")
    lines.append(f"- Grounded runs: {product_truth_grounded_runs}")
    lines.append(f"- Updated-first runs: {product_truth_updated_first_runs}")
    lines.append(
        f"- Grounding rate: {summary['product_truth']['grounding_rate'] if summary['product_truth']['grounding_rate'] is not None else 'n/a'}"
    )
    if product_truth_status_counts:
        lines.extend([f"- Status {name}: {count}" for name, count in product_truth_status_counts.most_common()])

    lines.extend(["", "## Top Failure Gates"])
    if failure_gate_counts:
        lines.extend([f"- {name}: {count}" for name, count in failure_gate_counts.most_common(5)])
    else:
        lines.append("- none")

    lines.extend(["", "## Top Risks"])
    if risk_counts:
        lines.extend([f"- {name}: {count}" for name, count in risk_counts.most_common(5)])
    else:
        lines.append("- none")

    save_text(out_dir / "recent-run-summary.md", "\n".join(lines) + "\n")
    print(str(out_dir / "recent-run-summary.json"))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
