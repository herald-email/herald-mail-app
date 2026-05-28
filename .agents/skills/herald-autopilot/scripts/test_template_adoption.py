from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from template_adoption import build_template_adoption, render_template_adoption_markdown


class TemplateAdoptionTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_root = Path(self.temp_dir.name)

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def write_run(
        self,
        run_id: str,
        created_at: str,
        *,
        retry_count: int,
        publication_actions: list[str],
        matched_templates: list[dict] | None = None,
        drag: list[str] | None = None,
    ) -> None:
        run_dir = self.repo_root / ".superpowers" / "autopilot" / "runs" / run_id
        run_dir.mkdir(parents=True, exist_ok=True)
        (run_dir / "run.json").write_text(
            json.dumps(
                {
                    "run_id": run_id,
                    "created_at": created_at,
                    "metrics": {"retry_count": retry_count},
                    "publication": {"actions": publication_actions},
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
                    "publication_actions": publication_actions,
                    "summary": "Synthetic reflection.",
                    "what_went_well": [],
                    "what_created_drag": drag or [],
                    "matched_templates": matched_templates or [],
                    "suggested_changes": [],
                },
                indent=2,
            )
            + "\n"
        )

    def test_build_template_adoption_measures_published_eligible_runs(self) -> None:
        self.write_run(
            "20260501-template-match",
            "2026-05-01T10:00:00+00:00",
            retry_count=1,
            publication_actions=["commit"],
            drag=["The run needed 1 retry attempt."],
            matched_templates=[
                {
                    "key": "focused-tests",
                    "title": "Focused test remediation template",
                    "why": "Repeated test failures need a checklist.",
                    "checklist": [],
                }
            ],
        )
        self.write_run(
            "20260501-no-template",
            "2026-05-01T11:00:00+00:00",
            retry_count=2,
            publication_actions=["merge"],
            drag=["The run needed 2 retry attempts."],
        )
        self.write_run(
            "20260501-no-drag",
            "2026-05-01T12:00:00+00:00",
            retry_count=0,
            publication_actions=["commit"],
        )
        self.write_run(
            "20260501-unpublished",
            "2026-05-01T13:00:00+00:00",
            retry_count=1,
            publication_actions=[],
            matched_templates=[{"key": "app-tests", "title": "Application test-suite remediation template"}],
        )

        adoption = build_template_adoption(self.repo_root)

        summary = adoption["summary"]
        self.assertEqual(summary["published_reflections_analyzed"], 3)
        self.assertEqual(summary["eligible_runs"], 2)
        self.assertEqual(summary["eligible_runs_with_templates"], 1)
        self.assertEqual(summary["eligible_adoption_rate"], 0.5)
        self.assertEqual(summary["published_runs_with_templates"], 1)
        self.assertEqual(summary["average_retry_count_with_templates"], 1.0)
        self.assertEqual(summary["average_retry_count_without_templates"], 1.0)

        self.assertEqual(adoption["templates"][0]["key"], "focused-tests")
        self.assertEqual(adoption["templates"][0]["matched_runs"], 1)
        self.assertEqual(adoption["unmatched_eligible_runs"][0]["run_id"], "20260501-no-template")

    def test_render_template_adoption_markdown_names_rates_and_unmatched_runs(self) -> None:
        self.write_run(
            "20260501-template-match",
            "2026-05-01T10:00:00+00:00",
            retry_count=1,
            publication_actions=["commit"],
            drag=["The run needed 1 retry attempt."],
            matched_templates=[{"key": "focused-tests", "title": "Focused test remediation template"}],
        )
        self.write_run(
            "20260501-no-template",
            "2026-05-01T11:00:00+00:00",
            retry_count=2,
            publication_actions=["merge"],
            drag=["The run needed 2 retry attempts."],
        )

        markdown = render_template_adoption_markdown(build_template_adoption(self.repo_root))

        self.assertIn("Eligible adoption rate: 50%", markdown)
        self.assertIn("focused-tests", markdown)
        self.assertIn("20260501-no-template", markdown)


if __name__ == "__main__":
    unittest.main()
