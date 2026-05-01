#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import re
from pathlib import Path

from artifact_io import save_json, save_text
from input_routing import INPUT_ROUTING_GATE, default_input_routing, infer_input_routing_required
from visual_evidence import VISUAL_GATE, default_visual_evidence


def slugify(value: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", value.lower()).strip("-")
    return slug or "task"


def now_utc() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def parse_surfaces(raw: str) -> list[str]:
    if not raw.strip():
        return []
    return [part.strip() for part in raw.split(",") if part.strip()]


def infer_product_truth_status(args, required: bool) -> str:
    if args.product_truth_status:
        return args.product_truth_status
    if not required:
        return "not-needed"
    if args.doc_updated:
        return "updated-first"
    if args.truth_source or args.product_truth_summary:
        return "consulted"
    return "pending"


def main() -> int:
    parser = argparse.ArgumentParser(description="Create a Herald Autopilot run folder.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    parser.add_argument("--task", required=True, help="Original user request")
    parser.add_argument("--task-type", default="bug", choices=["bug", "feature", "workflow-improvement"], help="Task type")
    parser.add_argument("--surfaces", default="code", help="Comma-separated affected surfaces, e.g. code,tui")
    parser.add_argument("--plan-summary", default="", help="Concise plan summary")
    parser.add_argument("--status", default="initialized", help="Initial run status")
    parser.add_argument("--run-id", default="", help="Optional explicit run id")
    parser.add_argument("--slug", default="", help="Optional explicit slug")
    parser.add_argument("--retry-limit", type=int, default=2, help="Maximum retry count")
    parser.add_argument("--requires-product-truth", action="store_true", help="Mark product-truth grounding as required for this run")
    parser.add_argument("--product-truth-status", default="", choices=["pending", "consulted", "updated-first", "not-needed"], help="Initial product-truth grounding status")
    parser.add_argument("--product-truth-summary", default="", help="Short note describing how the task was grounded")
    parser.add_argument("--truth-source", action="append", default=[], help="Canonical product-truth source consulted for the run")
    parser.add_argument("--doc-updated", action="append", default=[], help="Product doc updated before implementation")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    slug = args.slug or slugify(args.task)
    timestamp = dt.datetime.now().strftime("%Y%m%d-%H%M%S")
    run_id = args.run_id or f"{timestamp}-{slug}"
    worktree = repo_root / ".worktrees" / f"{run_id}-{slug}"
    run_dir = repo_root / ".superpowers" / "autopilot" / "runs" / run_id
    evidence_dir = run_dir / "evidence"
    reflections_dir = run_dir / "reflections"
    date_prefix = dt.datetime.now().strftime("%Y-%m-%d")
    branch = f"codex/autopilot-{slug}-{timestamp}"
    report_path = repo_root / "reports" / f"TEST_REPORT_{date_prefix}_{slug}.md"
    evolution_doc = repo_root / "docs" / "superpowers" / "gepa-evolution.md"
    product_truth_snapshot = repo_root / ".superpowers" / "autopilot" / "state" / "product-truth.md"
    surfaces = parse_surfaces(args.surfaces)
    product_truth_required = args.requires_product_truth or args.task_type == "feature"
    product_truth_status = infer_product_truth_status(args, product_truth_required)
    visual_required = "tui" in surfaces
    input_routing_required = infer_input_routing_required(args.task, slug, surfaces)
    required_gates = []
    if visual_required:
        required_gates.append(VISUAL_GATE)
    if input_routing_required:
        required_gates.append(INPUT_ROUTING_GATE)

    evidence_dir.mkdir(parents=True, exist_ok=True)
    reflections_dir.mkdir(parents=True, exist_ok=True)

    run = {
        "schema_version": "herald-autopilot.v1",
        "run_id": run_id,
        "created_at": now_utc(),
        "updated_at": now_utc(),
        "status": args.status,
        "mode": "reflective-single-run",
        "task": {
            "request": args.task,
            "type": args.task_type,
            "slug": slug,
            "surfaces": surfaces,
        },
        "paths": {
            "repo_root": str(repo_root),
            "run_dir": str(run_dir),
            "worktree": str(worktree),
            "branch": branch,
            "report_path": str(report_path),
            "evolution_doc": str(evolution_doc),
            "product_truth_snapshot": str(product_truth_snapshot),
        },
        "policy": {
            "approval_mode": "interrupt-only-for-real-decisions",
            "verification_mode": "impact-based",
            "retry_limit": args.retry_limit,
        },
        "baseline": {
            "status": "unknown",
            "summary": "",
        },
        "preflight": {
            "status": "pending",
            "required_checks": [],
            "results": [],
            "resources": {},
        },
        "plan": {
            "summary": args.plan_summary,
            "questions_asked": [],
            "decisions": [],
        },
        "product_truth": {
            "required": product_truth_required,
            "status": product_truth_status,
            "summary": args.product_truth_summary,
            "sources": args.truth_source,
            "docs_updated": args.doc_updated,
        },
        "publication": {
            "actions": [],
            "summary": "",
        },
        "visual_evidence": default_visual_evidence(required=visual_required),
        "input_routing": default_input_routing(required=input_routing_required),
        "verification": {
            "required_gates": required_gates,
            "results": [],
        },
        "metrics": {
            "retry_count": 0,
            "files_changed": 0,
            "human_followup_needed": False,
        },
        "outcome": {
            "summary": "",
            "remaining_risks": [],
        },
        "latest_feedback": [],
    }

    save_json(run_dir / "run.json", run)
    save_text(
        run_dir / "intake.md",
        "\n".join(
            [
                f"# Intake — {run_id}",
                "",
                "## Request",
                args.task,
                "",
                f"- Type: {args.task_type}",
                f"- Surfaces: {', '.join(surfaces) if surfaces else 'none yet'}",
                f"- Product truth required: {'yes' if product_truth_required else 'no'}",
                f"- Product truth status: {product_truth_status}",
            ]
        )
        + "\n",
    )
    save_text(
        run_dir / "plan.md",
        "\n".join(
            [
                f"# Plan — {run_id}",
                "",
                args.plan_summary or "Plan summary not recorded yet.",
                "",
                "## Product Truth",
                args.product_truth_summary or "Product-truth grounding not recorded yet.",
                "",
                f"- Required: {'yes' if product_truth_required else 'no'}",
                f"- Status: {product_truth_status}",
                f"- Sources: {', '.join(args.truth_source) if args.truth_source else 'none recorded'}",
                f"- Docs updated first: {', '.join(args.doc_updated) if args.doc_updated else 'none recorded'}",
            ]
        )
        + "\n",
    )
    save_text(evidence_dir / "manifest.json", "[]\n")

    print(str(run_dir))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
