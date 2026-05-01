from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from pending_approvals import apply_pending_decision, build_pending_approvals, make_suggestion_key


def load_json(path: Path) -> dict:
    return json.loads(path.read_text())


class PendingApprovalsTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_root = Path(self.temp_dir.name)

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def write_run(
        self,
        run_id: str,
        *,
        suggestion_title: str,
        suggestion_why: str,
        approval_prompt: str,
        publication_actions: list[str] | None = None,
        created_at: str = "2026-05-01T19:00:00+00:00",
    ) -> Path:
        run_dir = self.repo_root / ".superpowers" / "autopilot" / "runs" / run_id
        run_dir.mkdir(parents=True, exist_ok=True)
        (run_dir / "run.json").write_text(
            json.dumps(
                {
                    "run_id": run_id,
                    "created_at": created_at,
                    "paths": {"repo_root": str(self.repo_root)},
                },
                indent=2,
            )
            + "\n"
        )
        (run_dir / "self_reflection.json").write_text(
            json.dumps(
                {
                    "generated_from_run": run_id,
                    "publication_actions": publication_actions or ["commit"],
                    "summary": "Synthetic reflection for pending-approval queue tests.",
                    "what_went_well": [],
                    "what_created_drag": [],
                    "matched_templates": [],
                    "suggested_changes": [
                        {
                            "title": suggestion_title,
                            "why": suggestion_why,
                            "approval_prompt": approval_prompt,
                        }
                    ],
                },
                indent=2,
            )
            + "\n"
        )
        return run_dir

    def test_build_pending_approvals_dedupes_repeated_published_suggestions(self) -> None:
        self.write_run(
            "20260501-run-a",
            suggestion_title="template-evidence-manifest-feedback",
            suggestion_why="Evidence-manifest failures should become a reusable template.",
            approval_prompt="Approve turning the evidence-manifest lesson into a reusable GEPA workflow template.",
            publication_actions=["commit"],
            created_at="2026-05-01T19:00:00+00:00",
        )
        self.write_run(
            "20260501-run-b",
            suggestion_title="template-evidence-manifest-feedback",
            suggestion_why="Evidence-manifest failures should become a reusable template.",
            approval_prompt="Approve turning the evidence-manifest lesson into a reusable GEPA workflow template.",
            publication_actions=["merge"],
            created_at="2026-05-01T19:05:00+00:00",
        )

        queue = build_pending_approvals(self.repo_root)

        self.assertEqual(queue["summary"]["total"], 1)
        self.assertEqual(queue["summary"]["pending"], 1)
        item = queue["items"][0]
        self.assertEqual(item["title"], "template-evidence-manifest-feedback")
        self.assertEqual(item["status"], "pending")
        self.assertEqual(item["occurrences"], 2)
        self.assertEqual(item["last_seen_at"], "2026-05-01T19:05:00+00:00")
        self.assertEqual(
            {source["run_id"] for source in item["source_runs"]},
            {"20260501-run-a", "20260501-run-b"},
        )

    def test_build_pending_approvals_preserves_existing_decision_state(self) -> None:
        title = "template-evidence-manifest-feedback"
        why = "Evidence-manifest failures should become a reusable template."
        approval_prompt = "Approve turning the evidence-manifest lesson into a reusable GEPA workflow template."
        queue_key = make_suggestion_key(
            {
                "title": title,
                "why": why,
                "approval_prompt": approval_prompt,
            }
        )
        state_dir = self.repo_root / ".superpowers" / "autopilot" / "state"
        state_dir.mkdir(parents=True, exist_ok=True)
        (state_dir / "pending-approvals.json").write_text(
            json.dumps(
                {
                    "schema_version": "herald-autopilot.pending-approvals.v1",
                    "updated_at": "2026-05-01T19:10:00+00:00",
                    "summary": {"total": 1, "pending": 0, "approved": 1, "rejected": 0, "implemented": 0},
                    "items": [
                        {
                            "key": queue_key,
                            "title": title,
                            "why": why,
                            "approval_prompt": approval_prompt,
                            "status": "approved",
                            "decision": {
                                "status": "approved",
                                "decided_at": "2026-05-01T19:10:00+00:00",
                                "note": "Looks good.",
                            },
                            "first_seen_at": "2026-05-01T19:00:00+00:00",
                            "last_seen_at": "2026-05-01T19:00:00+00:00",
                            "occurrences": 1,
                            "source_runs": [],
                        }
                    ],
                },
                indent=2,
            )
            + "\n"
        )
        self.write_run(
            "20260501-run-a",
            suggestion_title=title,
            suggestion_why=why,
            approval_prompt=approval_prompt,
            created_at="2026-05-01T19:15:00+00:00",
        )

        queue = build_pending_approvals(self.repo_root)

        self.assertEqual(queue["summary"]["approved"], 1)
        item = queue["items"][0]
        self.assertEqual(item["status"], "approved")
        self.assertEqual(item["decision"]["note"], "Looks good.")
        self.assertEqual(item["last_seen_at"], "2026-05-01T19:15:00+00:00")
        self.assertEqual(item["occurrences"], 1)

    def test_apply_pending_decision_updates_multiple_items(self) -> None:
        self.write_run(
            "20260501-run-a",
            suggestion_title="template-evidence-manifest-feedback",
            suggestion_why="Evidence-manifest failures should become a reusable template.",
            approval_prompt="Approve turning the evidence-manifest lesson into a reusable GEPA workflow template.",
        )
        self.write_run(
            "20260501-run-b",
            suggestion_title="Require publication summary after merge",
            suggestion_why="Merge handoffs are clearer when the report includes a publication summary.",
            approval_prompt="Approve enforcing a publication summary after merge actions.",
            created_at="2026-05-01T19:20:00+00:00",
        )

        queue = build_pending_approvals(self.repo_root)
        keys = [item["key"] for item in queue["items"]]

        updated = apply_pending_decision(queue, keys, "approved", note="Approved in batch.")

        self.assertEqual(updated["summary"]["pending"], 0)
        self.assertEqual(updated["summary"]["approved"], 2)
        self.assertTrue(all(item["status"] == "approved" for item in updated["items"]))
        self.assertTrue(all(item["decision"]["note"] == "Approved in batch." for item in updated["items"]))


if __name__ == "__main__":
    unittest.main()
