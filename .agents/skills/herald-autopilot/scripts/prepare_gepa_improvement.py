#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

from optimizer_common import load_json, now_utc, save_json, save_text, state_dir
from remediation_templates import load_remediation_templates, match_remediation_template


def main() -> int:
    parser = argparse.ArgumentParser(description="Prepare a GEPA improvement brief from recent optimizer artifacts.")
    parser.add_argument("--repo-root", default=".", help="Repository root")
    args = parser.parse_args()

    repo_root = Path(args.repo_root).resolve()
    out_dir = state_dir(repo_root)
    summary = load_json(out_dir / "recent-run-summary.json")
    frontier = load_json(out_dir / "frontier.json")
    patterns = load_json(out_dir / "feedback-patterns.json")
    templates = load_remediation_templates(repo_root)

    total_runs = int(summary.get("total_runs", 0))
    failed_runs = int(summary.get("status_counts", {}).get("failed", 0))
    product_truth = summary.get("product_truth", {})
    grounded_runs = int(product_truth.get("grounded_runs", 0))
    required_grounding_runs = int(product_truth.get("required_runs", 0))
    grounding_rate = product_truth.get("grounding_rate")
    visual = summary.get("visual_evidence", {})
    visual_required_runs = int(visual.get("required_runs", 0))
    visual_ready_runs = int(visual.get("ready_runs", 0))
    visual_readiness_rate = visual.get("readiness_rate")
    real_task_gap = total_runs < 5
    top_failure = patterns.get("top_failing_evidence", [])
    top_failure_name = top_failure[0]["name"] if top_failure else ""
    top_failure_template_key, _ = match_remediation_template(top_failure_name, templates)
    next_uncovered_failure = ""
    for item in top_failure:
        key, _ = match_remediation_template(item.get("name", ""), templates)
        if key is None:
            next_uncovered_failure = item.get("name", "")
            break

    if required_grounding_runs > 0 and grounding_rate is not None and grounding_rate < 0.8:
        bottleneck = "Feature and behavior runs are not yet consistently grounded in the product-definition docs before implementation."
        recommendation = {
            "name": "enforce-product-truth-gating",
            "why": "Improving doc-first grounding is a safer and more leveraged next step than expanding search while specs and vision handoff are still inconsistent.",
            "risk": "low",
            "value": "high",
        }
    elif visual_required_runs >= 3 and visual_readiness_rate is not None and visual_readiness_rate < 0.8:
        bottleneck = "TUI-facing runs are still not consistently closing the visual-evidence gate across the canonical terminal sizes."
        recommendation = {
            "name": "enforce-visual-evidence-gate-adoption",
            "why": "Phase 3 only pays off if TUI-facing runs reliably capture canonical before/after evidence instead of treating screenshots as optional.",
            "risk": "low",
            "value": "high",
        }
    elif real_task_gap:
        bottleneck = "The workflow does not yet have enough real bug or feature runs to justify aggressive self-modification."
        recommendation = {
            "name": "auto-ledger-and-state-sync",
            "why": "The safest next step is to reduce operator effort and accumulate higher-quality history before adding challenger worktrees.",
            "risk": "low",
            "value": "high",
        }
    elif top_failure_name and top_failure_template_key and next_uncovered_failure:
        bottleneck = f"The most repeated failure `{top_failure_name}` now has a reusable template, so the next uncovered repeated failure is `{next_uncovered_failure}`."
        recommendation = {
            "name": f"template-{next_uncovered_failure}-feedback",
            "why": "The next best improvement is to extend template coverage to the next repeated failure class instead of re-implementing an existing template.",
            "risk": "low",
            "value": "medium",
        }
    elif top_failure_name and top_failure_template_key:
        bottleneck = f"The most repeated failure `{top_failure_name}` already has a reusable template, so the next step is to measure and enforce template adoption consistently."
        recommendation = {
            "name": "measure-remediation-template-adoption",
            "why": "Once the main repeated failure classes have templates, the next leverage comes from checking whether runs actually use them and whether retries decline.",
            "risk": "low",
            "value": "medium",
        }
    elif top_failure_name:
        bottleneck = f"The most repeated failing evidence is `{top_failure_name}`, which suggests a reusable verification or remediation pattern is missing."
        recommendation = {
            "name": f"template-{top_failure_name}-feedback",
            "why": "Repeated failure shapes are strong candidates for reusable feedback templates and tighter gate guidance.",
            "risk": "low",
            "value": "medium",
        }
    elif failed_runs > 0:
        bottleneck = "The workflow is still losing confidence on some runs because failures are not being converted into enough reusable policy."
        recommendation = {
            "name": "risk-ranked-remediation-policies",
            "why": "Codifying repeated failure classes is safer than jumping straight to challenger worktrees.",
            "risk": "medium",
            "value": "medium",
        }
    else:
        bottleneck = "The workflow has healthy bootstrap signals and can start exploring limited candidate comparison."
        recommendation = {
            "name": "two-candidate-worktree-trial",
            "why": "A narrow challenger-worktree experiment is the next step toward true GEPA-style search once the baseline is trustworthy.",
            "risk": "medium",
            "value": "high",
        }

    brief = {
        "generated_at": now_utc(),
        "current_bottleneck": bottleneck,
        "recommended_experiment": recommendation,
        "evidence": {
            "recent_run_count": total_runs,
            "frontier_count": frontier.get("frontier_count", 0),
            "failed_run_count": failed_runs,
            "product_truth_required_runs": required_grounding_runs,
            "product_truth_grounded_runs": grounded_runs,
            "product_truth_grounding_rate": grounding_rate,
            "visual_required_runs": visual_required_runs,
            "visual_ready_runs": visual_ready_runs,
            "visual_readiness_rate": visual_readiness_rate,
            "top_failing_evidence": patterns.get("top_failing_evidence", []),
            "top_risks": patterns.get("top_risks", []),
        },
        "secondary_experiments": [
            "spec-to-implementation handoff templates",
            "frontier-backed candidate comparison",
            "feedback-template mining",
            "verification cost measurement",
        ],
    }

    save_json(out_dir / "improvement-brief.json", brief)
    save_json(
        out_dir / "optimizer-state.json",
        {
            "generated_at": brief["generated_at"],
            "summary_path": str(out_dir / "recent-run-summary.json"),
            "frontier_path": str(out_dir / "frontier.json"),
            "patterns_path": str(out_dir / "feedback-patterns.json"),
            "brief_path": str(out_dir / "improvement-brief.json"),
        },
    )

    markdown = "\n".join(
        [
            "# GEPA Improvement Brief",
            "",
            f"- Generated at: {brief['generated_at']}",
            "",
            "## Current Bottleneck",
            brief["current_bottleneck"],
            "",
            "## Recommended Experiment",
            f"- Name: {recommendation['name']}",
            f"- Why: {recommendation['why']}",
            f"- Value: {recommendation['value']}",
            f"- Risk: {recommendation['risk']}",
            "",
            "## Evidence",
            f"- Recent runs: {brief['evidence']['recent_run_count']}",
            f"- Frontier members: {brief['evidence']['frontier_count']}",
            f"- Failed runs: {brief['evidence']['failed_run_count']}",
            f"- Product-truth required runs: {brief['evidence']['product_truth_required_runs']}",
            f"- Product-truth grounded runs: {brief['evidence']['product_truth_grounded_runs']}",
            f"- Product-truth grounding rate: {brief['evidence']['product_truth_grounding_rate'] if brief['evidence']['product_truth_grounding_rate'] is not None else 'n/a'}",
            f"- Visual-required runs: {brief['evidence']['visual_required_runs']}",
            f"- Visual-ready runs: {brief['evidence']['visual_ready_runs']}",
            f"- Visual readiness rate: {brief['evidence']['visual_readiness_rate'] if brief['evidence']['visual_readiness_rate'] is not None else 'n/a'}",
        ]
        + [f"- Top failing evidence: {item['name']} ({item['count']})" for item in brief["evidence"]["top_failing_evidence"][:3]]
        + [""]
    )
    save_text(out_dir / "improvement-brief.md", markdown)
    print(str(out_dir / "improvement-brief.json"))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
