#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
from pathlib import Path

from artifact_io import load_json, locked_paths, save_json


def now_utc() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def main() -> int:
    parser = argparse.ArgumentParser(description="Record a reflection for a Herald Autopilot run.")
    parser.add_argument("--run-dir", required=True, help="Path to the run directory")
    parser.add_argument("--attempt", required=True, type=int, help="Attempt number")
    parser.add_argument("--failing-evidence", required=True, help="Name of the failing gate or evidence")
    parser.add_argument("--hypothesis", required=True, help="Root cause hypothesis")
    parser.add_argument("--next-step", required=True, help="Bounded next action")
    parser.add_argument("--decision", required=True, choices=["continue", "stop"], help="Whether to continue the loop")
    parser.add_argument("--feedback", action="append", default=[], help="Natural-language feedback for the next retry")
    args = parser.parse_args()

    run_dir = Path(args.run_dir).resolve()
    run_path = run_dir / "run.json"
    reflection_path = run_dir / "reflections" / f"{args.attempt:03d}.json"
    with locked_paths(run_path):
        run = load_json(run_path)
        reflection = {
            "attempt": args.attempt,
            "timestamp": now_utc(),
            "failing_evidence": args.failing_evidence,
            "hypothesis": args.hypothesis,
            "next_step": args.next_step,
            "decision": args.decision,
            "feedback": args.feedback,
        }
        save_json(reflection_path, reflection)

        run["metrics"]["retry_count"] = max(run["metrics"].get("retry_count", 0), args.attempt)
        run["latest_feedback"] = args.feedback
        run["updated_at"] = now_utc()
        save_json(run_path, run)
    print(str(reflection_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
