#!/usr/bin/env python3
from __future__ import annotations

from typing import Iterable


DEGRADATION_GATE = "degradation-review"
DEGRADATION_QUESTION = (
    "Does this plan intentionally degrade, remove, or weaken any existing behavior, "
    "compatibility, UI affordance, preview/media behavior, docs/demo output, or surface contract?"
)


def default_degradation_review(required: bool = True) -> dict:
    return {
        "required": required,
        "status": "pending" if required else "not-needed",
        "question": DEGRADATION_QUESTION,
        "answer": "unanswered",
        "user_response": "",
        "allowed_degradations": [],
        "preserved_behaviors": [],
        "regression_checks": [],
        "notes": [],
    }


def ensure_degradation_review(run: dict) -> dict:
    review = run.setdefault("degradation_review", default_degradation_review(required=False))
    review.setdefault("required", True)
    review.setdefault("status", "pending" if review["required"] else "not-needed")
    review.setdefault("question", DEGRADATION_QUESTION)
    review.setdefault("answer", "unanswered")
    review.setdefault("user_response", "")
    review.setdefault("allowed_degradations", [])
    review.setdefault("preserved_behaviors", [])
    review.setdefault("regression_checks", [])
    review.setdefault("notes", [])
    if not review["required"]:
        review["status"] = "not-needed"
    return review


def require_degradation_review(run: dict) -> dict:
    review = ensure_degradation_review(run)
    review["required"] = True
    if review.get("status") == "not-needed":
        review["status"] = "pending"
    required_gates = run.setdefault("verification", {}).setdefault("required_gates", [])
    if DEGRADATION_GATE not in required_gates:
        required_gates.append(DEGRADATION_GATE)
    return review


def review_issues(review: dict) -> list[str]:
    if not review.get("required", True):
        return []

    issues: list[str] = []
    answer = (review.get("answer") or "unanswered").strip().lower()
    if answer not in {"yes", "no"}:
        issues.append("explicit user answer")
    if not review.get("user_response", "").strip():
        issues.append("user response")

    preserved = nonempty_items(review.get("preserved_behaviors", []))
    checks = nonempty_items(review.get("regression_checks", []))
    allowed = nonempty_items(review.get("allowed_degradations", []))

    if not preserved:
        issues.append("preserved behaviors")
    if not checks:
        issues.append("regression checks")
    if answer == "yes" and not allowed:
        issues.append("approved degradation list")
    if answer == "no" and allowed:
        issues.append("allowed degradations must be empty when answer is no")

    return issues


def review_complete(review: dict) -> bool:
    return len(review_issues(review)) == 0


def degradation_status(review: dict) -> str:
    if not review.get("required", True):
        return "not-needed"
    return "passed" if review_complete(review) else "pending"


def summarize_degradation_review(review: dict) -> str:
    if not review.get("required", True):
        return "Degradation review is not required for this run."
    answer = (review.get("answer") or "unanswered").strip().lower()
    if review_complete(review):
        if answer == "yes":
            count = len(nonempty_items(review.get("allowed_degradations", [])))
            return f"Degradation review passed with {count} explicitly approved degradation(s)."
        return "Degradation review passed with no intended degradations and recorded regression checks."
    issues = ", ".join(review_issues(review)) or "unknown items"
    return f"Degradation review is incomplete: {issues}."


def degradation_feedback_messages(review: dict) -> list[str]:
    if not review.get("required", True):
        return []
    if review_complete(review):
        return []
    return [
        "Required degradation review was not completed. Ask whether the plan intentionally degrades existing behavior, then record preserved behaviors and regression checks."
    ]


def append_unique(existing: list[str], incoming: Iterable[str]) -> None:
    seen = {item.strip() for item in existing if item.strip()}
    for item in incoming:
        text = item.strip()
        if not text or text in seen:
            continue
        existing.append(text)
        seen.add(text)


def nonempty_items(items: Iterable[str]) -> list[str]:
    return [item.strip() for item in items if item and item.strip()]
