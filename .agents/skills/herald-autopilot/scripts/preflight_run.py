#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import os
import shutil
from pathlib import Path

from artifact_io import load_json, locked_paths, save_json

KNOWN_CHECKS = ("docs-deps", "ssh-host-key", "media-batches")
MEDIA_KEYWORDS = ("demo", "gif", "media", "screenshot", "screenshots", "vhs")


def now_utc() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def infer_required_checks(run: dict) -> set[str]:
    task = run.get("task", {})
    request = " ".join([task.get("request", ""), task.get("slug", "")]).lower()
    surfaces = set(task.get("surfaces", []))
    checks: set[str] = set()
    if "docs" in surfaces:
        checks.add("docs-deps")
    if "ssh" in surfaces:
        checks.add("ssh-host-key")
    if any(keyword in request for keyword in MEDIA_KEYWORDS):
        checks.add("media-batches")
    return checks


def ensure_preflight(run: dict) -> dict:
    preflight = run.setdefault("preflight", {})
    preflight.setdefault("status", "pending")
    preflight.setdefault("required_checks", [])
    preflight.setdefault("results", [])
    preflight.setdefault("resources", {})
    return preflight


def docs_deps_check(repo_root: Path, run_dir: Path) -> tuple[dict, dict]:
    docs_dir = repo_root / "docs"
    package_json = docs_dir / "package.json"
    astro_bin = docs_dir / "node_modules" / ".bin" / "astro"
    resources: dict[str, str] = {}
    if not package_json.exists():
        return (
            {
                "check": "docs-deps",
                "status": "pass",
                "summary": "Docs surface has no local package.json, so no docs node dependency bootstrap is required.",
                "artifact": "",
                "required": True,
                "recorded_at": now_utc(),
            },
            resources,
        )
    if astro_bin.exists():
        resources["docs_astro_bin"] = str(astro_bin)
        return (
            {
                "check": "docs-deps",
                "status": "pass",
                "summary": "Docs dependencies are installed and Astro is available from docs/node_modules/.bin/astro.",
                "artifact": str(astro_bin),
                "required": True,
                "recorded_at": now_utc(),
            },
            resources,
        )
    return (
        {
            "check": "docs-deps",
            "status": "fail",
            "summary": "Docs build prerequisites are missing because docs/node_modules/.bin/astro is not available. Run npm install in docs before docs verification.",
            "artifact": str(package_json),
            "required": True,
            "recorded_at": now_utc(),
        },
        resources,
    )


def ssh_host_key_check(run_dir: Path) -> tuple[dict, dict]:
    host_key_dir = run_dir / "evidence" / "ssh-hostkeys"
    host_key_dir.mkdir(parents=True, exist_ok=True)
    os.chmod(host_key_dir, 0o700)
    host_key_path = host_key_dir / "host_ed25519"
    resources = {"ssh_host_key_path": str(host_key_path)}
    return (
        {
            "check": "ssh-host-key",
            "status": "pass",
            "summary": "Prepared an owned run-local SSH host-key path so smoke checks can avoid fragile /tmp host-key parents.",
            "artifact": str(host_key_path),
            "required": True,
            "recorded_at": now_utc(),
        },
        resources,
    )


def media_batches_check(repo_root: Path, run_dir: Path) -> tuple[dict, dict]:
    media_dir = run_dir / "evidence" / "media-batches"
    media_dir.mkdir(parents=True, exist_ok=True)
    state_path = media_dir / "state.json"
    if state_path.exists():
        state = load_json(state_path)
    else:
        state = {
            "schema_version": "herald-autopilot.media-batches.v1",
            "created_at": now_utc(),
            "completed_batches": [],
        }
    state["updated_at"] = now_utc()
    state["resume_supported"] = True
    state["vhs_available"] = shutil.which("vhs") is not None
    state["demos_dir"] = str(repo_root / "demos")
    state["screenshots_dir"] = str(repo_root / "docs" / "public" / "screenshots")
    save_json(state_path, state)
    resources = {"media_batch_state": str(state_path)}
    if state["vhs_available"]:
        return (
            {
                "check": "media-batches",
                "status": "pass",
                "summary": "Prepared a resumable media-batch state file and confirmed vhs is available for long-running demo or screenshot work.",
                "artifact": str(state_path),
                "required": True,
                "recorded_at": now_utc(),
            },
            resources,
        )
    return (
        {
            "check": "media-batches",
            "status": "fail",
            "summary": "Prepared a resumable media-batch state file, but vhs is unavailable. Install vhs before running long-form demo or screenshot batches.",
            "artifact": str(state_path),
            "required": True,
            "recorded_at": now_utc(),
        },
        resources,
    )


def run_check(name: str, repo_root: Path, run_dir: Path) -> tuple[dict, dict]:
    if name == "docs-deps":
        return docs_deps_check(repo_root, run_dir)
    if name == "ssh-host-key":
        return ssh_host_key_check(run_dir)
    if name == "media-batches":
        return media_batches_check(repo_root, run_dir)
    raise ValueError(f"unknown preflight check: {name}")


def summarize_status(required_checks: set[str], results_by_check: dict[str, dict]) -> str:
    if not required_checks:
        return "not-needed"
    for check in required_checks:
        if results_by_check.get(check, {}).get("status") == "fail":
            return "failed"
    if all(results_by_check.get(check, {}).get("status") == "pass" for check in required_checks):
        return "passed"
    return "pending"


def main() -> int:
    parser = argparse.ArgumentParser(description="Run Herald Autopilot environment preflight checks.")
    parser.add_argument("--run-dir", required=True, help="Path to the run directory")
    parser.add_argument("--require-check", action="append", choices=KNOWN_CHECKS, default=[], help="Force a specific preflight check")
    parser.add_argument("--skip-check", action="append", choices=KNOWN_CHECKS, default=[], help="Skip a specific inferred preflight check")
    args = parser.parse_args()

    run_dir = Path(args.run_dir).resolve()
    run_path = run_dir / "run.json"

    with locked_paths(run_path):
        run = load_json(run_path)
        repo_root = Path(run["paths"]["repo_root"])
        preflight = ensure_preflight(run)
        required_checks = infer_required_checks(run)
        required_checks.update(args.require_check)
        required_checks.difference_update(args.skip_check)

        existing_results = {item.get("check"): item for item in preflight.get("results", []) if item.get("check")}
        resources = dict(preflight.get("resources", {}))

        for check_name in sorted(required_checks):
            result, new_resources = run_check(check_name, repo_root, run_dir)
            existing_results[check_name] = result
            resources.update(new_resources)

        preflight["required_checks"] = sorted(required_checks)
        preflight["results"] = [existing_results[name] for name in sorted(existing_results)]
        preflight["resources"] = resources
        preflight["status"] = summarize_status(required_checks, existing_results)
        run["updated_at"] = now_utc()

        if preflight["status"] == "failed":
            run["status"] = "blocked"
        elif run.get("status") == "initialized":
            run["status"] = "preflight_checked"

        save_json(run_path, run)
        save_json(run_dir / "preflight.json", preflight)

    print(str(run_dir / "preflight.json"))
    return 1 if preflight["status"] == "failed" else 0


if __name__ == "__main__":
    raise SystemExit(main())
