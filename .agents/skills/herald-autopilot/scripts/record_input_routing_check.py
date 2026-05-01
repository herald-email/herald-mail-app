#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import uuid
from pathlib import Path

from artifact_io import load_json, locked_paths, save_json
from input_routing import (
    INPUT_ROUTING_GATE,
    check_issues,
    ensure_input_routing,
    input_routing_status,
    require_input_routing,
    summarize_input_routing_gate,
)


def now_utc() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def make_entry(kind: str, summary: str, artifact: str, gate: str) -> dict:
    return {
        "id": str(uuid.uuid4()),
        "timestamp": now_utc(),
        "kind": kind,
        "summary": summary,
        "status": "pass",
        "gate": gate,
        "artifact": artifact,
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


def replace_check(checks: list[dict], payload: dict) -> None:
    surface = payload.get("surface")
    sequence = payload.get("input_sequence")
    for index, item in enumerate(checks):
        if item.get("surface") == surface and item.get("input_sequence") == sequence:
            checks[index] = payload
            return
    checks.append(payload)


def main() -> int:
    parser = argparse.ArgumentParser(description="Record input-routing safety evidence for a Herald Autopilot run.")
    parser.add_argument("--run-dir", required=True, help="Path to the run directory")
    parser.add_argument("--surface", required=True, choices=["compose", "prompt", "editor"], help="Text-entry surface being exercised")
    parser.add_argument("--input-sequence", required=True, help="Literal input or shortcut sequence being tested")
    parser.add_argument("--expected-behavior", required=True, help="Expected user-visible behavior on this surface")
    parser.add_argument("--observed-behavior", required=True, help="Observed user-visible behavior on this surface")
    parser.add_argument("--artifact", required=True, help="Path to transcript, capture, or test log proving the check")
    parser.add_argument("--text-preserved", action="store_true", help="Confirm the typed text stayed in the active text field")
    parser.add_argument("--repro-step", action="append", default=[], help="One repro-path step. Provide in chronological order.")
    parser.add_argument("--note", default="", help="Optional extra note for this input-routing check")
    args = parser.parse_args()

    run_dir = Path(args.run_dir).resolve()
    run_path = run_dir / "run.json"
    manifest_path = run_dir / "evidence" / "manifest.json"

    with locked_paths(run_path, manifest_path):
        run = load_json(run_path)
        manifest = load_json(manifest_path)
        gate = ensure_input_routing(run)
        if not gate.get("required"):
            gate = require_input_routing(run)

        check = {
            "id": str(uuid.uuid4()),
            "recorded_at": now_utc(),
            "surface": args.surface,
            "input_sequence": args.input_sequence,
            "expected_behavior": args.expected_behavior,
            "observed_behavior": args.observed_behavior,
            "artifact": args.artifact,
            "text_preserved": args.text_preserved,
            "repro_steps": args.repro_step,
            "note": args.note,
        }
        check["issues"] = check_issues(check)
        check["complete"] = len(check["issues"]) == 0
        replace_check(gate["checks"], check)
        gate["status"] = input_routing_status(gate)

        gate_status = "pass" if gate["status"] == "passed" else "info"
        run["verification"]["results"].append(
            {
                "gate": INPUT_ROUTING_GATE,
                "status": gate_status,
                "summary": summarize_input_routing_gate(gate),
                "artifact": args.artifact,
                "required": True,
                "recorded_at": now_utc(),
            }
        )

        repro_summary = " -> ".join(args.repro_step) if args.repro_step else "No repro path recorded."
        upsert_manifest_entry(
            manifest,
            make_entry("artifact", f"Input routing evidence: {args.surface} ({args.input_sequence})", args.artifact, INPUT_ROUTING_GATE),
        )
        upsert_manifest_entry(
            manifest,
            make_entry("note", f"Input routing repro: {args.surface} ({args.input_sequence}) — {repro_summary}", "", INPUT_ROUTING_GATE),
        )
        if args.note:
            upsert_manifest_entry(
                manifest,
                make_entry("note", f"Input routing note: {args.surface} ({args.input_sequence}) — {args.note}", "", INPUT_ROUTING_GATE),
            )

        run["updated_at"] = now_utc()
        save_json(manifest_path, manifest)
        save_json(run_path, run)

    print(str(run_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
