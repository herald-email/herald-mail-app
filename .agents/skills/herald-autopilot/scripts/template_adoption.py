from __future__ import annotations

from collections import defaultdict
from pathlib import Path
from typing import Any

from artifact_io import load_json
from optimizer_common import list_runs, now_utc


def average(values: list[int | float]) -> float:
    if not values:
        return 0.0
    return sum(values) / len(values)


def rate(numerator: int, denominator: int) -> float | None:
    if denominator == 0:
        return None
    return numerator / denominator


def percent(value: float | None) -> str:
    if value is None:
        return "n/a"
    return f"{value:.0%}"


def load_self_reflection(run_dir: Path) -> dict[str, Any] | None:
    path = run_dir / "self_reflection.json"
    if not path.exists():
        return None
    return load_json(path)


def publication_actions(run: dict[str, Any], reflection: dict[str, Any]) -> list[str]:
    actions = reflection.get("publication_actions") or run.get("publication", {}).get("actions", [])
    return [str(action) for action in actions if str(action).strip()]


def normalized_template(template: dict[str, Any]) -> dict[str, str]:
    key = str(template.get("key") or template.get("title") or "unknown-template").strip()
    title = str(template.get("title") or key).strip()
    return {"key": key, "title": title}


def run_retry_count(run: dict[str, Any], score: dict[str, Any] | None) -> int:
    if score:
        counts = score.get("counts", {})
        if "retry_count" in counts:
            return int(counts.get("retry_count") or 0)
    return int(run.get("metrics", {}).get("retry_count", 0))


def is_template_eligible(retry_count: int, template_keys: list[str]) -> bool:
    return retry_count > 0 or bool(template_keys)


def build_template_adoption(repo_root: Path, limit: int | None = None) -> dict[str, Any]:
    repo_root = repo_root.resolve()
    runs: list[dict[str, Any]] = []
    template_runs: dict[str, dict[str, Any]] = {}
    template_retry_counts: defaultdict[str, list[int]] = defaultdict(list)

    for record in list_runs(repo_root, limit=limit):
        reflection = load_self_reflection(record.run_dir)
        if not reflection:
            continue
        actions = publication_actions(record.run, reflection)
        if not actions:
            continue

        retry_count = run_retry_count(record.run, record.score)
        matched_templates = [normalized_template(item) for item in reflection.get("matched_templates", [])]
        template_keys = sorted({item["key"] for item in matched_templates if item["key"]})
        template_titles = {item["key"]: item["title"] for item in matched_templates if item["key"]}
        eligible = is_template_eligible(retry_count, template_keys)
        run_item = {
            "run_id": record.run_id,
            "created_at": record.created_at,
            "publication_actions": actions,
            "retry_count": retry_count,
            "eligible": eligible,
            "matched_template_keys": template_keys,
            "matched_template_titles": [template_titles[key] for key in template_keys],
        }
        runs.append(run_item)

        for key in template_keys:
            if key not in template_runs:
                template_runs[key] = {"key": key, "title": template_titles.get(key, key), "run_ids": []}
            template_runs[key]["run_ids"].append(record.run_id)
            template_retry_counts[key].append(retry_count)

    published_count = len(runs)
    runs_with_templates = [item for item in runs if item["matched_template_keys"]]
    runs_without_templates = [item for item in runs if not item["matched_template_keys"]]
    eligible_runs = [item for item in runs if item["eligible"]]
    eligible_with_templates = [item for item in eligible_runs if item["matched_template_keys"]]
    unmatched_eligible_runs = [item for item in eligible_runs if not item["matched_template_keys"]]

    templates = []
    for key, item in template_runs.items():
        retry_counts = template_retry_counts[key]
        templates.append(
            {
                "key": key,
                "title": item["title"],
                "matched_runs": len(item["run_ids"]),
                "average_retry_count": average(retry_counts),
                "run_ids": sorted(item["run_ids"]),
            }
        )
    templates.sort(key=lambda item: (-item["matched_runs"], item["key"]))

    summary = {
        "published_reflections_analyzed": published_count,
        "published_runs_with_templates": len(runs_with_templates),
        "published_adoption_rate": rate(len(runs_with_templates), published_count),
        "eligible_runs": len(eligible_runs),
        "eligible_runs_with_templates": len(eligible_with_templates),
        "eligible_adoption_rate": rate(len(eligible_with_templates), len(eligible_runs)),
        "total_template_matches": sum(len(item["matched_template_keys"]) for item in runs),
        "average_retry_count_all": average([item["retry_count"] for item in runs]),
        "average_retry_count_with_templates": average([item["retry_count"] for item in runs_with_templates]),
        "average_retry_count_without_templates": average([item["retry_count"] for item in runs_without_templates]),
        "average_retry_count_eligible_with_templates": average([item["retry_count"] for item in eligible_with_templates]),
        "average_retry_count_eligible_without_templates": average([item["retry_count"] for item in unmatched_eligible_runs]),
        "unmatched_eligible_runs": len(unmatched_eligible_runs),
    }
    summary["eligible_retry_delta_with_templates"] = (
        summary["average_retry_count_eligible_with_templates"] - summary["average_retry_count_eligible_without_templates"]
        if eligible_with_templates and unmatched_eligible_runs
        else None
    )

    return {
        "schema_version": "herald-autopilot.template-adoption.v1",
        "generated_at": now_utc(),
        "summary": summary,
        "templates": templates,
        "runs": sorted(runs, key=lambda item: item["created_at"], reverse=True),
        "unmatched_eligible_runs": sorted(unmatched_eligible_runs, key=lambda item: item["created_at"], reverse=True),
    }


def render_template_adoption_markdown(adoption: dict[str, Any]) -> str:
    summary = adoption["summary"]
    lines = [
        "# Herald GEPA Remediation Template Adoption",
        "",
        "This report measures whether published Herald autopilot self-reflections actually use reusable remediation templates, and whether retry counts differ between matched and unmatched eligible runs.",
        "",
        "## Summary",
        "",
        f"- Generated at: {adoption.get('generated_at', '')}",
        f"- Published reflections analyzed: {summary['published_reflections_analyzed']}",
        f"- Published runs with templates: {summary['published_runs_with_templates']}",
        f"- Published adoption rate: {percent(summary['published_adoption_rate'])}",
        f"- Eligible runs: {summary['eligible_runs']}",
        f"- Eligible runs with templates: {summary['eligible_runs_with_templates']}",
        f"- Eligible adoption rate: {percent(summary['eligible_adoption_rate'])}",
        f"- Total template matches: {summary['total_template_matches']}",
        f"- Average retries with templates: {summary['average_retry_count_with_templates']:.2f}",
        f"- Average retries without templates: {summary['average_retry_count_without_templates']:.2f}",
        f"- Eligible retry delta with templates: {summary['eligible_retry_delta_with_templates']:+.2f}"
        if summary["eligible_retry_delta_with_templates"] is not None
        else "- Eligible retry delta with templates: n/a",
        "",
        "## Template Matches",
        "",
    ]

    if adoption["templates"]:
        for template in adoption["templates"]:
            lines.extend(
                [
                    f"### {template['title']}",
                    "",
                    f"- Key: `{template['key']}`",
                    f"- Matched runs: {template['matched_runs']}",
                    f"- Average retries: {template['average_retry_count']:.2f}",
                    f"- Runs: {', '.join(f'`{run_id}`' for run_id in template['run_ids'])}",
                    "",
                ]
            )
    else:
        lines.extend(["- No remediation templates were matched in published reflections.", ""])

    lines.extend(["## Unmatched Eligible Runs", ""])
    if adoption["unmatched_eligible_runs"]:
        for run in adoption["unmatched_eligible_runs"]:
            lines.append(f"- `{run['run_id']}`: retries {run['retry_count']}, actions {', '.join(run['publication_actions'])}")
    else:
        lines.append("- Every eligible published reflection matched at least one remediation template.")

    lines.extend(
        [
            "",
            "## Caveats",
            "",
            "- Template matches appear only when self-reflection records them, so older runs without `self_reflection.json` are outside this measurement.",
            "- This is observational. Lower or higher retries on matched runs can reflect task difficulty as much as template usefulness.",
        ]
    )
    return "\n".join(lines) + "\n"
