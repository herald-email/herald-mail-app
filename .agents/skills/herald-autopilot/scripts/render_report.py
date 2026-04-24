#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
from pathlib import Path


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def artifact_path(run: dict, artifact: str) -> str:
    if not artifact:
        return ""
    path = Path(artifact)
    if not path.is_absolute():
        path = Path(run["paths"]["repo_root"]) / path
    return str(path)


def visual_screenshot_sections(run: dict, results: list[dict]) -> list[str]:
    screenshots = []
    for item in results:
        artifact = item.get("artifact", "")
        if item.get("kind") != "screenshot" or not artifact.lower().endswith(".png"):
            continue
        summary = item.get("summary", "")
        key = f"{summary} {artifact}".lower()
        if "before" in key:
            group = "Before"
        elif "after" in key:
            group = "After"
        else:
            continue
        screenshots.append((group, summary or group, artifact_path(run, artifact)))

    if not screenshots:
        return []

    lines = ["", "## Visual Evidence"]
    for group in ("Before", "After"):
        group_items = [item for item in screenshots if item[0] == group]
        if not group_items:
            continue
        lines.extend(["", f"### {group}"])
        for _, summary, path in group_items:
            lines.extend([f"![{summary}]({path})", ""])
    return lines[:-1] if lines[-1] == "" else lines


def main() -> int:
    parser = argparse.ArgumentParser(description="Render a Herald Autopilot report from a run folder.")
    parser.add_argument("--run-dir", required=True, help="Path to the run directory")
    args = parser.parse_args()

    run_dir = Path(args.run_dir).resolve()
    run = load_json(run_dir / "run.json")
    score_path = run_dir / "score.json"
    score = load_json(score_path) if score_path.exists() else None
    report_path = Path(run["paths"]["report_path"])
    report_path.parent.mkdir(parents=True, exist_ok=True)

    required = set(run["verification"].get("required_gates", []))
    results = run["verification"].get("results", [])
    manifest_path = run_dir / "evidence" / "manifest.json"
    evidence_items = load_json(manifest_path) if manifest_path.exists() else results
    required_results = [item for item in results if item["gate"] in required]
    skipped = [item for item in results if item["status"] == "skip"]
    reflections = sorted((run_dir / "reflections").glob("*.json"))
    remaining_risks = run["outcome"].get("remaining_risks", [])
    feedback = score["feedback"] if score else run.get("latest_feedback", [])
    product_truth = run.get("product_truth", {})
    truth_sources = product_truth.get("sources", [])
    docs_updated = product_truth.get("docs_updated", [])

    lines = [
        f"# Herald Autopilot Report — {run['run_id']}",
        "",
        "## Task",
        run["task"]["request"],
        "",
        f"- Type: {run['task']['type']}",
        f"- Surfaces: {', '.join(run['task']['surfaces']) if run['task']['surfaces'] else 'none recorded'}",
        f"- Status: {run['status']}",
        f"- Branch: `{run['paths']['branch']}`",
        f"- Worktree: `{run['paths']['worktree']}`",
        "",
        "## Plan Summary",
        run["plan"].get("summary") or "No plan summary recorded.",
        "",
        "## Product Truth",
        f"- Required: {'yes' if product_truth.get('required') else 'no'}",
        f"- Status: {product_truth.get('status', 'not recorded')}",
        f"- Summary: {product_truth.get('summary') or 'No grounding summary recorded.'}",
        f"- Sources: {', '.join(truth_sources) if truth_sources else 'none recorded'}",
        f"- Docs updated first: {', '.join(docs_updated) if docs_updated else 'none recorded'}",
        "",
        "## Outcome",
        run["outcome"].get("summary") or "No outcome summary recorded.",
    ]

    lines.extend(visual_screenshot_sections(run, evidence_items))
    lines.extend(["", "## Verification"])

    if required_results:
        for item in required_results:
            lines.append(f"- [{item['status']}] `{item['gate']}` — {item['summary']}")
    else:
        lines.append("- No required verification gates recorded.")

    if skipped:
        lines.extend(
            [
                "",
                "## Skipped Gates",
                *[f"- `{item.get('gate') or 'ungated'}` — {item['summary']}" for item in skipped],
            ]
        )

    lines.extend(
        [
            "",
            "## GEPA Reflection",
            f"- Reflections recorded: {len(reflections)}",
        ]
    )
    if feedback:
        lines.extend([f"- {item}" for item in feedback])
    else:
        lines.append("- No reflection feedback recorded.")

    lines.extend(["", "## Risks"])
    if remaining_risks:
        lines.extend([f"- {item}" for item in remaining_risks])
    else:
        lines.append("- No remaining risks recorded.")

    if score:
        lines.extend(
            [
                "",
                "## Score",
                f"- Overall score: {score['overall_score']}",
                f"- Status: {score['status']}",
                f"- Required gates passed: {score['counts']['required_passed']}/{score['counts']['required_gates']}",
                f"- Retry count: {score['counts']['retry_count']}",
            ]
        )

    markdown = "\n".join(lines) + "\n"
    report_path.write_text(markdown, encoding="utf-8")
    (run_dir / "summary.md").write_text(markdown, encoding="utf-8")
    print(str(report_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
