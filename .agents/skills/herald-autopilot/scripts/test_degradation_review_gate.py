from __future__ import annotations

import json
import subprocess
import tempfile
import unittest
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
BOOTSTRAP = SCRIPT_DIR / "bootstrap_run.py"
RECORD_DEGRADATION = SCRIPT_DIR / "record_degradation_review.py"
SCORE = SCRIPT_DIR / "score_run.py"
RENDER = SCRIPT_DIR / "render_report.py"


def run_python(script: Path, *args: str, cwd: Path) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["python3", str(script), *args],
        cwd=cwd,
        check=True,
        capture_output=True,
        text=True,
    )


def load_json(path: Path) -> dict:
    return json.loads(path.read_text())


class DegradationReviewGateTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_root = Path(self.temp_dir.name)

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def bootstrap_run(self, *, task: str = "Make cleanup chrome readable") -> Path:
        result = run_python(
            BOOTSTRAP,
            "--repo-root",
            str(self.repo_root),
            "--task",
            task,
            "--task-type",
            "feature",
            "--surfaces",
            "code,tui",
            "--plan-summary",
            "Make chrome readable while preserving existing preview and button behavior.",
            cwd=self.repo_root,
        )
        return Path(result.stdout.strip())

    def test_bootstrap_requires_degradation_review_for_every_run(self) -> None:
        run_dir = self.bootstrap_run()

        run = load_json(run_dir / "run.json")

        self.assertIn("degradation_review", run)
        self.assertTrue(run["degradation_review"]["required"])
        self.assertEqual(run["degradation_review"]["status"], "pending")
        self.assertEqual(run["degradation_review"]["answer"], "unanswered")
        self.assertEqual(run["degradation_review"]["allowed_degradations"], [])
        self.assertEqual(run["degradation_review"]["preserved_behaviors"], [])
        self.assertEqual(run["degradation_review"]["regression_checks"], [])
        self.assertIn("degradation-review", run["verification"]["required_gates"])

    def test_score_and_report_fail_when_degradation_review_is_missing(self) -> None:
        run_dir = self.bootstrap_run()

        run_python(SCORE, "--run-dir", str(run_dir), cwd=self.repo_root)
        run_python(RENDER, "--run-dir", str(run_dir), cwd=self.repo_root)

        score = load_json(run_dir / "score.json")
        run = load_json(run_dir / "run.json")
        report = Path(run["paths"]["report_path"]).read_text()

        self.assertEqual(score["status"], "fail")
        self.assertEqual(score["axes"]["degradation_review_readiness"], 0)
        self.assertTrue(any("degradation review" in item.lower() for item in score["feedback"]))
        self.assertIn("Degradation Review Gate", report)
        self.assertIn("pending", report.lower())

    def test_score_preserves_backward_compatibility_for_old_runs_without_degradation_gate(self) -> None:
        run_dir = self.bootstrap_run()
        run_path = run_dir / "run.json"
        run = load_json(run_path)
        run.pop("degradation_review")
        run["verification"]["required_gates"] = [
            gate for gate in run["verification"]["required_gates"] if gate != "degradation-review"
        ]
        run["baseline"]["status"] = "pass"
        run_path.write_text(json.dumps(run, indent=2) + "\n")

        run_python(SCORE, "--run-dir", str(run_dir), cwd=self.repo_root)

        score = load_json(run_dir / "score.json")
        self.assertEqual(score["axes"]["degradation_review_readiness"], 1)
        self.assertEqual(score["counts"]["degradation_review_required"], 0)

    def test_no_degradation_passes_only_with_preserved_behaviors_and_regression_checks(self) -> None:
        run_dir = self.bootstrap_run()

        run_python(
            RECORD_DEGRADATION,
            "--run-dir",
            str(run_dir),
            "--answer",
            "no",
            "--user-response",
            "No degradations are planned.",
            cwd=self.repo_root,
        )
        run = load_json(run_dir / "run.json")
        self.assertEqual(run["degradation_review"]["status"], "pending")

        run_python(
            RECORD_DEGRADATION,
            "--run-dir",
            str(run_dir),
            "--answer",
            "no",
            "--user-response",
            "No degradations are planned.",
            "--preserved-behavior",
            "Chrome buttons remain visible in demo screenshots.",
            "--regression-check",
            "Run tmux visual captures at 220x50, 80x24, and 50x15.",
            cwd=self.repo_root,
        )

        run = load_json(run_dir / "run.json")
        latest_result = run["verification"]["results"][-1]
        self.assertEqual(run["degradation_review"]["status"], "passed")
        self.assertEqual(latest_result["gate"], "degradation-review")
        self.assertEqual(latest_result["status"], "pass")

    def test_approved_degradation_requires_explicit_allowed_degradation_and_user_response(self) -> None:
        run_dir = self.bootstrap_run()

        run_python(
            RECORD_DEGRADATION,
            "--run-dir",
            str(run_dir),
            "--answer",
            "yes",
            "--user-response",
            "Approve removing the legacy compact button label.",
            cwd=self.repo_root,
        )
        run = load_json(run_dir / "run.json")
        self.assertEqual(run["degradation_review"]["status"], "pending")

        run_python(
            RECORD_DEGRADATION,
            "--run-dir",
            str(run_dir),
            "--answer",
            "yes",
            "--user-response",
            "Approve removing the legacy compact button label.",
            "--allowed-degradation",
            "Legacy compact button label is removed from the title row.",
            "--preserved-behavior",
            "Remaining chrome buttons stay visible.",
            "--regression-check",
            "Compare before/after title-row captures for visible button affordances.",
            cwd=self.repo_root,
        )

        run = load_json(run_dir / "run.json")
        latest_result = run["verification"]["results"][-1]
        self.assertEqual(run["degradation_review"]["status"], "passed")
        self.assertEqual(latest_result["gate"], "degradation-review")
        self.assertEqual(latest_result["status"], "pass")


if __name__ == "__main__":
    unittest.main()
