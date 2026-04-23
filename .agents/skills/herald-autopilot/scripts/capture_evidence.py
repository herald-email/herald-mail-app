#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import json
import uuid
from pathlib import Path


def now_utc() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def save_json(path: Path, payload) -> None:
    path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description="Append evidence to a Herald Autopilot run.")
    parser.add_argument("--run-dir", required=True, help="Path to the run directory")
    parser.add_argument("--kind", required=True, choices=["command", "screenshot", "note", "artifact", "link"], help="Evidence kind")
    parser.add_argument("--summary", required=True, help="Short evidence summary")
    parser.add_argument("--status", required=True, choices=["pass", "fail", "info", "skip"], help="Evidence status")
    parser.add_argument("--artifact", default="", help="Optional artifact path")
    parser.add_argument("--gate", default="", help="Optional verification gate name")
    parser.add_argument("--required", action="store_true", help="Mark the gate as required")
    parser.add_argument("--baseline", action="store_true", help="Update baseline from this evidence")
    args = parser.parse_args()

    run_dir = Path(args.run_dir).resolve()
    manifest_path = run_dir / "evidence" / "manifest.json"
    run_path = run_dir / "run.json"

    manifest = load_json(manifest_path)
    run = load_json(run_path)

    entry = {
        "id": str(uuid.uuid4()),
        "timestamp": now_utc(),
        "kind": args.kind,
        "summary": args.summary,
        "status": args.status,
        "gate": args.gate or None,
        "artifact": args.artifact or None,
    }
    manifest.append(entry)
    save_json(manifest_path, manifest)

    if args.baseline:
        run["baseline"] = {
            "status": "pass" if args.status == "pass" else "fail",
            "summary": args.summary,
            "artifact": args.artifact or "",
        }

    if args.gate:
        if args.gate not in run["verification"]["required_gates"] and args.required:
            run["verification"]["required_gates"].append(args.gate)
        result = {
            "gate": args.gate,
            "status": args.status,
            "summary": args.summary,
            "artifact": args.artifact or "",
            "required": args.required,
            "recorded_at": now_utc(),
        }
        run["verification"]["results"].append(result)

    run["updated_at"] = now_utc()
    save_json(run_path, run)
    print(entry["id"])
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
