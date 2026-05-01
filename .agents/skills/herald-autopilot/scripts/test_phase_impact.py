from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from phase_impact import PHASE_TITLES, build_phase_impact


class PhaseImpactTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_root = Path(self.temp_dir.name)
        state_dir = self.repo_root / ".superpowers" / "autopilot" / "state"
        state_dir.mkdir(parents=True, exist_ok=True)
        (state_dir / "improvement-log.json").write_text(
            json.dumps(
                {
                    "schema_version": "herald-autopilot.improvement-log.v1",
                    "updated_at": "2026-05-01T14:00:00+00:00",
                    "entries": [
                        {"logged_at": "2026-05-01T10:00:00+00:00", "title": PHASE_TITLES["phase1"]},
                        {"logged_at": "2026-05-01T11:00:00+00:00", "title": PHASE_TITLES["phase2"]},
                        {"logged_at": "2026-05-01T12:00:00+00:00", "title": PHASE_TITLES["phase3"]},
                        {"logged_at": "2026-05-01T13:00:00+00:00", "title": PHASE_TITLES["phase4"]},
                    ],
                },
                indent=2,
            )
            + "\n"
        )
        (state_dir / "pending-approvals.json").write_text(
            json.dumps(
                {
                    "schema_version": "herald-autopilot.pending-approvals.v1",
                    "updated_at": "2026-05-01T14:00:00+00:00",
                    "summary": {
                        "total": 3,
                        "pending": 2,
                        "approved": 1,
                        "rejected": 0,
                        "implemented": 0,
                        "published_runs_analyzed": 2,
                    },
                    "items": [],
                },
                indent=2,
            )
            + "\n"
        )

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def write_run(
        self,
        run_id: str,
        created_at: str,
        *,
        retry_count: int,
        skipped_gates: int,
        human_followup: bool,
        questions_asked: int,
    ) -> None:
        run_dir = self.repo_root / ".superpowers" / "autopilot" / "runs" / run_id
        run_dir.mkdir(parents=True, exist_ok=True)
        results = []
        for index in range(skipped_gates):
            results.append(
                {
                    "gate": f"gate-{index}",
                    "status": "skip",
                    "summary": f"Skipped gate {index}",
                }
            )
        (run_dir / "run.json").write_text(
            json.dumps(
                {
                    "run_id": run_id,
                    "created_at": created_at,
                    "metrics": {
                        "retry_count": retry_count,
                        "files_changed": 1,
                        "human_followup_needed": human_followup,
                    },
                    "plan": {
                        "summary": "Synthetic measurement run",
                        "questions_asked": [f"question-{index}" for index in range(questions_asked)],
                        "decisions": [],
                    },
                    "verification": {
                        "required_gates": [],
                        "results": results,
                    },
                    "publication": {"actions": [], "summary": ""},
                    "paths": {"repo_root": str(self.repo_root)},
                },
                indent=2,
            )
            + "\n"
        )

    def test_build_phase_impact_computes_phase_window_metrics(self) -> None:
        self.write_run(
            "baseline-run",
            "2026-05-01T09:30:00+00:00",
            retry_count=2,
            skipped_gates=2,
            human_followup=True,
            questions_asked=1,
        )
        self.write_run(
            "phase1-run",
            "2026-05-01T10:30:00+00:00",
            retry_count=1,
            skipped_gates=1,
            human_followup=False,
            questions_asked=0,
        )
        self.write_run(
            "phase2-run",
            "2026-05-01T11:30:00+00:00",
            retry_count=0,
            skipped_gates=0,
            human_followup=False,
            questions_asked=0,
        )
        self.write_run(
            "phase3-run",
            "2026-05-01T12:30:00+00:00",
            retry_count=0,
            skipped_gates=0,
            human_followup=False,
            questions_asked=0,
        )
        self.write_run(
            "phase4-run",
            "2026-05-01T13:30:00+00:00",
            retry_count=0,
            skipped_gates=0,
            human_followup=False,
            questions_asked=0,
        )

        impact = build_phase_impact(self.repo_root)

        baseline = impact["windows"]["baseline_pre_phase1"]["metrics"]
        phase4 = impact["windows"]["phase4_pending_approval_queue"]["metrics"]
        current = impact["current_vs_baseline"]["current_metrics"]
        delta = impact["current_vs_baseline"]["delta"]

        self.assertEqual(baseline["run_count"], 1)
        self.assertEqual(baseline["average_retry_count"], 2.0)
        self.assertEqual(baseline["average_skipped_gates"], 2.0)
        self.assertEqual(baseline["human_followup_rate"], 1.0)
        self.assertEqual(baseline["average_clarification_touches"], 2.0)

        self.assertEqual(phase4["run_count"], 1)
        self.assertEqual(phase4["average_retry_count"], 0.0)
        self.assertEqual(current["run_count"], 4)
        self.assertEqual(current["average_retry_count"], 0.25)
        self.assertAlmostEqual(delta["average_retry_count"], -1.75)
        self.assertAlmostEqual(delta["average_skipped_gates"], -1.75)
        self.assertAlmostEqual(delta["human_followup_rate"], -1.0)
        self.assertAlmostEqual(delta["average_clarification_touches"], -2.0)

    def test_build_phase_impact_reports_queue_visibility_when_phase4_has_no_runs(self) -> None:
        self.write_run(
            "baseline-run",
            "2026-05-01T09:30:00+00:00",
            retry_count=1,
            skipped_gates=1,
            human_followup=True,
            questions_asked=1,
        )
        self.write_run(
            "phase3-run",
            "2026-05-01T12:30:00+00:00",
            retry_count=0,
            skipped_gates=0,
            human_followup=False,
            questions_asked=0,
        )

        impact = build_phase_impact(self.repo_root)

        phase4 = impact["windows"]["phase4_pending_approval_queue"]["metrics"]
        queue = impact["pending_approvals"]

        self.assertEqual(phase4["run_count"], 0)
        self.assertEqual(queue["pending"], 2)
        self.assertEqual(queue["approved"], 1)
        self.assertEqual(queue["published_runs_analyzed"], 2)
        self.assertTrue(any("queue" in finding.lower() for finding in impact["findings"]))


if __name__ == "__main__":
    unittest.main()
