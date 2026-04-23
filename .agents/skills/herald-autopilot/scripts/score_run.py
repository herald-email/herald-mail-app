#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import json
from pathlib import Path


def now_utc() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def save_json(path: Path, payload) -> None:
    path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Score a Herald Autopilot run.")
    parser.add_argument("--run-dir", required=True, help="Path to the run directory")
    args = parser.parse_args()

    run_dir = Path(args.run_dir).resolve()
    run = load_json(run_dir / "run.json")
    required = set(run["verification"].get("required_gates", []))
    results = run["verification"].get("results", [])
    failed_required = [item for item in results if item["gate"] in required and item["status"] == "fail"]
    passed_required = [item for item in results if item["gate"] in required and item["status"] == "pass"]
    retry_count = int(run["metrics"].get("retry_count", 0))
    human_followup = bool(run["metrics"].get("human_followup_needed", False))
    baseline_pass = run["baseline"].get("status") == "pass"
    files_changed = int(run["metrics"].get("files_changed", 0))
    product_truth = run.get("product_truth", {})
    product_truth_required = bool(product_truth.get("required", False))
    product_truth_grounded = (not product_truth_required) or product_truth.get("status") in {"consulted", "updated-first"}

    overall = 100
    if not baseline_pass:
        overall -= 20
    overall -= min(len(failed_required) * 25, 50)
    overall -= min(retry_count * 8, 24)
    overall -= 10 if human_followup else 0
    overall -= 5 if files_changed > 25 else 0
    overall -= 10 if product_truth_required and not product_truth_grounded else 0
    overall = max(overall, 0)

    feedback = list(run.get("latest_feedback", []))
    feedback.extend(item["summary"] for item in failed_required)

    status = "pass"
    if failed_required:
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
            "verification_completeness": len(passed_required) / len(required) if required else 1.0,
            "retry_efficiency": max(0, 1 - (retry_count / max(run["policy"].get("retry_limit", 1), 1))),
            "handoff_readiness": 0 if human_followup else 1,
            "product_truth_grounding": 1 if product_truth_grounded else 0,
        },
        "counts": {
            "required_gates": len(required),
            "required_passed": len(passed_required),
            "required_failed": len(failed_required),
            "retry_count": retry_count,
            "files_changed": files_changed,
            "product_truth_required": 1 if product_truth_required else 0,
            "product_truth_grounded": 1 if product_truth_required and product_truth_grounded else 0,
        },
        "feedback": feedback,
        "pareto_axes": {
            "verification": len(passed_required),
            "retries": retry_count,
            "followup_needed": 1 if human_followup else 0,
            "grounding_gap": 0 if product_truth_grounded else 1,
        },
    }

    save_json(run_dir / "score.json", score)
    print(json.dumps(score, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
