#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

from optimizer_common import load_json, save_text, state_dir


def render_table(entries: list[dict]) -> list[str]:
    lines = [
        "| Logged At | Title | Status | Runs | Avg Score | Grounding | Failed Runs | Frontier |",
        "|---|---|---:|---:|---:|---:|---:|---:|",
    ]
    for entry in reversed(entries):
        metrics = entry.get("metrics_snapshot", {})
        grounding_rate = metrics.get("product_truth_grounding_rate")
        grounding_display = f"{grounding_rate:.0%}" if isinstance(grounding_rate, (int, float)) else "n/a"
        lines.append(
            f"| {entry.get('logged_at', '')[:19]} | {entry.get('title', '')} | {entry.get('status', '')} | {metrics.get('recent_run_count', 'n/a')} | {metrics.get('average_score', 'n/a')} | {grounding_display} | {metrics.get('failed_run_count', 'n/a')} | {metrics.get('frontier_count', 'n/a')} |"
        )
    return lines


def main() -> int:
    parser = argparse.ArgumentParser(description="Render the GEPA improvement log to markdown.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    out_dir = state_dir(repo_root)
    log = load_json(out_dir / "improvement-log.json")
    entries = log.get("entries", [])
    markdown_path = repo_root / "docs" / "superpowers" / "gepa-improvement-log.md"

    lines = [
        "# Herald GEPA Improvement Log",
        "",
        "This document is the durable history of changes to the Herald autopilot workflow. It is designed to answer two questions quickly:",
        "",
        "- Are we getting better?",
        "- Do we have enough structured evidence to write about the approach later?",
        "",
        "## Snapshot Table",
        "",
    ]

    if entries:
        lines.extend(render_table(entries))
    else:
        lines.append("No improvement entries have been logged yet.")

    lines.extend(["", "## Entries", ""])
    if entries:
        for entry in reversed(entries):
            metrics = entry.get("metrics_snapshot", {})
            delta = entry.get("delta_from_previous", {})
            recommendation = entry.get("recommended_experiment_at_log_time", {})
            lines.extend(
                [
                    f"### {entry.get('title', '')}",
                    "",
                    f"- Logged at: {entry.get('logged_at', '')}",
                    f"- Status: {entry.get('status', '')}",
                    f"- Kind: {entry.get('kind', '')}",
                    f"- Bottleneck: {entry.get('bottleneck', '')}",
                    f"- Summary: {entry.get('summary', '')}",
                    "",
                    "Metrics at log time:",
                    f"- Recent runs: {metrics.get('recent_run_count', 'n/a')}",
                    f"- Average score: {metrics.get('average_score', 'n/a')}",
                    f"- Average retries: {metrics.get('average_retry_count', 'n/a')}",
                    f"- Failed runs: {metrics.get('failed_run_count', 'n/a')}",
                    f"- Frontier members: {metrics.get('frontier_count', 'n/a')}",
                    f"- Product-truth required runs: {metrics.get('product_truth_required_runs', 'n/a')}",
                    f"- Product-truth grounding rate: {metrics.get('product_truth_grounding_rate', 'n/a')}",
                    f"- Product-truth updated-first runs: {metrics.get('product_truth_updated_first_runs', 'n/a')}",
                ]
            )
            if delta:
                lines.append("Delta from previous entry:")
                lines.extend([f"- {key}: {value:+}" for key, value in delta.items()])
            if entry.get("changes"):
                lines.append("Changes:")
                lines.extend([f"- {item}" for item in entry["changes"]])
            if recommendation:
                lines.append("Recommended experiment at log time:")
                lines.append(
                    f"- `{recommendation.get('name', '')}` ({recommendation.get('value', 'n/a')} value, {recommendation.get('risk', 'n/a')} risk)"
                )
            if entry.get("article_notes"):
                lines.append("Article notes:")
                lines.extend([f"- {item}" for item in entry["article_notes"]])
            if entry.get("followups"):
                lines.append("Follow-ups:")
                lines.extend([f"- {item}" for item in entry["followups"]])
            lines.append("")
    else:
        lines.append("No entries yet.")

    save_text(markdown_path, "\n".join(lines) + "\n")
    print(str(markdown_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
