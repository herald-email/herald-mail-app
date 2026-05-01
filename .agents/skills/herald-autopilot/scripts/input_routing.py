#!/usr/bin/env python3
from __future__ import annotations

from typing import Iterable


INPUT_ROUTING_GATE = "input-routing-safety"
CANONICAL_SURFACES = ("compose", "prompt", "editor")
INPUT_ROUTING_KEYWORDS = (
    "shortcut",
    "shortcuts",
    "hotkey",
    "hotkeys",
    "alias",
    "aliases",
    "keybinding",
    "keybindings",
    "routing",
    "keyboard",
    "ime",
    "pre-edit",
    "compose-comma",
    "comma-alias",
    "non-latin",
)


def default_input_routing(required: bool = False) -> dict:
    return {
        "required": required,
        "status": "pending" if required else "not-needed",
        "required_surfaces": list(CANONICAL_SURFACES) if required else [],
        "checks": [],
    }


def ensure_input_routing(run: dict) -> dict:
    gate = run.setdefault("input_routing", default_input_routing())
    gate.setdefault("required", False)
    gate.setdefault("status", "pending" if gate["required"] else "not-needed")
    gate.setdefault("required_surfaces", list(CANONICAL_SURFACES) if gate["required"] else [])
    gate.setdefault("checks", [])
    if gate["required"] and not gate["required_surfaces"]:
        gate["required_surfaces"] = list(CANONICAL_SURFACES)
    if not gate["required"]:
        gate["status"] = "not-needed"
    return gate


def require_input_routing(run: dict, surfaces: Iterable[str] | None = None) -> dict:
    gate = ensure_input_routing(run)
    gate["required"] = True
    if surfaces is not None:
        gate["required_surfaces"] = [surface for surface in surfaces if surface]
    elif not gate.get("required_surfaces"):
        gate["required_surfaces"] = list(CANONICAL_SURFACES)
    required_gates = run.setdefault("verification", {}).setdefault("required_gates", [])
    if INPUT_ROUTING_GATE not in required_gates:
        required_gates.append(INPUT_ROUTING_GATE)
    if gate.get("status") == "not-needed":
        gate["status"] = "pending"
    return gate


def infer_input_routing_required(task_request: str, task_slug: str, surfaces: Iterable[str]) -> bool:
    surface_set = {surface.strip().lower() for surface in surfaces if surface}
    if "tui" not in surface_set:
        return False
    haystack = f"{task_request} {task_slug}".lower()
    return any(keyword in haystack for keyword in INPUT_ROUTING_KEYWORDS)


def check_issues(check: dict) -> list[str]:
    issues: list[str] = []
    if not check.get("artifact"):
        issues.append("artifact")
    if not check.get("repro_steps"):
        issues.append("repro steps")
    if not check.get("expected_behavior"):
        issues.append("expected behavior")
    if not check.get("observed_behavior"):
        issues.append("observed behavior")
    if not check.get("text_preserved", False):
        issues.append("text-preservation proof")
    return issues


def check_complete(check: dict) -> bool:
    return len(check_issues(check)) == 0


def covered_surfaces(gate: dict) -> set[str]:
    covered: set[str] = set()
    for check in gate.get("checks", []):
        if check_complete(check):
            surface = check.get("surface", "").strip()
            if surface:
                covered.add(surface)
    return covered


def missing_surfaces(gate: dict) -> list[str]:
    covered = covered_surfaces(gate)
    return [surface for surface in gate.get("required_surfaces", []) if surface not in covered]


def input_routing_status(gate: dict) -> str:
    if not gate.get("required"):
        return "not-needed"
    return "passed" if not missing_surfaces(gate) else "pending"


def summarize_input_routing_gate(gate: dict) -> str:
    if not gate.get("required"):
        return "Input routing safety is not required for this run."
    covered = covered_surfaces(gate)
    total = len(gate.get("required_surfaces", []))
    if total == 0:
        return "Input routing safety is required, but no canonical surfaces were configured."
    if len(covered) == total:
        return f"Recorded safe input-routing evidence for {len(covered)}/{total} required text-entry surfaces."
    missing = ", ".join(missing_surfaces(gate)) or "unknown surfaces"
    return f"Input routing safety is still missing coverage for: {missing}."


def input_routing_feedback_messages(gate: dict) -> list[str]:
    if not gate.get("required"):
        return []
    missing = missing_surfaces(gate)
    messages: list[str] = []
    if not gate.get("checks"):
        messages.append(
            "Required input routing safety was not recorded. Prove shortcut or alias changes preserve text entry in compose, prompt, and editor surfaces."
        )
        return messages
    if missing:
        messages.append(f"Required input routing safety is still missing for surfaces: {', '.join(missing)}.")
    for check in gate.get("checks", []):
        issues = check_issues(check)
        if issues:
            surface = check.get("surface", "unknown surface")
            messages.append(f"Input routing evidence for `{surface}` is incomplete: {', '.join(issues)}.")
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
