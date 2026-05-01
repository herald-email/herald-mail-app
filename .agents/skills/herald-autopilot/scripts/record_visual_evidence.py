#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import uuid
from pathlib import Path

from artifact_io import load_json, locked_paths, save_json
from visual_evidence import (
    VISUAL_GATE,
    ensure_visual_evidence,
    pair_issues,
    require_visual_evidence,
    summarize_visual_gate,
    visual_status,
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


def replace_pair(pairs: list[dict], payload: dict) -> None:
    state_label = payload.get("state_label")
    size = payload.get("size")
    for index, item in enumerate(pairs):
        if item.get("state_label") == state_label and item.get("size") == size:
            pairs[index] = payload
            return
    pairs.append(payload)


def main() -> int:
    parser = argparse.ArgumentParser(description="Record canonical before/after visual evidence for a Herald Autopilot run.")
    parser.add_argument("--run-dir", required=True, help="Path to the run directory")
    parser.add_argument("--state-label", required=True, help="Stable label for the rendered state, such as cleanup-preview")
    parser.add_argument("--size", required=True, help="Terminal size label, such as 220x50")
    parser.add_argument("--before-png", required=True, help="Path to the baseline PNG screenshot")
    parser.add_argument("--after-png", required=True, help="Path to the final PNG screenshot")
    parser.add_argument("--before-text", required=True, help="Path to the baseline ANSI/text capture")
    parser.add_argument("--after-text", required=True, help="Path to the final ANSI/text capture")
    parser.add_argument("--repro-step", action="append", default=[], help="One repro-path step. Provide in chronological order.")
    parser.add_argument("--snapshot-sensitive", action="store_true", help="Mark this capture as sensitive to snapshot whitespace fidelity.")
    parser.add_argument("--snapshot-reviewed", action="store_true", help="Confirm snapshot-sensitive whitespace or golden handling was reviewed.")
    parser.add_argument("--note", default="", help="Optional extra note for this visual pair")
    args = parser.parse_args()

    run_dir = Path(args.run_dir).resolve()
    run_path = run_dir / "run.json"
    manifest_path = run_dir / "evidence" / "manifest.json"

    with locked_paths(run_path, manifest_path):
        run = load_json(run_path)
        manifest = load_json(manifest_path)
        visual = ensure_visual_evidence(run)
        if not visual.get("required"):
            visual = require_visual_evidence(run)

        pair = {
            "id": str(uuid.uuid4()),
            "recorded_at": now_utc(),
            "state_label": args.state_label,
            "size": args.size,
            "before_png": args.before_png,
            "after_png": args.after_png,
            "before_text": args.before_text,
            "after_text": args.after_text,
            "repro_steps": args.repro_step,
            "snapshot_sensitive": args.snapshot_sensitive,
            "snapshot_reviewed": args.snapshot_reviewed,
            "note": args.note,
        }
        pair["issues"] = pair_issues(pair)
        pair["complete"] = len(pair["issues"]) == 0
        replace_pair(visual["pairs"], pair)
        visual["status"] = visual_status(visual)

        gate_status = "pass" if visual["status"] == "passed" else "info"
        run["verification"]["results"].append(
            {
                "gate": VISUAL_GATE,
                "status": gate_status,
                "summary": summarize_visual_gate(visual),
                "artifact": args.after_png,
                "required": True,
                "recorded_at": now_utc(),
            }
        )

        repro_summary = " -> ".join(args.repro_step) if args.repro_step else "No repro path recorded."
        upsert_manifest_entry(
            manifest,
            make_entry("screenshot", f"Before: {args.state_label} at {args.size}", args.before_png, VISUAL_GATE),
        )
        upsert_manifest_entry(
            manifest,
            make_entry("screenshot", f"After: {args.state_label} at {args.size}", args.after_png, VISUAL_GATE),
        )
        upsert_manifest_entry(
            manifest,
            make_entry("artifact", f"Before ANSI: {args.state_label} at {args.size}", args.before_text, VISUAL_GATE),
        )
        upsert_manifest_entry(
            manifest,
            make_entry("artifact", f"After ANSI: {args.state_label} at {args.size}", args.after_text, VISUAL_GATE),
        )
        upsert_manifest_entry(
            manifest,
            make_entry("note", f"Repro path: {args.state_label} at {args.size} — {repro_summary}", "", VISUAL_GATE),
        )
        if args.note:
            upsert_manifest_entry(
                manifest,
                make_entry("note", f"Visual note: {args.state_label} at {args.size} — {args.note}", "", VISUAL_GATE),
            )

        run["updated_at"] = now_utc()
        save_json(manifest_path, manifest)
        save_json(run_path, run)

    print(str(run_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
