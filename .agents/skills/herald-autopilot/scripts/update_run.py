#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
from pathlib import Path

from artifact_io import load_json, locked_paths, save_json
from visual_evidence import ensure_visual_evidence, require_visual_evidence


def now_utc() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def ensure_product_truth(run: dict) -> dict:
    product_truth = run.setdefault("product_truth", {})
    product_truth.setdefault("required", False)
    product_truth.setdefault("status", "not-needed")
    product_truth.setdefault("summary", "")
    product_truth.setdefault("sources", [])
    product_truth.setdefault("docs_updated", [])
    return product_truth


def ensure_publication(run: dict) -> dict:
    publication = run.setdefault("publication", {})
    publication.setdefault("actions", [])
    publication.setdefault("summary", "")
    return publication


def ensure_preflight(run: dict) -> dict:
    preflight = run.setdefault("preflight", {})
    preflight.setdefault("status", "pending")
    preflight.setdefault("required_checks", [])
    preflight.setdefault("results", [])
    preflight.setdefault("resources", {})
    return preflight


def configure_visual_requirement(run: dict, required: bool) -> dict:
    if required:
        return require_visual_evidence(run)
    visual = ensure_visual_evidence(run)
    visual["required"] = False
    visual["status"] = "not-needed"
    visual["required_sizes"] = []
    return visual


def main() -> int:
    parser = argparse.ArgumentParser(description="Update summary fields on a Herald Autopilot run.")
    parser.add_argument("--run-dir", required=True, help="Path to the run directory")
    parser.add_argument("--status", default="", help="New run status")
    parser.add_argument("--plan-summary", default="", help="Replacement plan summary")
    parser.add_argument("--decision", action="append", default=[], help="Append a plan decision")
    parser.add_argument("--outcome-summary", default="", help="Set the final outcome summary")
    parser.add_argument("--risk", action="append", default=[], help="Append a remaining risk")
    parser.add_argument("--clear-risks", action="store_true", help="Clear all recorded risks")
    parser.add_argument("--files-changed", type=int, default=None, help="Set files changed count")
    parser.add_argument("--human-followup", action="store_true", help="Mark human follow-up as needed")
    parser.add_argument("--no-human-followup", action="store_true", help="Mark human follow-up as not needed")
    parser.add_argument("--requires-product-truth", action="store_true", help="Mark product-truth grounding as required")
    parser.add_argument("--no-requires-product-truth", action="store_true", help="Mark product-truth grounding as not required")
    parser.add_argument("--product-truth-status", default="", choices=["pending", "consulted", "updated-first", "not-needed"], help="Set product-truth grounding status")
    parser.add_argument("--product-truth-summary", default="", help="Set the product-truth grounding summary")
    parser.add_argument("--truth-source", action="append", default=[], help="Append a consulted product-truth source")
    parser.add_argument("--doc-updated", action="append", default=[], help="Append a product doc updated before code")
    parser.add_argument("--clear-truth-sources", action="store_true", help="Clear recorded product-truth sources")
    parser.add_argument("--clear-docs-updated", action="store_true", help="Clear recorded pre-code doc updates")
    parser.add_argument("--publish-action", action="append", default=[], help="Append a publish action such as commit, merge, push, or pr")
    parser.add_argument("--clear-publish-actions", action="store_true", help="Clear recorded publish actions")
    parser.add_argument("--publication-summary", default="", help="Set the publication summary")
    parser.add_argument("--require-visual-evidence", action="store_true", help="Require the canonical visual-evidence gate for this run")
    parser.add_argument("--no-require-visual-evidence", action="store_true", help="Mark the visual-evidence gate as not required")
    parser.add_argument("--visual-status", default="", choices=["pending", "passed", "not-needed"], help="Override the visual-evidence status")
    args = parser.parse_args()

    run_path = Path(args.run_dir).resolve() / "run.json"

    with locked_paths(run_path):
        run = load_json(run_path)
        product_truth = ensure_product_truth(run)
        publication = ensure_publication(run)
        ensure_preflight(run)
        visual = ensure_visual_evidence(run)

        if args.status:
            run["status"] = args.status
        if args.plan_summary:
            run["plan"]["summary"] = args.plan_summary
        if args.decision:
            run["plan"]["decisions"].extend(args.decision)
        if args.outcome_summary:
            run["outcome"]["summary"] = args.outcome_summary
        if args.clear_risks:
            run["outcome"]["remaining_risks"] = []
        if args.risk:
            run["outcome"]["remaining_risks"].extend(args.risk)
        if args.files_changed is not None:
            run["metrics"]["files_changed"] = args.files_changed
        if args.human_followup:
            run["metrics"]["human_followup_needed"] = True
        if args.no_human_followup:
            run["metrics"]["human_followup_needed"] = False
        if args.requires_product_truth:
            product_truth["required"] = True
        if args.no_requires_product_truth:
            product_truth["required"] = False
        if args.product_truth_status:
            product_truth["status"] = args.product_truth_status
        if args.product_truth_summary:
            product_truth["summary"] = args.product_truth_summary
        if args.clear_truth_sources:
            product_truth["sources"] = []
        if args.truth_source:
            product_truth["sources"].extend(args.truth_source)
        if args.clear_docs_updated:
            product_truth["docs_updated"] = []
        if args.doc_updated:
            product_truth["docs_updated"].extend(args.doc_updated)
        if args.clear_publish_actions:
            publication["actions"] = []
        if args.publish_action:
            publication["actions"].extend(args.publish_action)
        if args.publication_summary:
            publication["summary"] = args.publication_summary
        if args.require_visual_evidence:
            visual = configure_visual_requirement(run, required=True)
        if args.no_require_visual_evidence:
            visual = configure_visual_requirement(run, required=False)
        if args.visual_status:
            visual["status"] = args.visual_status

        run["updated_at"] = now_utc()
        save_json(run_path, run)
    print(str(run_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
