from __future__ import annotations

import hashlib
import re
from pathlib import Path
from typing import Any

from optimizer_common import list_runs, now_utc


QUEUE_SCHEMA_VERSION = "herald-autopilot.pending-approvals.v1"
QUEUE_STATUSES = ("pending", "approved", "rejected", "implemented")


def queue_path(repo_root: Path) -> Path:
    return repo_root / ".superpowers" / "autopilot" / "state" / "pending-approvals.json"


def queue_doc_path(repo_root: Path) -> Path:
    return repo_root / "docs" / "superpowers" / "gepa-pending-approvals.md"


def slugify(value: str) -> str:
    slug = re.sub(r"[^a-z0-9]+", "-", value.lower()).strip("-")
    return slug or "suggestion"


def make_suggestion_key(item: dict[str, Any]) -> str:
    title = str(item.get("title", "")).strip()
    prompt = str(item.get("approval_prompt", "")).strip()
    digest = hashlib.sha1(f"{title}\n{prompt}".encode("utf-8")).hexdigest()[:10]
    return f"{slugify(title or prompt)}-{digest}"


def empty_queue() -> dict[str, Any]:
    return {
        "schema_version": QUEUE_SCHEMA_VERSION,
        "updated_at": now_utc(),
        "summary": {
            "total": 0,
            "pending": 0,
            "approved": 0,
            "rejected": 0,
            "implemented": 0,
            "published_runs_analyzed": 0,
        },
        "items": [],
    }


def summarize_queue(items: list[dict[str, Any]], *, published_runs_analyzed: int) -> dict[str, int]:
    counts = {status: 0 for status in QUEUE_STATUSES}
    for item in items:
        status = item.get("status", "pending")
        if status in counts:
            counts[status] += 1
    return {
        "total": len(items),
        "pending": counts["pending"],
        "approved": counts["approved"],
        "rejected": counts["rejected"],
        "implemented": counts["implemented"],
        "published_runs_analyzed": published_runs_analyzed,
    }


def sort_queue_items(items: list[dict[str, Any]]) -> list[dict[str, Any]]:
    status_order = {name: index for index, name in enumerate(QUEUE_STATUSES)}

    def key(item: dict[str, Any]) -> tuple[int, int, str, str]:
        return (
            status_order.get(item.get("status", "pending"), 999),
            -int(item.get("occurrences", 0)),
            str(item.get("last_seen_at", "")),
            str(item.get("title", "")),
        )

    return sorted(items, key=key)


def build_pending_approvals(repo_root: Path) -> dict[str, Any]:
    from artifact_io import load_json

    repo_root = repo_root.resolve()
    existing = empty_queue()
    existing_path = queue_path(repo_root)
    if existing_path.exists():
        existing = load_json(existing_path)

    existing_by_key = {item["key"]: item for item in existing.get("items", []) if item.get("key")}
    items_by_key: dict[str, dict[str, Any]] = {}
    published_runs_analyzed = 0

    for record in list_runs(repo_root):
        reflection_path = record.run_dir / "self_reflection.json"
        if not reflection_path.exists():
            continue
        reflection = load_json(reflection_path)
        publication_actions = reflection.get("publication_actions", [])
        suggestions = reflection.get("suggested_changes", [])
        if not publication_actions or not suggestions:
            continue

        published_runs_analyzed += 1
        seen_in_run: set[str] = set()
        created_at = record.run.get("created_at", "")
        for suggestion in suggestions:
            queue_key = suggestion.get("queue_key") or make_suggestion_key(suggestion)
            if not queue_key or queue_key in seen_in_run:
                continue
            seen_in_run.add(queue_key)

            existing_item = existing_by_key.get(queue_key, {})
            item = items_by_key.setdefault(
                queue_key,
                {
                    "key": queue_key,
                    "title": suggestion.get("title", ""),
                    "why": suggestion.get("why", ""),
                    "approval_prompt": suggestion.get("approval_prompt", ""),
                    "status": existing_item.get("status", "pending"),
                    "decision": existing_item.get("decision", {}),
                    "first_seen_at": created_at,
                    "last_seen_at": created_at,
                    "occurrences": 0,
                    "publication_actions": [],
                    "source_runs": [],
                },
            )
            item["title"] = item.get("title") or suggestion.get("title", "")
            item["why"] = item.get("why") or suggestion.get("why", "")
            item["approval_prompt"] = item.get("approval_prompt") or suggestion.get("approval_prompt", "")
            item["occurrences"] += 1
            if created_at and (not item.get("first_seen_at") or created_at < item["first_seen_at"]):
                item["first_seen_at"] = created_at
            if created_at and (not item.get("last_seen_at") or created_at > item["last_seen_at"]):
                item["last_seen_at"] = created_at
            item["publication_actions"] = sorted(set(item.get("publication_actions", [])) | set(publication_actions))
            item["source_runs"].append(
                {
                    "run_id": record.run_id,
                    "created_at": created_at,
                    "publication_actions": publication_actions,
                    "self_reflection_path": str(reflection_path),
                }
            )

    for queue_key, existing_item in existing_by_key.items():
        if queue_key in items_by_key:
            continue
        items_by_key[queue_key] = existing_item

    items = sort_queue_items(list(items_by_key.values()))
    return {
        "schema_version": QUEUE_SCHEMA_VERSION,
        "updated_at": now_utc(),
        "summary": summarize_queue(items, published_runs_analyzed=published_runs_analyzed),
        "items": items,
    }


def apply_pending_decision(
    queue: dict[str, Any],
    keys: list[str],
    status: str,
    *,
    note: str = "",
    decided_at: str | None = None,
) -> dict[str, Any]:
    if status not in QUEUE_STATUSES:
        raise ValueError(f"unsupported queue status: {status}")

    selected = set(keys)
    changed = False
    for item in queue.get("items", []):
        if item.get("key") not in selected:
            continue
        changed = True
        item["status"] = status
        if status == "pending":
            item["decision"] = {}
        else:
            item["decision"] = {
                "status": status,
                "decided_at": decided_at or now_utc(),
                "note": note,
            }
    if not changed:
        raise ValueError("no pending-approval items matched the requested key(s)")
    queue["items"] = sort_queue_items(queue.get("items", []))
    queue["updated_at"] = now_utc()
    published_runs_analyzed = int(queue.get("summary", {}).get("published_runs_analyzed", 0))
    queue["summary"] = summarize_queue(queue.get("items", []), published_runs_analyzed=published_runs_analyzed)
    return queue


def render_pending_approvals_markdown(queue: dict[str, Any]) -> str:
    summary = queue.get("summary", {})
    items = queue.get("items", [])
    lines = [
        "# Herald GEPA Pending Approvals",
        "",
        "This document is the visible approval backlog for workflow suggestions recovered from published Herald autopilot runs. It turns scattered post-publish self-reflection into one place where approvals, rejections, and implemented ideas can be reviewed without digging through run folders.",
        "",
        "## Snapshot",
        "",
        f"- Updated at: {queue.get('updated_at', '')}",
        f"- Published runs analyzed: {summary.get('published_runs_analyzed', 0)}",
        f"- Total queue items: {summary.get('total', 0)}",
        f"- Pending: {summary.get('pending', 0)}",
        f"- Approved: {summary.get('approved', 0)}",
        f"- Rejected: {summary.get('rejected', 0)}",
        f"- Implemented: {summary.get('implemented', 0)}",
        "",
        "## How To Update",
        "",
        "- Sync from the latest published reflections: `python3 .agents/skills/herald-autopilot/scripts/sync_pending_approvals.py --repo-root .`",
        "- Approve one or more items: `python3 .agents/skills/herald-autopilot/scripts/update_pending_approvals.py --repo-root . --status approved --key <queue-key>`",
        "- Batch-approve everything still pending: `python3 .agents/skills/herald-autopilot/scripts/update_pending_approvals.py --repo-root . --status approved --all-pending`",
        "",
    ]

    sections = (
        ("Pending Items", "pending"),
        ("Approved Items", "approved"),
        ("Rejected Items", "rejected"),
        ("Implemented Items", "implemented"),
    )
    for heading, status in sections:
        lines.extend([f"## {heading}", "",])
        matching = [item for item in items if item.get("status") == status]
        if not matching:
            lines.append(f"- No {status} items.")
            lines.append("")
            continue
        for item in matching:
            lines.append(f"### {item.get('title', '')}")
            lines.append("")
            lines.append(f"- Queue key: `{item.get('key', '')}`")
            lines.append(f"- Status: {item.get('status', '')}")
            lines.append(f"- Seen in runs: {item.get('occurrences', 0)}")
            lines.append(f"- First seen: {item.get('first_seen_at', '') or 'unknown'}")
            lines.append(f"- Last seen: {item.get('last_seen_at', '') or 'unknown'}")
            if item.get("publication_actions"):
                lines.append(f"- Publish actions: {', '.join(item['publication_actions'])}")
            lines.append(f"- Why: {item.get('why', '')}")
            lines.append(f"- Approval prompt: {item.get('approval_prompt', '')}")
            if item.get("decision"):
                decision = item["decision"]
                lines.append(f"- Decision: {decision.get('status', '')} at {decision.get('decided_at', '')}")
                if decision.get("note"):
                    lines.append(f"- Note: {decision['note']}")
            if item.get("source_runs"):
                lines.append("- Source runs:")
                for source in item["source_runs"][:5]:
                    lines.append(
                        f"- `{source.get('run_id', '')}` at {source.get('created_at', '') or 'unknown'} via {', '.join(source.get('publication_actions', [])) or 'no publish action'}"
                    )
            lines.append("")
    return "\n".join(lines).rstrip() + "\n"
