#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import json
from pathlib import Path

from artifact_io import load_json, save_json
from visual_evidence import ensure_visual_evidence, unique_strings, visual_feedback_messages


def now_utc() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()

def latest_results_by_gate(items: list[dict], key_name: str) -> dict[str, dict]:
    latest: dict[str, dict] = {}
    for item in items:
        key = item.get(key_name)
        if key:
            latest[key] = item
    return latest


def main() -> int:
    parser = argparse.ArgumentParser(description="Score a Herald Autopilot run.")
    parser.add_argument("--run-dir", required=True, help="Path to the run directory")
    args = parser.parse_args()

    run_dir = Path(args.run_dir).resolve()
    run = load_json(run_dir / "run.json")
    required = set(run["verification"].get("required_gates", []))
    results = run["verification"].get("results", [])
    latest_required = latest_results_by_gate(results, "gate")
    failed_required = [latest_required[gate] for gate in required if latest_required.get(gate, {}).get("status") == "fail"]
    passed_required = [latest_required[gate] for gate in required if latest_required.get(gate, {}).get("status") == "pass"]
    missing_required = [gate for gate in required if gate not in latest_required]
    preflight = run.get("preflight", {})
    required_preflight = set(preflight.get("required_checks", []))
    latest_preflight = latest_results_by_gate(preflight.get("results", []), "check")
    failed_required_preflight = [
        latest_preflight[check]
        for check in required_preflight
        if latest_preflight.get(check, {}).get("status") == "fail"
    ]
    missing_required_preflight = [check for check in required_preflight if check not in latest_preflight]
    preflight_ready = not required_preflight or (not failed_required_preflight and not missing_required_preflight)
    retry_count = int(run["metrics"].get("retry_count", 0))
    human_followup = bool(run["metrics"].get("human_followup_needed", False))
    baseline_pass = run["baseline"].get("status") == "pass"
    files_changed = int(run["metrics"].get("files_changed", 0))
    product_truth = run.get("product_truth", {})
    product_truth_required = bool(product_truth.get("required", False))
    product_truth_grounded = (not product_truth_required) or product_truth.get("status") in {"consulted", "updated-first"}
    visual = ensure_visual_evidence(run)
    visual_required = bool(visual.get("required", False))
    visual_ready = (not visual_required) or visual.get("status") == "passed"

    overall = 100
    if not baseline_pass:
        overall -= 20
    overall -= min(len(failed_required) * 25, 50)
    overall -= min(len(missing_required) * 15, 30)
    overall -= min(len(failed_required_preflight) * 15, 30)
    overall -= min(len(missing_required_preflight) * 8, 16)
    overall -= min(retry_count * 8, 24)
    overall -= 10 if human_followup else 0
    overall -= 5 if files_changed > 25 else 0
    overall -= 10 if product_truth_required and not product_truth_grounded else 0
    overall -= 12 if visual_required and not visual_ready else 0
    overall = max(overall, 0)

    feedback = list(run.get("latest_feedback", []))
    feedback.extend(item["summary"] for item in failed_required)
    feedback.extend(f"Required gate `{gate}` did not run before handoff." for gate in missing_required)
    feedback.extend(item["summary"] for item in failed_required_preflight)
    feedback.extend(f"Required preflight `{check}` did not run before implementation." for check in missing_required_preflight)
    feedback.extend(visual_feedback_messages(visual))
    feedback = unique_strings(feedback)

    status = "pass"
    if failed_required or missing_required or failed_required_preflight or missing_required_preflight or (visual_required and not visual_ready):
        status = "fail"
    elif human_followup:
        status = "needs_followup"

    score = {
        "scored_at": now_utc(),
        "run_id": run["run_id"],
        "status": status,
        "overall_score": overall,
        "axes": {
            "baseline_cleanliness": 1 if baseline_pass else 0,
            "preflight_readiness": 1 if preflight_ready else 0,
            "verification_completeness": len(passed_required) / len(required) if required else 1.0,
            "retry_efficiency": max(0, 1 - (retry_count / max(run["policy"].get("retry_limit", 1), 1))),
            "handoff_readiness": 0 if human_followup else 1,
            "product_truth_grounding": 1 if product_truth_grounded else 0,
            "visual_evidence_readiness": 1 if visual_ready else 0,
        },
        "counts": {
            "required_gates": len(required),
            "required_passed": len(passed_required),
            "required_failed": len(failed_required),
            "required_missing": len(missing_required),
            "preflight_required": len(required_preflight),
            "preflight_failed": len(failed_required_preflight),
            "preflight_missing": len(missing_required_preflight),
            "retry_count": retry_count,
            "files_changed": files_changed,
            "product_truth_required": 1 if product_truth_required else 0,
            "product_truth_grounded": 1 if product_truth_required and product_truth_grounded else 0,
            "visual_required": 1 if visual_required else 0,
            "visual_complete": 1 if visual_required and visual_ready else 0,
        },
        "feedback": feedback,
        "pareto_axes": {
            "verification": len(passed_required),
            "retries": retry_count,
            "followup_needed": 1 if human_followup else 0,
            "preflight_gap": 0 if preflight_ready else 1,
            "grounding_gap": 0 if product_truth_grounded else 1,
            "visual_gap": 0 if visual_ready else 1,
        },
    }

    save_json(run_dir / "score.json", score)
    print(json.dumps(score, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
