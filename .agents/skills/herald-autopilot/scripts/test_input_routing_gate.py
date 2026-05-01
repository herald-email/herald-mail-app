from __future__ import annotations

import json
import subprocess
import tempfile
import unittest
from pathlib import Path


SCRIPT_DIR = Path(__file__).resolve().parent
BOOTSTRAP = SCRIPT_DIR / "bootstrap_run.py"
RECORD_INPUT_ROUTING = SCRIPT_DIR / "record_input_routing_check.py"
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


class InputRoutingGateTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temp_dir = tempfile.TemporaryDirectory()
        self.repo_root = Path(self.temp_dir.name)

    def tearDown(self) -> None:
        self.temp_dir.cleanup()

    def bootstrap_run(self, *, task: str = "Fix compose comma alias shortcut routing") -> Path:
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
            "Prove alias routing does not steal text entry before scoring the run.",
            cwd=self.repo_root,
        )
        return Path(result.stdout.strip())

    def write_artifact(self, relative_path: str, content: str) -> Path:
        path = self.repo_root / relative_path
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
        return path

    def test_bootstrap_marks_shortcut_sensitive_tui_runs_as_input_routing_required(self) -> None:
        run_dir = self.bootstrap_run()

        run = load_json(run_dir / "run.json")

        self.assertIn("input_routing", run)
        self.assertTrue(run["input_routing"]["required"])
        self.assertEqual(run["input_routing"]["status"], "pending")
        self.assertEqual(run["input_routing"]["required_surfaces"], ["compose", "prompt", "editor"])
        self.assertEqual(run["input_routing"]["checks"], [])

    def test_record_input_routing_passes_after_all_required_surfaces(self) -> None:
        run_dir = self.bootstrap_run()

        for surface in ("compose", "prompt", "editor"):
            artifact = self.write_artifact(f"artifacts/{surface}.txt", f"{surface} transcript")
            run_python(
                RECORD_INPUT_ROUTING,
                "--run-dir",
                str(run_dir),
                "--surface",
                surface,
                "--input-sequence",
                ",",
                "--expected-behavior",
                "Literal comma is inserted into the active text field.",
                "--observed-behavior",
                "Literal comma stayed in the field and no alias fired.",
                "--artifact",
                str(artifact),
                "--text-preserved",
                "--repro-step",
                "Focus the text-entry surface.",
                "--repro-step",
                "Type a comma while the alias feature is enabled.",
                cwd=self.repo_root,
            )

        run = load_json(run_dir / "run.json")

        self.assertEqual(run["input_routing"]["status"], "passed")
        self.assertEqual(sorted(run["input_routing"]["required_surfaces"]), ["compose", "editor", "prompt"])
        self.assertEqual(len(run["input_routing"]["checks"]), 3)
        self.assertIn("input-routing-safety", run["verification"]["required_gates"])
        latest_result = run["verification"]["results"][-1]
        self.assertEqual(latest_result["gate"], "input-routing-safety")
        self.assertEqual(latest_result["status"], "pass")

    def test_score_and_report_surface_missing_input_routing_gate(self) -> None:
        run_dir = self.bootstrap_run()

        run_python(SCORE, "--run-dir", str(run_dir), cwd=self.repo_root)
        run_python(RENDER, "--run-dir", str(run_dir), cwd=self.repo_root)

        score = load_json(run_dir / "score.json")
        run = load_json(run_dir / "run.json")
        report = Path(run["paths"]["report_path"]).read_text()

        self.assertEqual(score["status"], "fail")
        self.assertEqual(score["axes"]["input_routing_readiness"], 0)
        self.assertTrue(any("input routing" in item.lower() for item in score["feedback"]))
        self.assertIn("Input Routing Safety Gate", report)
        self.assertIn("pending", report.lower())


if __name__ == "__main__":
    unittest.main()
