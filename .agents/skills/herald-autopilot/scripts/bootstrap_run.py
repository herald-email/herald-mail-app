#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import json
import re
from pathlib import Path


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
        "verification": {
            "required_gates": [],
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

    (run_dir / "run.json").write_text(json.dumps(run, indent=2) + "\n", encoding="utf-8")
    (run_dir / "intake.md").write_text(
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
        encoding="utf-8",
    )
    (run_dir / "plan.md").write_text(
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
        encoding="utf-8",
    )
    (evidence_dir / "manifest.json").write_text("[]\n", encoding="utf-8")

    print(str(run_dir))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
