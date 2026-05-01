#!/usr/bin/env python3
from __future__ import annotations

from typing import Iterable


VISUAL_GATE = "visual-evidence"
CANONICAL_SIZES = ("220x50", "80x24", "50x15")


def default_visual_evidence(required: bool = False) -> dict:
    return {
        "required": required,
        "status": "pending" if required else "not-needed",
        "required_sizes": list(CANONICAL_SIZES) if required else [],
        "pairs": [],
    }


def ensure_visual_evidence(run: dict) -> dict:
    visual = run.setdefault("visual_evidence", default_visual_evidence())
    visual.setdefault("required", False)
    visual.setdefault("status", "pending" if visual["required"] else "not-needed")
    visual.setdefault("required_sizes", list(CANONICAL_SIZES) if visual["required"] else [])
    visual.setdefault("pairs", [])
    if visual["required"] and not visual["required_sizes"]:
        visual["required_sizes"] = list(CANONICAL_SIZES)
    if not visual["required"]:
        visual["status"] = "not-needed"
        visual["required_sizes"] = visual.get("required_sizes", [])
    return visual


def require_visual_evidence(run: dict) -> dict:
    visual = ensure_visual_evidence(run)
    visual["required"] = True
    if not visual.get("required_sizes"):
        visual["required_sizes"] = list(CANONICAL_SIZES)
    required_gates = run.setdefault("verification", {}).setdefault("required_gates", [])
    if VISUAL_GATE not in required_gates:
        required_gates.append(VISUAL_GATE)
    if visual.get("status") == "not-needed":
        visual["status"] = "pending"
    return visual


def pair_issues(pair: dict) -> list[str]:
    issues: list[str] = []
    required_fields = (
        ("before_png", "before PNG"),
        ("after_png", "after PNG"),
        ("before_text", "before ANSI/text capture"),
        ("after_text", "after ANSI/text capture"),
    )
    for key, label in required_fields:
        if not pair.get(key):
            issues.append(label)
    if not pair.get("repro_steps"):
        issues.append("repro steps")
    if pair.get("snapshot_sensitive") and not pair.get("snapshot_reviewed"):
        issues.append("snapshot review")
    return issues


def pair_complete(pair: dict) -> bool:
    return len(pair_issues(pair)) == 0


def covered_sizes(visual: dict) -> set[str]:
    sizes: set[str] = set()
    for pair in visual.get("pairs", []):
        if pair_complete(pair):
            size = pair.get("size", "").strip()
            if size:
                sizes.add(size)
    return sizes


def missing_sizes(visual: dict) -> list[str]:
    covered = covered_sizes(visual)
    return [size for size in visual.get("required_sizes", []) if size not in covered]


def visual_status(visual: dict) -> str:
    if not visual.get("required"):
        return "not-needed"
    return "passed" if not missing_sizes(visual) else "pending"


def summarize_visual_gate(visual: dict) -> str:
    if not visual.get("required"):
        return "Visual evidence is not required for this run."
    covered = covered_sizes(visual)
    total = len(visual.get("required_sizes", []))
    if total == 0:
        return "Visual evidence is required, but no canonical sizes were configured."
    if len(covered) == total:
        return f"Captured canonical before/after visual evidence for {len(covered)}/{total} required sizes."
    missing = ", ".join(missing_sizes(visual)) or "unknown sizes"
    return f"Visual evidence is still missing canonical coverage for: {missing}."


def visual_feedback_messages(visual: dict) -> list[str]:
    if not visual.get("required"):
        return []
    missing = missing_sizes(visual)
    messages: list[str] = []
    if not visual.get("pairs"):
        messages.append(
            "Required visual evidence was not recorded. Capture before/after PNG plus ANSI/text evidence at 220x50, 80x24, and 50x15."
        )
        return messages
    if missing:
        messages.append(f"Required visual evidence is still missing for terminal sizes: {', '.join(missing)}.")
    for pair in visual.get("pairs", []):
        issues = pair_issues(pair)
        if issues:
            state = pair.get("state_label", "state")
            size = pair.get("size", "unknown size")
            messages.append(f"Visual evidence for `{state}` at `{size}` is incomplete: {', '.join(issues)}.")
    return unique_strings(messages)


def unique_strings(items: Iterable[str]) -> list[str]:
    seen: set[str] = set()
    ordered: list[str] = []
    for item in items:
        text = item.strip()
        if not text or text in seen:
            continue
        seen.add(text)
        ordered.append(text)
    return ordered
