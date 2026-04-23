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
    surfaces = parse_surfaces(args.surfaces)

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
