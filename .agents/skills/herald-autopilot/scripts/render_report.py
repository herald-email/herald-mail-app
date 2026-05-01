#!/usr/bin/env python3
from __future__ import annotations

import argparse
from pathlib import Path

from artifact_io import load_json, save_json, save_text
from remediation_templates import load_remediation_templates, match_remediation_template


def load_optional_json(path: Path):
    if not path.exists():
        return None
    return load_json(path)


def artifact_path(run: dict, artifact: str) -> str:
    if not artifact:
        return ""
    path = Path(artifact)
    if not path.is_absolute():
        path = Path(run["paths"]["repo_root"]) / path
    return str(path)


def visual_screenshot_sections(run: dict, results: list[dict]) -> list[str]:
    screenshots = []
    for item in results:
        artifact = item.get("artifact", "")
        if item.get("kind") != "screenshot" or not artifact.lower().endswith(".png"):
            continue
        summary = item.get("summary", "")
        key = f"{summary} {artifact}".lower()
        if "before" in key:
            group = "Before"
        elif "after" in key:
            group = "After"
        else:
            continue
        screenshots.append((group, summary or group, artifact_path(run, artifact)))

    if not screenshots:
        return []

    lines = ["", "## Visual Evidence"]
    for group in ("Before", "After"):
        group_items = [item for item in screenshots if item[0] == group]
        if not group_items:
            continue
        lines.extend(["", f"### {group}"])
        for _, summary, path in group_items:
            lines.extend([f"![{summary}]({path})", ""])
    return lines[:-1] if lines[-1] == "" else lines


def unique(items: list[str]) -> list[str]:
    seen: set[str] = set()
    ordered: list[str] = []
    for item in items:
        text = item.strip()
        if not text or text in seen:
            continue
        seen.add(text)
        ordered.append(text)
    return ordered


def latest_preflight_results(preflight: dict) -> list[dict]:
    latest: dict[str, dict] = {}
    for item in preflight.get("results", []):
        name = item.get("check")
        if name:
            latest[name] = item
    return [latest[name] for name in sorted(latest)]


def build_self_reflection(run: dict, score: dict | None, reflections: list[Path], results: list[dict]) -> dict:
    publication = run.get("publication", {})
    actions = unique(publication.get("actions", []))
    product_truth = run.get("product_truth", {})
    preflight = run.get("preflight", {})
    preflight_results = latest_preflight_results(preflight)
    failed_preflight = [item for item in preflight_results if item.get("status") == "fail"]
    required = set(run.get("verification", {}).get("required_gates", []))
    required_results = [item for item in results if item.get("gate") in required]
    skipped = [item for item in results if item.get("status") == "skip"]
    failed_required = [item for item in required_results if item.get("status") == "fail"]
    latest_feedback = unique(run.get("latest_feedback", []))
    retry_count = int(run.get("metrics", {}).get("retry_count", 0))
    repo_root = Path(run["paths"]["repo_root"])
    current_brief = load_optional_json(repo_root / ".superpowers" / "autopilot" / "state" / "improvement-brief.json")
    templates = load_remediation_templates(repo_root)

    strengths: list[str] = []
    drag: list[str] = []
    suggestions: list[dict] = []
    matched_templates: list[dict] = []
    latest_evidence_name = ""

    if actions:
        strengths.append(f"Completed the requested publish action(s): {', '.join(actions)}.")
    if publication.get("summary"):
        strengths.append(publication["summary"])
    required_preflight = preflight.get("required_checks", [])
    if required_preflight and len(preflight_results) >= len(required_preflight) and not failed_preflight:
        strengths.append(f"All required preflight checks passed ({len(required_preflight)}/{len(required_preflight)}).")
    if required_results and not failed_required:
        strengths.append(f"All required verification gates passed ({len(required_results)}/{len(required_results)}).")
    if product_truth.get("required") and product_truth.get("status") in {"consulted", "updated-first"}:
        strengths.append(f"Product intent was grounded through `{product_truth.get('status')}` doc usage.")
    if retry_count == 0:
        strengths.append("The run reached handoff without needing a bounded retry loop.")

    if failed_required:
        drag.extend([f"Required gate `{item.get('gate', 'ungated')}` failed: {item.get('summary', '')}" for item in failed_required])
    if failed_preflight:
        drag.extend([f"Required preflight `{item.get('check', 'unknown')}` failed: {item.get('summary', '')}" for item in failed_preflight])
    if skipped:
        drag.extend([f"Gate `{item.get('gate') or 'ungated'}` was skipped: {item.get('summary', '')}" for item in skipped[:3]])
    if retry_count > 0:
        drag.append(f"The run needed {retry_count} retry attempt(s), which suggests reusable guidance is still missing.")
    if latest_feedback:
        drag.extend(latest_feedback[:3])
    if product_truth.get("required") and product_truth.get("status") != "updated-first":
        drag.append("Product docs informed the run, but the workflow did not update them first before implementation.")
    if score and score.get("status") == "needs_followup":
        drag.append("The scored handoff still flagged human follow-up as needed.")

    if retry_count > 0 or latest_feedback:
        if reflections:
            latest_reflection = load_json(reflections[-1])
            latest_evidence_name = latest_reflection.get("failing_evidence") or ""
        template_key, template = match_remediation_template(latest_evidence_name, templates)
        if template_key and template:
            matched_templates.append(
                {
                    "key": template_key,
                    "title": template.get("title", template_key),
                    "why": template.get("why", ""),
                    "checklist": template.get("checklist", []),
                    "approval_prompt": template.get("approval_prompt", ""),
                }
            )
            suggestions.append(
                {
                    "title": template.get("title", template_key),
                    "why": template.get("why", "This repeated failure class now has a reusable remediation template."),
                    "approval_prompt": template.get("approval_prompt", f"Approve keeping the `{template_key}` remediation template in GEPA."),
                }
            )
        else:
            evidence_name = latest_evidence_name or "a repeated failure mode"
            suggestions.append(
                {
                    "title": f"Template `{evidence_name}` remediation guidance",
                    "why": "This run needed retries or explicit feedback that could become reusable autopilot guidance.",
                    "approval_prompt": f"Approve turning the `{evidence_name}` lesson from this run into a reusable GEPA workflow template.",
                }
            )
    if run.get("task", {}).get("type") == "feature" and product_truth.get("status") != "updated-first":
        suggestions.append(
            {
                "title": "Require doc-first feature grounding",
                "why": "Feature work is safer when VISION, ARCHITECTURE, and specs are updated before code rather than only consulted during implementation.",
                "approval_prompt": "Approve a stricter doc-first gate for non-trivial feature runs.",
            }
        )
    if actions and not publication.get("summary"):
        suggestions.append(
            {
                "title": "Require a publication summary after commit or merge",
                "why": "Publish actions are more legible when the run records exactly what was committed or merged and why.",
                "approval_prompt": "Approve enforcing a publication summary whenever a run performs a commit, merge, push, or PR step.",
            }
        )
    if current_brief:
        recommendation = current_brief.get("recommended_experiment", {})
        name = recommendation.get("name")
        why = recommendation.get("why")
        if name and why:
            suggestions.append(
                {
                    "title": name,
                    "why": why,
                    "approval_prompt": f"Approve exploring `{name}` as the next explicit GEPA improvement pass.",
                }
            )

    strengths = unique(strengths)
    drag = unique(drag)

    deduped_suggestions: list[dict] = []
    seen_titles: set[str] = set()
    for item in suggestions:
        title = item["title"].strip()
        if not title or title in seen_titles:
            continue
        seen_titles.add(title)
        deduped_suggestions.append(item)
        if len(deduped_suggestions) >= 3:
            break

    if actions:
        summary = "The requested publish step completed, and this reflection captures what the run suggests improving next before GEPA changes itself."
    else:
        summary = "This reflection captures what the run suggests improving next, but no explicit publish step was recorded."

    return {
        "generated_from_run": run["run_id"],
        "publication_actions": actions,
        "summary": summary,
        "what_went_well": strengths,
        "what_created_drag": drag,
        "matched_templates": matched_templates,
        "suggested_changes": deduped_suggestions,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="Render a Herald Autopilot report from a run folder.")
    parser.add_argument("--run-dir", required=True, help="Path to the run directory")
    args = parser.parse_args()

    run_dir = Path(args.run_dir).resolve()
    run = load_json(run_dir / "run.json")
    score_path = run_dir / "score.json"
    score = load_json(score_path) if score_path.exists() else None
    report_path = Path(run["paths"]["report_path"])
    report_path.parent.mkdir(parents=True, exist_ok=True)

    required = set(run["verification"].get("required_gates", []))
    results = run["verification"].get("results", [])
    manifest_path = run_dir / "evidence" / "manifest.json"
    evidence_items = load_json(manifest_path) if manifest_path.exists() else results
    required_results = [item for item in results if item["gate"] in required]
    skipped = [item for item in results if item["status"] == "skip"]
    reflections = sorted((run_dir / "reflections").glob("*.json"))
    remaining_risks = run["outcome"].get("remaining_risks", [])
    feedback = score["feedback"] if score else run.get("latest_feedback", [])
    product_truth = run.get("product_truth", {})
    truth_sources = product_truth.get("sources", [])
    docs_updated = product_truth.get("docs_updated", [])
    publication = run.get("publication", {})
    preflight = run.get("preflight", {})
    preflight_results = latest_preflight_results(preflight)
    preflight_resources = preflight.get("resources", {})
    self_reflection = build_self_reflection(run, score, reflections, results)
    self_reflection_json_path = run_dir / "self_reflection.json"
    self_reflection_md_path = run_dir / "self_reflection.md"

    lines = [
        f"# Herald Autopilot Report — {run['run_id']}",
        "",
        "## Task",
        run["task"]["request"],
        "",
        f"- Type: {run['task']['type']}",
        f"- Surfaces: {', '.join(run['task']['surfaces']) if run['task']['surfaces'] else 'none recorded'}",
        f"- Status: {run['status']}",
        f"- Branch: `{run['paths']['branch']}`",
        f"- Worktree: `{run['paths']['worktree']}`",
        "",
        "## Plan Summary",
        run["plan"].get("summary") or "No plan summary recorded.",
        "",
        "## Preflight",
        f"- Status: {preflight.get('status', 'not recorded')}",
        f"- Required checks: {', '.join(preflight.get('required_checks', [])) if preflight.get('required_checks') else 'none recorded'}",
    ]
    if preflight_results:
        lines.extend([f"- [{item.get('status', 'unknown')}] `{item.get('check', 'unknown')}` — {item.get('summary', '')}" for item in preflight_results])
    else:
        lines.append("- No preflight results recorded.")
    if preflight_resources:
        lines.append("- Prepared resources:")
        for key, value in sorted(preflight_resources.items()):
            lines.append(f"- `{key}`: `{value}`")

    lines.extend(
        [
            "",
        "## Product Truth",
        f"- Required: {'yes' if product_truth.get('required') else 'no'}",
        f"- Status: {product_truth.get('status', 'not recorded')}",
        f"- Summary: {product_truth.get('summary') or 'No grounding summary recorded.'}",
        f"- Sources: {', '.join(truth_sources) if truth_sources else 'none recorded'}",
        f"- Docs updated first: {', '.join(docs_updated) if docs_updated else 'none recorded'}",
        "",
        "## Publication",
        f"- Actions: {', '.join(publication.get('actions', [])) if publication.get('actions') else 'none recorded'}",
        f"- Summary: {publication.get('summary') or 'No publication summary recorded.'}",
        "",
        "## Outcome",
        run["outcome"].get("summary") or "No outcome summary recorded.",
        ]
    )

    lines.extend(visual_screenshot_sections(run, evidence_items))
    lines.extend(["", "## Verification"])

    if required_results:
        for item in required_results:
            lines.append(f"- [{item['status']}] `{item['gate']}` — {item['summary']}")
    else:
        lines.append("- No required verification gates recorded.")

    if skipped:
        lines.extend(
            [
                "",
                "## Skipped Gates",
                *[f"- `{item.get('gate') or 'ungated'}` — {item['summary']}" for item in skipped],
            ]
        )

    lines.extend(
        [
            "",
            "## GEPA Reflection",
            f"- Reflections recorded: {len(reflections)}",
        ]
    )
    if feedback:
        lines.extend([f"- {item}" for item in feedback])
    else:
        lines.append("- No reflection feedback recorded.")

    if self_reflection["matched_templates"]:
        lines.extend(["", "Matched remediation templates:"])
        for template in self_reflection["matched_templates"]:
            lines.append(f"- {template['title']}: {template['why']}")
            for item in template.get("checklist", []):
                lines.append(f"- Checklist: {item}")

    lines.extend(
        [
            "",
            "## Self-Reflection",
            self_reflection["summary"],
            "",
            "What went well:",
        ]
    )
    if self_reflection["what_went_well"]:
        lines.extend([f"- {item}" for item in self_reflection["what_went_well"]])
    else:
        lines.append("- No standout strengths were recorded.")
    lines.append("What created drag:")
    if self_reflection["what_created_drag"]:
        lines.extend([f"- {item}" for item in self_reflection["what_created_drag"]])
    else:
        lines.append("- No notable drag points were recorded.")
    lines.append("Suggested GEPA changes pending approval:")
    if self_reflection["suggested_changes"]:
        for item in self_reflection["suggested_changes"]:
            lines.append(f"- {item['title']}: {item['why']}")
            lines.append(f"Approval required: {item['approval_prompt']}")
    else:
        lines.append("- No approval-ready workflow changes were suggested by this run.")

    lines.extend(["", "## Risks"])
    if remaining_risks:
        lines.extend([f"- {item}" for item in remaining_risks])
    else:
        lines.append("- No remaining risks recorded.")

    if score:
        lines.extend(
            [
                "",
                "## Score",
                f"- Overall score: {score['overall_score']}",
                f"- Status: {score['status']}",
                f"- Preflight readiness: {score['axes'].get('preflight_readiness', 'n/a')}",
                f"- Required gates passed: {score['counts']['required_passed']}/{score['counts']['required_gates']}",
                f"- Required preflight checks passed: {score['counts'].get('preflight_required', 0) - score['counts'].get('preflight_failed', 0) - score['counts'].get('preflight_missing', 0)}/{score['counts'].get('preflight_required', 0)}",
                f"- Retry count: {score['counts']['retry_count']}",
            ]
        )

    markdown = "\n".join(lines) + "\n"
    save_json(self_reflection_json_path, self_reflection)
    self_reflection_md_lines = [
        f"# Herald Autopilot Self-Reflection — {run['run_id']}",
        "",
        f"- Publication actions: {', '.join(self_reflection['publication_actions']) if self_reflection['publication_actions'] else 'none recorded'}",
        "",
        "## Summary",
        self_reflection["summary"],
        "",
        "## What Went Well",
    ]
    if self_reflection["what_went_well"]:
        self_reflection_md_lines.extend([f"- {item}" for item in self_reflection["what_went_well"]])
    else:
        self_reflection_md_lines.append("- No standout strengths were recorded.")
    self_reflection_md_lines.extend(["", "## What Created Drag"])
    if self_reflection["what_created_drag"]:
        self_reflection_md_lines.extend([f"- {item}" for item in self_reflection["what_created_drag"]])
    else:
        self_reflection_md_lines.append("- No notable drag points were recorded.")
    if self_reflection["matched_templates"]:
        self_reflection_md_lines.extend(["", "## Matched Remediation Templates"])
        for template in self_reflection["matched_templates"]:
            self_reflection_md_lines.append(f"- {template['title']}: {template['why']}")
            for item in template.get("checklist", []):
                self_reflection_md_lines.append(f"- Checklist: {item}")
    self_reflection_md_lines.extend(["", "## Suggested Changes Pending Approval"])
    if self_reflection["suggested_changes"]:
        for item in self_reflection["suggested_changes"]:
            self_reflection_md_lines.append(f"- {item['title']}: {item['why']}")
            self_reflection_md_lines.append(f"Approval required: {item['approval_prompt']}")
    else:
        self_reflection_md_lines.append("- No approval-ready workflow changes were suggested by this run.")
    save_text(self_reflection_md_path, "\n".join(self_reflection_md_lines) + "\n")
    save_text(report_path, markdown)
    save_text(run_dir / "summary.md", markdown)
    print(str(report_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
