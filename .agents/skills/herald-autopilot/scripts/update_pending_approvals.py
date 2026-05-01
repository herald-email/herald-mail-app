#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

from artifact_io import save_json, save_text
from pending_approvals import (
    apply_pending_decision,
    build_pending_approvals,
    queue_doc_path,
    queue_path,
    render_pending_approvals_markdown,
)


def main() -> int:
    parser = argparse.ArgumentParser(description="Update decisions in the GEPA pending-approval queue.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    parser.add_argument("--status", required=True, choices=["pending", "approved", "rejected", "implemented"], help="New queue status")
    parser.add_argument("--key", action="append", default=[], help="Queue key to update (repeatable)")
    parser.add_argument("--all-pending", action="store_true", help="Apply the decision to every item still marked pending")
    parser.add_argument("--note", default="", help="Optional decision note")
    args = parser.parse_args()

    if not args.key and not args.all_pending:
        raise SystemExit("Provide at least one --key or use --all-pending.")

    repo_root = Path(args.repo_root).resolve()
    queue = build_pending_approvals(repo_root)
    keys = list(args.key)
    if args.all_pending:
        keys.extend(item["key"] for item in queue.get("items", []) if item.get("status") == "pending")
    queue = apply_pending_decision(queue, sorted(set(keys)), args.status, note=args.note)

    json_path = queue_path(repo_root)
    markdown_path = queue_doc_path(repo_root)
    save_json(json_path, queue)
    save_text(markdown_path, render_pending_approvals_markdown(queue))
    print(str(json_path))
    print(str(markdown_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
