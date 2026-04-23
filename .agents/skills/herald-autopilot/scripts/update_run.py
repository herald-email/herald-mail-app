#!/usr/bin/env python3
from __future__ import annotations

import argparse
import datetime as dt
import json
from pathlib import Path


def now_utc() -> str:
    return dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat()


def load_json(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def save_json(path: Path, payload) -> None:
    path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")


def ensure_product_truth(run: dict) -> dict:
    product_truth = run.setdefault("product_truth", {})
    product_truth.setdefault("required", False)
    product_truth.setdefault("status", "not-needed")
    product_truth.setdefault("summary", "")
    product_truth.setdefault("sources", [])
    product_truth.setdefault("docs_updated", [])
    return product_truth


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
    args = parser.parse_args()

    run_path = Path(args.run_dir).resolve() / "run.json"
    run = load_json(run_path)
    product_truth = ensure_product_truth(run)

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

    run["updated_at"] = now_utc()
    save_json(run_path, run)
    print(str(run_path))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
