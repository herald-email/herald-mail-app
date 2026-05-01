#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

from artifact_io import save_json, save_text
from pending_approvals import build_pending_approvals, queue_doc_path, queue_path, render_pending_approvals_markdown


def main() -> int:
    parser = argparse.ArgumentParser(description="Sync the GEPA pending-approval queue from published run reflections.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    queue = build_pending_approvals(repo_root)
    json_path = queue_path(repo_root)
    markdown_path = queue_doc_path(repo_root)
    save_json(json_path, queue)
    save_text(markdown_path, render_pending_approvals_markdown(queue))
    print(str(json_path))
    print(str(markdown_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
