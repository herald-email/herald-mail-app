#!/usr/bin/env python3
from __future__ import annotations

import argparse
from collections import Counter
from pathlib import Path

from optimizer_common import list_runs, load_json, now_utc, save_json, save_text, state_dir


def main() -> int:
    parser = argparse.ArgumentParser(description="Extract repeated feedback patterns from Herald Autopilot runs.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    parser.add_argument("--limit", type=int, default=30, help="Maximum number of recent runs to inspect")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    runs = list_runs(repo_root, limit=args.limit)
    failing_evidence_counts: Counter[str] = Counter()
    feedback_counts: Counter[str] = Counter()
    risk_counts: Counter[str] = Counter()

    for record in runs:
        for feedback in record.run.get("latest_feedback", []):
            feedback_counts[feedback] += 1
        for risk in record.run.get("outcome", {}).get("remaining_risks", []):
            risk_counts[risk] += 1
        reflections_dir = record.run_dir / "reflections"
        if reflections_dir.exists():
            for path in sorted(reflections_dir.glob("*.json")):
                reflection = load_json(path)
                failing = reflection.get("failing_evidence")
                if failing:
                    failing_evidence_counts[failing] += 1
                for feedback in reflection.get("feedback", []):
                    feedback_counts[feedback] += 1

    result = {
        "generated_at": now_utc(),
        "top_failing_evidence": [{"name": name, "count": count} for name, count in failing_evidence_counts.most_common(5)],
        "top_feedback": [{"name": name, "count": count} for name, count in feedback_counts.most_common(5)],
        "top_risks": [{"name": name, "count": count} for name, count in risk_counts.most_common(5)],
    }

    out_dir = state_dir(repo_root)
    save_json(out_dir / "feedback-patterns.json", result)

    lines = [
        "# Feedback Patterns",
        "",
        f"- Generated at: {result['generated_at']}",
        "",
        "## Top Failing Evidence",
    ]
    if failing_evidence_counts:
        lines.extend([f"- {name}: {count}" for name, count in failing_evidence_counts.most_common(5)])
    else:
        lines.append("- none")

    lines.extend(["", "## Top Feedback"])
    if feedback_counts:
        lines.extend([f"- {name}: {count}" for name, count in feedback_counts.most_common(5)])
    else:
        lines.append("- none")

    lines.extend(["", "## Top Risks"])
    if risk_counts:
        lines.extend([f"- {name}: {count}" for name, count in risk_counts.most_common(5)])
    else:
        lines.append("- none")

    save_text(out_dir / "feedback-patterns.md", "\n".join(lines) + "\n")
    print(str(out_dir / "feedback-patterns.json"))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
