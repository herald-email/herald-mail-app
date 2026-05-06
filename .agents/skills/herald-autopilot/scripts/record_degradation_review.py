#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import uuid
from pathlib import Path

from artifact_io import load_json, locked_paths, save_json
from degradation_review import (
    DEGRADATION_GATE,
    append_unique,
    degradation_status,
    ensure_degradation_review,
    require_degradation_review,
    review_issues,
    summarize_degradation_review,
)


def now_utc() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def make_entry(kind: str, summary: str, gate: str) -> dict:
    return {
        "id": str(uuid.uuid4()),
        "timestamp": now_utc(),
        "kind": kind,
        "summary": summary,
        "status": "pass",
        "gate": gate,
        "artifact": "",
    }


def upsert_manifest_entry(manifest: list[dict], entry: dict) -> None:
    manifest[:] = [
        item
        for item in manifest
        if not (
            item.get("kind") == entry["kind"]
            and item.get("summary") == entry["summary"]
            and (item.get("artifact") or "") == entry["artifact"]
        )
    ]
    manifest.append(entry)


def main() -> int:
    parser = argparse.ArgumentParser(description="Record the Herald Autopilot degradation review gate.")
    parser.add_argument("--run-dir", required=True, help="Path to the run directory")
    parser.add_argument("--answer", required=True, choices=["yes", "no"], help="Whether the user approved planned degradation")
    parser.add_argument("--user-response", required=True, help="The user's explicit answer or approval text")
    parser.add_argument("--allowed-degradation", action="append", default=[], help="Approved degradation, required when answer is yes")
    parser.add_argument("--preserved-behavior", action="append", default=[], help="Existing behavior this plan must preserve")
    parser.add_argument("--regression-check", action="append", default=[], help="Regression check that protects preserved behavior")
    parser.add_argument("--note", action="append", default=[], help="Optional degradation-review note")
    args = parser.parse_args()

    run_dir = Path(args.run_dir).resolve()
    run_path = run_dir / "run.json"
    manifest_path = run_dir / "evidence" / "manifest.json"

    with locked_paths(run_path, manifest_path):
        run = load_json(run_path)
        manifest = load_json(manifest_path)
        review = ensure_degradation_review(run)
        if not review.get("required", True):
            review = require_degradation_review(run)

        review["answer"] = args.answer
        review["user_response"] = args.user_response
        append_unique(review["allowed_degradations"], args.allowed_degradation)
        append_unique(review["preserved_behaviors"], args.preserved_behavior)
        append_unique(review["regression_checks"], args.regression_check)
        append_unique(review["notes"], args.note)
        review["issues"] = review_issues(review)
        review["complete"] = len(review["issues"]) == 0
        review["status"] = degradation_status(review)

        gate_status = "pass" if review["status"] == "passed" else "info"
        run["verification"]["results"].append(
            {
                "gate": DEGRADATION_GATE,
                "status": gate_status,
                "summary": summarize_degradation_review(review),
                "artifact": "",
                "required": True,
                "recorded_at": now_utc(),
            }
        )

        upsert_manifest_entry(
            manifest,
            make_entry("note", f"Degradation review: {summarize_degradation_review(review)}", DEGRADATION_GATE),
        )

        run["updated_at"] = now_utc()
        save_json(manifest_path, manifest)
        save_json(run_path, run)

    print(str(run_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
