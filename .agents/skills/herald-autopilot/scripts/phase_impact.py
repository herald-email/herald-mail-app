from __future__ import annotations

from pathlib import Path
from typing import Any

from artifact_io import load_json
from optimizer_common import list_runs, now_utc


PHASE_TITLES = {
    "phase1": "Reusable remediation templates for repeated test failures",
    "phase2": "Workflow safety preflight and serialized artifact writes",
    "phase3": "Input-routing safety gate for shortcut-sensitive TUI runs",
    "phase4": "Pending-approval queue for post-publish GEPA suggestions",
}


def load_phase_anchors(repo_root: Path) -> dict[str, str]:
    log_path = repo_root / ".superpowers" / "autopilot" / "state" / "improvement-log.json"
    log = load_json(log_path)
    anchors: dict[str, str] = {}
    by_title = {entry.get("title", ""): entry.get("logged_at", "") for entry in log.get("entries", [])}
    for phase_id, title in PHASE_TITLES.items():
        if by_title.get(title):
            anchors[phase_id] = by_title[title]
    return anchors


def count_skipped_gates(run: dict[str, Any]) -> int:
    latest_by_gate: dict[str, dict[str, Any]] = {}
    skipped_without_gate = 0
    for item in run.get("verification", {}).get("results", []):
        gate = item.get("gate")
        if gate:
            latest_by_gate[gate] = item
        elif item.get("status") == "skip":
            skipped_without_gate += 1
    skipped_with_gate = sum(1 for item in latest_by_gate.values() if item.get("status") == "skip")
    return skipped_with_gate + skipped_without_gate


def run_signals(record) -> dict[str, Any]:
    run = record.run
    questions_asked = len(run.get("plan", {}).get("questions_asked", []))
    human_followup = bool(run.get("metrics", {}).get("human_followup_needed", False))
    publication_actions = run.get("publication", {}).get("actions", [])
    return {
        "run_id": run.get("run_id", ""),
        "created_at": run.get("created_at", ""),
        "task_type": run.get("task", {}).get("type", "unknown"),
        "retry_count": int(run.get("metrics", {}).get("retry_count", 0)),
        "skipped_gates": count_skipped_gates(run),
        "human_followup": 1 if human_followup else 0,
        "questions_asked": questions_asked,
        "clarification_touches": questions_asked + (1 if human_followup else 0),
        "published": 1 if publication_actions else 0,
        "published_with_self_reflection": 1 if publication_actions and (record.run_dir / "self_reflection.json").exists() else 0,
    }


def average(values: list[int | float]) -> float:
    if not values:
        return 0.0
    return sum(values) / len(values)


def compute_metrics(rows: list[dict[str, Any]]) -> dict[str, Any]:
    run_count = len(rows)
    return {
        "run_count": run_count,
        "average_retry_count": average([row["retry_count"] for row in rows]),
        "average_skipped_gates": average([row["skipped_gates"] for row in rows]),
        "human_followup_rate": average([row["human_followup"] for row in rows]),
        "average_questions_asked": average([row["questions_asked"] for row in rows]),
        "average_clarification_touches": average([row["clarification_touches"] for row in rows]),
        "published_run_rate": average([row["published"] for row in rows]),
        "published_reflection_rate": average([row["published_with_self_reflection"] for row in rows]),
    }


def compute_delta(before: dict[str, Any], after: dict[str, Any]) -> dict[str, float] | dict[str, Any]:
    if before.get("run_count", 0) == 0 or after.get("run_count", 0) == 0:
        return {}
    keys = (
        "average_retry_count",
        "average_skipped_gates",
        "human_followup_rate",
        "average_questions_asked",
        "average_clarification_touches",
        "published_run_rate",
        "published_reflection_rate",
    )
    return {key: float(after[key]) - float(before[key]) for key in keys}


def select_window(rows: list[dict[str, Any]], start_at: str | None, end_before: str | None) -> list[dict[str, Any]]:
    selected = []
    for row in rows:
        created_at = row["created_at"]
        if start_at and created_at < start_at:
            continue
        if end_before and created_at >= end_before:
            continue
        selected.append(row)
    return selected


def load_pending_approvals(repo_root: Path) -> dict[str, Any]:
    path = repo_root / ".superpowers" / "autopilot" / "state" / "pending-approvals.json"
    if not path.exists():
        return {
            "total": 0,
            "pending": 0,
            "approved": 0,
            "rejected": 0,
            "implemented": 0,
            "published_runs_analyzed": 0,
        }
    queue = load_json(path)
    return queue.get("summary", {})


def build_findings(
    baseline: dict[str, Any],
    current: dict[str, Any],
    delta: dict[str, Any],
    real_task_baseline: dict[str, Any],
    real_task_current: dict[str, Any],
    phase4: dict[str, Any],
    pending_approvals: dict[str, Any],
) -> list[str]:
    findings: list[str] = []
    if baseline.get("run_count", 0) and current.get("run_count", 0):
        if delta.get("average_retry_count", 0) < 0:
            findings.append(
                f"Average retries dropped from {baseline['average_retry_count']:.2f} before Phase 1 to {current['average_retry_count']:.2f} across post-Phase 1 runs."
            )
        elif delta.get("average_retry_count", 0) > 0:
            findings.append(
                f"Average retries rose from {baseline['average_retry_count']:.2f} before Phase 1 to {current['average_retry_count']:.2f} across post-Phase 1 runs."
            )
        if delta.get("average_skipped_gates", 0) < 0:
            findings.append(
                f"Skipped verification gates fell from {baseline['average_skipped_gates']:.2f} per run to {current['average_skipped_gates']:.2f}."
            )
        elif delta.get("average_skipped_gates", 0) > 0:
            findings.append(
                f"Skipped verification gates rose from {baseline['average_skipped_gates']:.2f} per run to {current['average_skipped_gates']:.2f}."
            )
        if delta.get("average_clarification_touches", 0) < 0:
            findings.append(
                f"Clarification load dropped from {baseline['average_clarification_touches']:.2f} touches per run to {current['average_clarification_touches']:.2f}."
            )
        elif delta.get("average_clarification_touches", 0) > 0:
            findings.append(
                f"Clarification load rose from {baseline['average_clarification_touches']:.2f} touches per run to {current['average_clarification_touches']:.2f}."
            )
    if current.get("run_count", 0) > 0 and real_task_current.get("run_count", 0) == 0:
        findings.append(
            f"All {current['run_count']} post-Phase 1 runs in the current sample are workflow-improvement validations, so we still lack post-improvement bug or feature runs for stronger real-task evidence."
        )
    elif real_task_baseline.get("run_count", 0) and real_task_current.get("run_count", 0):
        findings.append(
            f"Real-task evidence includes {real_task_current['run_count']} post-Phase 1 bug/feature run(s) compared with {real_task_baseline['run_count']} baseline run(s)."
        )
    if phase4.get("run_count", 0) == 0 and pending_approvals.get("total", 0) > 0:
        findings.append(
            f"Phase 4 has no post-implementation task runs yet, but the queue already surfaced {pending_approvals.get('total', 0)} approval items from {pending_approvals.get('published_runs_analyzed', 0)} published runs."
        )
    elif phase4.get("run_count", 0) > 0:
        findings.append(
            f"Phase 4 has {phase4['run_count']} measured post-implementation run(s), so queue visibility can now be compared alongside run-level metrics."
        )
    return findings


def build_phase_impact(repo_root: Path) -> dict[str, Any]:
    repo_root = repo_root.resolve()
    anchors = load_phase_anchors(repo_root)
    rows = [run_signals(record) for record in list_runs(repo_root, limit=None)]
    rows.sort(key=lambda item: item["created_at"])
    real_task_rows = [row for row in rows if row.get("task_type") != "workflow-improvement"]

    windows = {
        "baseline_pre_phase1": select_window(rows, None, anchors.get("phase1")),
        "phase1_templates": select_window(rows, anchors.get("phase1"), anchors.get("phase2")),
        "phase2_workflow_safety": select_window(rows, anchors.get("phase2"), anchors.get("phase3")),
        "phase3_tui_safety": select_window(rows, anchors.get("phase3"), anchors.get("phase4")),
        "phase4_pending_approval_queue": select_window(rows, anchors.get("phase4"), None),
    }
    window_metrics = {
        name: {
            "start_at": None if name == "baseline_pre_phase1" else anchors.get(name.split("_")[0]),
            "metrics": compute_metrics(window_rows),
        }
        for name, window_rows in windows.items()
    }
    baseline_metrics = window_metrics["baseline_pre_phase1"]["metrics"]
    current_rows = select_window(rows, anchors.get("phase1"), None)
    current_metrics = compute_metrics(current_rows)
    delta = compute_delta(baseline_metrics, current_metrics)
    real_task_baseline = compute_metrics(select_window(real_task_rows, None, anchors.get("phase1")))
    real_task_current = compute_metrics(select_window(real_task_rows, anchors.get("phase1"), None))
    pending_approvals = load_pending_approvals(repo_root)
    findings = build_findings(
        baseline_metrics,
        current_metrics,
        delta,
        real_task_baseline,
        real_task_current,
        window_metrics["phase4_pending_approval_queue"]["metrics"],
        pending_approvals,
    )
    return {
        "generated_at": now_utc(),
        "phase_titles": PHASE_TITLES,
        "anchors": anchors,
        "windows": window_metrics,
        "current_vs_baseline": {
            "baseline_metrics": baseline_metrics,
            "current_metrics": current_metrics,
            "delta": delta,
        },
        "real_task_current_vs_baseline": {
            "baseline_metrics": real_task_baseline,
            "current_metrics": real_task_current,
            "delta": compute_delta(real_task_baseline, real_task_current),
        },
        "pending_approvals": pending_approvals,
        "findings": findings,
    }


def render_phase_impact_markdown(impact: dict[str, Any]) -> str:
    lines = [
        "# Herald GEPA Phase Impact",
        "",
        "This document measures the current effect of the first four Herald GEPA improvements using the durable run corpus. It is meant to answer whether retries, skipped gates, and clarification load are trending down before we add more autonomous workflow behavior.",
        "",
        "## Summary",
        "",
        f"- Generated at: {impact.get('generated_at', '')}",
        f"- Baseline runs before Phase 1: {impact['current_vs_baseline']['baseline_metrics']['run_count']}",
        f"- Runs after Phase 1 started: {impact['current_vs_baseline']['current_metrics']['run_count']}",
        f"- Real bug/feature runs after Phase 1 started: {impact['real_task_current_vs_baseline']['current_metrics']['run_count']}",
        f"- Pending approval items: {impact['pending_approvals'].get('pending', 0)}",
        "",
        "## Window Metrics",
        "",
    ]

    window_labels = {
        "baseline_pre_phase1": "Baseline before Phase 1",
        "phase1_templates": "Phase 1 window",
        "phase2_workflow_safety": "Phase 2 window",
        "phase3_tui_safety": "Phase 3 window",
        "phase4_pending_approval_queue": "Phase 4 window",
    }
    for key, label in window_labels.items():
        metrics = impact["windows"][key]["metrics"]
        lines.extend(
            [
                f"### {label}",
                "",
                f"- Runs: {metrics['run_count']}",
                f"- Average retries: {metrics['average_retry_count']:.2f}",
                f"- Average skipped gates: {metrics['average_skipped_gates']:.2f}",
                f"- Human follow-up rate: {metrics['human_followup_rate']:.2f}",
                f"- Average questions asked: {metrics['average_questions_asked']:.2f}",
                f"- Average clarification touches: {metrics['average_clarification_touches']:.2f}",
                "",
            ]
        )

    baseline = impact["current_vs_baseline"]["baseline_metrics"]
    current = impact["current_vs_baseline"]["current_metrics"]
    delta = impact["current_vs_baseline"]["delta"]
    real_baseline = impact["real_task_current_vs_baseline"]["baseline_metrics"]
    real_current = impact["real_task_current_vs_baseline"]["current_metrics"]
    lines.extend(
        [
            "## Current Vs Baseline",
            "",
            f"- Baseline average retries: {baseline['average_retry_count']:.2f}",
            f"- Current average retries: {current['average_retry_count']:.2f}",
            f"- Retry delta: {delta.get('average_retry_count', 0.0):+.2f}" if delta else "- Retry delta: n/a",
            f"- Baseline average skipped gates: {baseline['average_skipped_gates']:.2f}",
            f"- Current average skipped gates: {current['average_skipped_gates']:.2f}",
            f"- Skipped gate delta: {delta.get('average_skipped_gates', 0.0):+.2f}" if delta else "- Skipped gate delta: n/a",
            f"- Baseline clarification touches: {baseline['average_clarification_touches']:.2f}",
            f"- Current clarification touches: {current['average_clarification_touches']:.2f}",
            f"- Clarification delta: {delta.get('average_clarification_touches', 0.0):+.2f}" if delta else "- Clarification delta: n/a",
            "",
            "## Real Task Evidence",
            "",
            f"- Baseline real-task runs: {real_baseline['run_count']}",
            f"- Post-Phase 1 real-task runs: {real_current['run_count']}",
            f"- Baseline real-task average retries: {real_baseline['average_retry_count']:.2f}",
            f"- Post-Phase 1 real-task average retries: {real_current['average_retry_count']:.2f}",
            "",
            "## Pending Approval Queue",
            "",
            f"- Total items: {impact['pending_approvals'].get('total', 0)}",
            f"- Pending: {impact['pending_approvals'].get('pending', 0)}",
            f"- Approved: {impact['pending_approvals'].get('approved', 0)}",
            f"- Implemented: {impact['pending_approvals'].get('implemented', 0)}",
            f"- Published runs analyzed: {impact['pending_approvals'].get('published_runs_analyzed', 0)}",
            "",
            "## Findings",
            "",
        ]
    )
    if impact["findings"]:
        lines.extend([f"- {item}" for item in impact["findings"]])
    else:
        lines.append("- No clear findings yet; the current sample is too small to support directional claims.")

    lines.extend(
        [
            "",
            "## Caveats",
            "",
            "- This measurement is observational, not causal proof. The phase windows are small and some windows may contain synthetic validation runs alongside real task runs.",
            "- Phase 4 visibility can be measured immediately through the queue, but its effect on retries or clarification load depends on future published runs.",
        ]
    )
    return "\n".join(lines) + "\n"
