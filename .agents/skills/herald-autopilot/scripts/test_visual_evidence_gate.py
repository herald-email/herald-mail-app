from __future__ import annotations

import json
import subprocess
import tempfile
import unittest
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
BOOTSTRAP = SCRIPT_DIR / "bootstrap_run.py"
RECORD_VISUAL = SCRIPT_DIR / "record_visual_evidence.py"
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


class VisualEvidenceGateTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_root = Path(self.temp_dir.name)

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def bootstrap_run(self, *, task: str = "Validate TUI visual gate") -> Path:
        result = run_python(
            BOOTSTRAP,
            "--repo-root",
            str(self.repo_root),
            "--task",
            task,
            "--task-type",
            "workflow-improvement",
            "--surfaces",
            "code,tui",
            "--plan-summary",
            "Record canonical TUI evidence before scoring the run.",
            cwd=self.repo_root,
        )
        return Path(result.stdout.strip())

    def write_artifact(self, relative_path: str, content: str) -> Path:
        path = self.repo_root / relative_path
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
        return path

    def test_bootstrap_marks_tui_runs_as_visual_gate_required(self) -> None:
        run_dir = self.bootstrap_run()

        run = load_json(run_dir / "run.json")

        self.assertIn("visual_evidence", run)
        self.assertTrue(run["visual_evidence"]["required"])
        self.assertEqual(run["visual_evidence"]["status"], "pending")
        self.assertEqual(run["visual_evidence"]["required_sizes"], ["220x50", "80x24", "50x15"])
        self.assertEqual(run["visual_evidence"]["pairs"], [])

    def test_record_visual_evidence_passes_after_all_canonical_sizes(self) -> None:
        run_dir = self.bootstrap_run()

        for size in ("220x50", "80x24", "50x15"):
            before_png = self.write_artifact(f"artifacts/{size}/before.png", "before")
            after_png = self.write_artifact(f"artifacts/{size}/after.png", "after")
            before_text = self.write_artifact(f"artifacts/{size}/before.ansi.txt", "before ansi")
            after_text = self.write_artifact(f"artifacts/{size}/after.ansi.txt", "after ansi")
            run_python(
                RECORD_VISUAL,
                "--run-dir",
                str(run_dir),
                "--state-label",
                "cleanup-preview",
                "--size",
                size,
                "--before-png",
                str(before_png),
                "--after-png",
                str(after_png),
                "--before-text",
                str(before_text),
                "--after-text",
                str(after_text),
                "--repro-step",
                "Launch Herald in tmux.",
                "--repro-step",
                "Open the cleanup preview for the selected sender.",
                cwd=self.repo_root,
            )

        run = load_json(run_dir / "run.json")

        self.assertEqual(run["visual_evidence"]["status"], "passed")
        self.assertEqual(sorted(run["visual_evidence"]["required_sizes"]), ["220x50", "50x15", "80x24"])
        self.assertEqual(len(run["visual_evidence"]["pairs"]), 3)
        self.assertIn("visual-evidence", run["verification"]["required_gates"])
        latest_result = run["verification"]["results"][-1]
        self.assertEqual(latest_result["gate"], "visual-evidence")
        self.assertEqual(latest_result["status"], "pass")

    def test_score_and_report_surface_missing_visual_gate(self) -> None:
        run_dir = self.bootstrap_run()

        run_python(SCORE, "--run-dir", str(run_dir), cwd=self.repo_root)
        run_python(RENDER, "--run-dir", str(run_dir), cwd=self.repo_root)

        score = load_json(run_dir / "score.json")
        run = load_json(run_dir / "run.json")
        report = Path(run["paths"]["report_path"]).read_text()

        self.assertEqual(score["status"], "fail")
        self.assertEqual(score["axes"]["visual_evidence_readiness"], 0)
        self.assertTrue(any("visual evidence" in item.lower() for item in score["feedback"]))
        self.assertIn("Visual Evidence", report)
        self.assertIn("pending", report.lower())


if __name__ == "__main__":
    unittest.main()
